package main_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	bbstestrunner "github.com/cloudfoundry-incubator/bbs/cmd/bbs/testrunner"
	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry-incubator/consuladapter/consulrunner"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var (
	bulkerPath string

	receptorPath string
	receptorPort int

	bbsPath string
	bbsURL  *url.URL
)

var bbsArgs bbstestrunner.Args
var bbsRunner *ginkgomon.Runner
var bbsProcess ifrit.Process

var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdClient storeadapter.StoreAdapter

var consulRunner *consulrunner.ClusterRunner
var consulSession *consuladapter.Session

func TestBulker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bulker Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	bulker, err := gexec.Build("github.com/cloudfoundry-incubator/nsync/cmd/nsync-bulker", "-race")
	Expect(err).NotTo(HaveOccurred())

	receptor, err := gexec.Build("github.com/cloudfoundry-incubator/receptor/cmd/receptor", "-race")
	Expect(err).NotTo(HaveOccurred())

	bbs, err := gexec.Build("github.com/cloudfoundry-incubator/bbs/cmd/bbs", "-race")
	Expect(err).NotTo(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"bulker":   bulker,
		"receptor": receptor,
		"bbs":      bbs,
	})
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	binaries := map[string]string{}

	err := json.Unmarshal(payload, &binaries)
	Expect(err).NotTo(HaveOccurred())

	etcdPort := 5001 + GinkgoParallelNode()
	receptorPort = 6001 + GinkgoParallelNode()

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1, nil)

	consulRunner = consulrunner.NewClusterRunner(
		9001+config.GinkgoConfig.ParallelNode*consulrunner.PortOffsetLength,
		1,
		"http",
	)

	bulkerPath = string(binaries["bulker"])
	receptorPath = string(binaries["receptor"])
	etcdClient = etcdRunner.Adapter(nil)

	bbsPath = string(binaries["bbs"])
	bbsAddress := fmt.Sprintf("127.0.0.1:%d", 13000+GinkgoParallelNode())

	bbsURL = &url.URL{
		Scheme: "http",
		Host:   bbsAddress,
	}

	bbsArgs = bbstestrunner.Args{
		Address:     bbsAddress,
		EtcdCluster: strings.Join(etcdRunner.NodeURLS(), ","),
	}
})

var _ = BeforeEach(func() {
	etcdRunner.Start()

	consulRunner.Start()
	consulRunner.WaitUntilReady()
	consulSession = consulRunner.NewSession("a-session")

	bbsRunner = bbstestrunner.New(bbsPath, bbsArgs)
	bbsProcess = ginkgomon.Invoke(bbsRunner)
})

var _ = AfterEach(func() {
	ginkgomon.Kill(bbsProcess)
	consulRunner.Stop()
	etcdRunner.Stop()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
