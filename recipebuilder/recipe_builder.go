package recipebuilder

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	RepRoutes "github.com/cloudfoundry-incubator/rep/routes"
	"github.com/cloudfoundry-incubator/runtime-schema/cc_messages"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	SchemaRouter "github.com/cloudfoundry-incubator/runtime-schema/router"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/rata"
)

const DockerScheme = "docker"

var ErrNoCircusDefined = errors.New("no lifecycle binary bundle defined for stack")
var ErrAppSourceMissing = errors.New("desired app missing both droplet_uri and docker_image_url; exactly one is required.")
var ErrMultipleAppSources = errors.New("desired app contains both droplet_uri and docker_image_url; exactly one is required.")

type RecipeBuilder struct {
	repAddrRelativeToExecutor string
	logger                    lager.Logger
	circuses                  map[string]string
}

func New(repAddrRelativeToExecutor string, circuses map[string]string, logger lager.Logger) *RecipeBuilder {
	return &RecipeBuilder{
		repAddrRelativeToExecutor: repAddrRelativeToExecutor,
		circuses:                  circuses,
		logger:                    logger,
	}
}

func (b *RecipeBuilder) Build(desiredApp cc_messages.DesireAppRequestFromCC) (models.DesiredLRP, error) {
	lrpGuid := desiredApp.ProcessGuid

	buildLogger := b.logger.Session("message-builder")

	if desiredApp.DropletUri == "" && desiredApp.DockerImageUrl == "" {
		buildLogger.Error("desired-app-invalid", ErrAppSourceMissing, lager.Data{"desired-app": desiredApp})
		return models.DesiredLRP{}, ErrAppSourceMissing
	}

	if desiredApp.DropletUri != "" && desiredApp.DockerImageUrl != "" {
		buildLogger.Error("desired-app-invalid", ErrMultipleAppSources, lager.Data{"desired-app": desiredApp})
		return models.DesiredLRP{}, ErrMultipleAppSources
	}

	rootFSPath := ""

	if desiredApp.DockerImageUrl != "" {
		dockerUrl, err := url.Parse(desiredApp.DockerImageUrl)
		if err != nil {
			buildLogger.Error("docker-url-invalid", err, lager.Data{"docker-url": desiredApp.DockerImageUrl})
			return models.DesiredLRP{}, err
		}
		dockerUrl.Scheme = DockerScheme
		rootFSPath = dockerUrl.String()
	}

	circusURL, err := b.circusDownloadURL(desiredApp.Stack, "PLACEHOLDER_FILESERVER_URL")
	if err != nil {
		buildLogger.Error("construct-circus-download-url-failed", err, lager.Data{
			"stack": desiredApp.Stack,
		})

		return models.DesiredLRP{}, err
	}

	var numFiles *uint64
	if desiredApp.FileDescriptors != 0 {
		numFiles = &desiredApp.FileDescriptors
	}

	repRequests := rata.NewRequestGenerator(
		"http://"+b.repAddrRelativeToExecutor,
		RepRoutes.Routes,
	)

	healthyHook, err := repRequests.CreateRequest(
		RepRoutes.LRPRunning,
		rata.Params{
			"process_guid": lrpGuid,

			// these go away once rep is polling, rather than receiving callbacks
			"index":         "PLACEHOLDER_INSTANCE_INDEX",
			"instance_guid": "PLACEHOLDER_INSTANCE_GUID",
		},
		nil,
	)
	if err != nil {
		return models.DesiredLRP{}, err
	}

	actions := []models.ExecutorAction{}
	actions = append(actions, models.ExecutorAction{
		Action: models.DownloadAction{
			From:    circusURL,
			To:      "/tmp/circus",
			Extract: true,
		},
	})

	if desiredApp.DropletUri != "" {
		actions = append(actions, models.ExecutorAction{
			Action: models.DownloadAction{
				From:     desiredApp.DropletUri,
				To:       ".",
				Extract:  true,
				CacheKey: fmt.Sprintf("droplets-%s", lrpGuid),
			},
		})
	}

	actions = append(actions, models.Parallel(
		models.ExecutorAction{
			models.RunAction{
				Path: "/tmp/circus/soldier",
				Args: append([]string{"/app"}, strings.Split(desiredApp.StartCommand, " ")...),
				Env:  createLrpEnv(desiredApp.Environment.BBSEnvironment()),
				ResourceLimits: models.ResourceLimits{
					Nofile: numFiles,
				},
			},
		},
		models.ExecutorAction{
			models.MonitorAction{
				Action: models.ExecutorAction{
					models.RunAction{
						Path: "/tmp/circus/spy",
						Args: []string{"-addr=:8080"},
					},
				},
				HealthyThreshold:   1,
				UnhealthyThreshold: 1,
				HealthyHook: models.HealthRequest{
					Method: healthyHook.Method,
					URL:    healthyHook.URL.String(),
				},
			},
		}))

	return models.DesiredLRP{
		ProcessGuid: lrpGuid,
		Instances:   desiredApp.NumInstances,
		Routes:      desiredApp.Routes,

		MemoryMB: desiredApp.MemoryMB,
		DiskMB:   desiredApp.DiskMB,

		Ports: []models.PortMapping{
			{ContainerPort: 8080},
		},

		RootFSPath: rootFSPath,

		Stack: desiredApp.Stack,

		Log: models.LogConfig{
			Guid:       desiredApp.LogGuid,
			SourceName: "App",
		},

		Actions: actions,
	}, nil
}

func (b RecipeBuilder) circusDownloadURL(stack string, fileServerURL string) (string, error) {
	checkPath, ok := b.circuses[stack]
	if !ok {
		return "", ErrNoCircusDefined
	}

	staticRoute, ok := SchemaRouter.NewFileServerRoutes().RouteForHandler(SchemaRouter.FS_STATIC)
	if !ok {
		return "", errors.New("couldn't generate the download path for the bundle of app lifecycle binaries")
	}

	return urljoiner.Join(fileServerURL, staticRoute.Path, checkPath), nil
}

func createLrpEnv(env []models.EnvironmentVariable) []models.EnvironmentVariable {
	env = append(env, models.EnvironmentVariable{Name: "PORT", Value: "8080"})
	env = append(env, models.EnvironmentVariable{Name: "VCAP_APP_PORT", Value: "8080"})
	env = append(env, models.EnvironmentVariable{Name: "VCAP_APP_HOST", Value: "0.0.0.0"})
	return env
}
