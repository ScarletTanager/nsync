package recipebuilder

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudfoundry-incubator/bbs/models"
	ssh_routes "github.com/cloudfoundry-incubator/diego-ssh/routes"
	"github.com/cloudfoundry-incubator/route-emitter/cfroutes"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/pivotal-golang/lager"
)

type BuildpackRecipeBuilder struct {
	logger lager.Logger
	config Config
}

func NewBuildpackRecipeBuilder(logger lager.Logger, config Config) *BuildpackRecipeBuilder {
	return &BuildpackRecipeBuilder{
		logger: logger,
		config: config,
	}
}

func (b *BuildpackRecipeBuilder) Build(desiredApp *cc_messages.DesireAppRequestFromCC) (*models.DesiredLRP, error) {
	lrpGuid := desiredApp.ProcessGuid

	buildLogger := b.logger.Session("message-builder")

	if desiredApp.DropletUri == "" {
		buildLogger.Error("desired-app-invalid", ErrDropletSourceMissing, lager.Data{"desired-app": desiredApp})
		return nil, ErrDropletSourceMissing
	}

	if desiredApp.DropletUri != "" && desiredApp.DockerImageUrl != "" {
		buildLogger.Error("desired-app-invalid", ErrMultipleAppSources, lager.Data{"desired-app": desiredApp})
		return nil, ErrMultipleAppSources
	}

	var lifecycle = "buildpack/" + desiredApp.Stack
	lifecyclePath, ok := b.config.Lifecycles[lifecycle]
	if !ok {
		buildLogger.Error("unknown-lifecycle", ErrNoLifecycleDefined, lager.Data{
			"lifecycle": lifecycle,
		})

		return nil, ErrNoLifecycleDefined
	}

	lifecycleURL := lifecycleDownloadURL(lifecyclePath, b.config.FileServerURL)

	rootFSPath := models.PreloadedRootFS(desiredApp.Stack)

	var privilegedContainer bool = true
	var containerEnvVars []*models.EnvironmentVariable
	containerEnvVars = append(containerEnvVars, &models.EnvironmentVariable{"LANG", DefaultLANG})

	numFiles := DefaultFileDescriptorLimit
	if desiredApp.FileDescriptors != 0 {
		numFiles = desiredApp.FileDescriptors
	}

	var setup []models.ActionInterface
	var actions []models.ActionInterface
	var monitor models.ActionInterface

	setup = append(setup, &models.DownloadAction{
		From:     lifecycleURL,
		To:       "/tmp/lifecycle",
		CacheKey: fmt.Sprintf("%s-lifecycle", strings.Replace(lifecycle, "/", "-", 1)),
		User:     "vcap",
	})

	var exposedPort = DefaultPort

	switch desiredApp.HealthCheckType {
	case cc_messages.PortHealthCheckType, cc_messages.UnspecifiedHealthCheckType:
		fileDescriptorLimit := DefaultFileDescriptorLimit
		monitor = models.Timeout(
			&models.RunAction{
				User:      "vcap",
				Path:      "/tmp/lifecycle/healthcheck",
				Args:      []string{fmt.Sprintf("-port=%d", exposedPort)},
				LogSource: HealthLogSource,
				ResourceLimits: &models.ResourceLimits{
					Nofile: &fileDescriptorLimit,
				},
			},
			30*time.Second,
		)
	}

	setup = append(setup, &models.DownloadAction{
		From:     desiredApp.DropletUri,
		To:       ".",
		CacheKey: fmt.Sprintf("droplets-%s", lrpGuid),
		User:     "vcap",
	})

	actions = append(actions, &models.RunAction{
		User: "vcap",
		Path: "/tmp/lifecycle/launcher",
		Args: append(
			[]string{"app"},
			desiredApp.StartCommand,
			desiredApp.ExecutionMetadata,
		),
		Env:       createLrpEnv(desiredApp.Environment, exposedPort),
		LogSource: AppLogSource,
		ResourceLimits: &models.ResourceLimits{
			Nofile: &numFiles,
		},
	})

	desiredAppRoutingInfo := cfroutes.CFRoutes{
		{Hostnames: desiredApp.Routes, Port: exposedPort},
	}.RoutingInfo()

	desiredAppPorts := []uint32{exposedPort}

	if desiredApp.AllowSSH {
		hostKeyPair, err := b.config.KeyFactory.NewKeyPair(1024)
		if err != nil {
			buildLogger.Error("new-host-key-pair-failed", err)
			return nil, err
		}

		userKeyPair, err := b.config.KeyFactory.NewKeyPair(1024)
		if err != nil {
			buildLogger.Error("new-user-key-pair-failed", err)
			return nil, err
		}

		actions = append(actions, &models.RunAction{
			User: "vcap",
			Path: "/tmp/lifecycle/diego-sshd",
			Args: []string{
				"-address=" + fmt.Sprintf("0.0.0.0:%d", DefaultSSHPort),
				"-hostKey=" + hostKeyPair.PEMEncodedPrivateKey(),
				"-authorizedKey=" + userKeyPair.AuthorizedKey(),
				"-inheritDaemonEnv",
				"-logLevel=fatal",
			},
			Env: createLrpEnv(desiredApp.Environment, exposedPort),
			ResourceLimits: &models.ResourceLimits{
				Nofile: &numFiles,
			},
		})

		sshRoutePayload, err := json.Marshal(ssh_routes.SSHRoute{
			ContainerPort:   2222,
			PrivateKey:      userKeyPair.PEMEncodedPrivateKey(),
			HostFingerprint: hostKeyPair.Fingerprint(),
		})

		if err != nil {
			buildLogger.Error("marshaling-ssh-route-failed", err)
			return nil, err
		}

		sshRouteMessage := json.RawMessage(sshRoutePayload)
		desiredAppRoutingInfo[ssh_routes.DIEGO_SSH] = &sshRouteMessage
		desiredAppPorts = append(desiredAppPorts, DefaultSSHPort)
	}

	setupAction := models.Serial(setup...)
	actionAction := models.Codependent(actions...)

	return &models.DesiredLRP{
		Privileged: privilegedContainer,

		Domain: cc_messages.AppLRPDomain,

		ProcessGuid: lrpGuid,
		Instances:   int32(desiredApp.NumInstances),
		Routes:      &desiredAppRoutingInfo,
		Annotation:  desiredApp.ETag,

		CpuWeight: cpuWeight(desiredApp.MemoryMB),

		MemoryMb: int32(desiredApp.MemoryMB),
		DiskMb:   int32(desiredApp.DiskMB),

		Ports: desiredAppPorts,

		RootFs: rootFSPath,

		LogGuid:   desiredApp.LogGuid,
		LogSource: LRPLogSource,

		MetricsGuid: desiredApp.LogGuid,

		EnvironmentVariables: containerEnvVars,
		Setup:                models.WrapAction(setupAction),
		Action:               models.WrapAction(actionAction),
		Monitor:              models.WrapAction(monitor),

		StartTimeout: uint32(desiredApp.HealthCheckTimeoutInSeconds),

		EgressRules: desiredApp.EgressRules,
	}, nil
}

func (b BuildpackRecipeBuilder) ExtractExposedPort(executionMetadata string) (uint32, error) {
	return DefaultPort, nil
}
