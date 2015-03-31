package main

import (
	"encoding/json"
	"flag"
	"os"
	"time"

	"github.com/cloudfoundry-incubator/cf-debug-server"
	cf_lager "github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/cf_http"
	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/receptor"
	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/lock_bbs"
	"github.com/cloudfoundry/dropsonde"
	"github.com/nu7hatch/gouuid"
	"github.com/pivotal-golang/clock"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"

	"github.com/cloudfoundry-incubator/nsync/bulk"
	"github.com/cloudfoundry-incubator/nsync/recipebuilder"
)

var diegoAPIURL = flag.String(
	"diegoAPIURL",
	"",
	"URL of diego API",
)

var consulCluster = flag.String(
	"consulCluster",
	"",
	"comma-separated list of consul server URLs (scheme://ip:port)",
)

var lockTTL = flag.Duration(
	"lockTTL",
	lock_bbs.LockTTL,
	"TTL for service lock",
)

var heartbeatRetryInterval = flag.Duration(
	"heartbeatRetryInterval",
	lock_bbs.RetryInterval,
	"interval to wait before retrying a failed lock acquisition",
)

var ccBaseURL = flag.String(
	"ccBaseURL",
	"",
	"base URL of the cloud controller",
)

var ccUsername = flag.String(
	"ccUsername",
	"",
	"basic auth username for CC bulk API",
)

var ccPassword = flag.String(
	"ccPassword",
	"",
	"basic auth password for CC bulk API",
)

var communicationTimeout = flag.Duration(
	"communicationTimeout",
	30*time.Second,
	"Timeout applied to all HTTP requests.",
)

var pollingInterval = flag.Duration(
	"pollingInterval",
	30*time.Second,
	"interval at which to poll bulk API",
)

var domainTTL = flag.Duration(
	"domainTTL",
	2*time.Minute,
	"duration of the domain; bumped on every bulk sync",
)

var bulkBatchSize = flag.Uint(
	"bulkBatchSize",
	500,
	"number of apps to fetch at once from bulk API",
)

var skipCertVerify = flag.Bool(
	"skipCertVerify",
	false,
	"skip SSL certificate verification",
)

var lifecycles = flag.String(
	"lifecycles",
	"",
	"app lifecycle binary bundle mapping (stack => bundle filename in fileserver)",
)

var fileServerURL = flag.String(
	"fileServerURL",
	"",
	"URL of the file server",
)

const (
	dropsondeOrigin      = "nsync_bulker"
	dropsondeDestination = "localhost:3457"
)

func main() {
	cf_debug_server.AddFlags(flag.CommandLine)
	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	cf_http.Initialize(*communicationTimeout)

	logger, reconfigurableSink := cf_lager.New("nsync-bulker")
	initializeDropsonde(logger)

	diegoAPIClient := receptor.NewClient(*diegoAPIURL)

	bbs := initializeBbs(logger)

	uuid, err := uuid.NewV4()
	if err != nil {
		logger.Fatal("Couldn't generate uuid", err)
	}

	var lifecycleDownloadURLs map[string]string
	err = json.Unmarshal([]byte(*lifecycles), &lifecycleDownloadURLs)
	if err != nil {
		logger.Fatal("invalid-lifecycle-mapping", err)
	}

	recipeBuilder := recipebuilder.New(lifecycleDownloadURLs, *fileServerURL, logger)

	heartbeater := bbs.NewNsyncBulkerLock(uuid.String(), *lockTTL, *heartbeatRetryInterval)

	runner := bulk.NewProcessor(
		diegoAPIClient,
		*pollingInterval,
		*domainTTL,
		*bulkBatchSize,
		*skipCertVerify,
		logger,
		&bulk.CCFetcher{
			BaseURI:   *ccBaseURL,
			BatchSize: int(*bulkBatchSize),
			Username:  *ccUsername,
			Password:  *ccPassword,
		},
		recipeBuilder,
		clock.NewClock(),
	)

	members := grouper.Members{
		{"heartbeater", heartbeater},
		{"runner", runner},
	}

	if dbgAddr := cf_debug_server.DebugAddress(flag.CommandLine); dbgAddr != "" {
		members = append(grouper.Members{
			{"debug-server", cf_debug_server.Runner(dbgAddr, reconfigurableSink)},
		}, members...)
	}

	group := grouper.NewOrdered(os.Interrupt, members)

	logger.Info("waiting-for-lock")

	monitor := ifrit.Invoke(sigmon.New(group))

	logger.Info("started")

	err = <-monitor.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
	os.Exit(0)
}

func initializeDropsonde(logger lager.Logger) {
	err := dropsonde.Initialize(dropsondeDestination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed to initialize dropsonde: %v", err)
	}
}

func initializeBbs(logger lager.Logger) Bbs.NsyncBBS {
	consulScheme, consulAddresses, err := consuladapter.Parse(*consulCluster)
	if err != nil {
		logger.Fatal("failed-parsing-consul-cluster", err)
	}

	consulAdapter, err := consuladapter.NewAdapter(consulAddresses, consulScheme)
	if err != nil {
		logger.Fatal("failed-building-consul-adapter", err)
	}

	return Bbs.NewNsyncBBS(consulAdapter, clock.NewClock(), logger)
}
