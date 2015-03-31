package main_test

import (
	"encoding/json"
	"testing"

	"github.com/cloudfoundry-incubator/consuladapter"
	"github.com/cloudfoundry/storeadapter"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	bulkerPath string

	receptorPath string
	receptorPort int
)

var etcdRunner *etcdstorerunner.ETCDClusterRunner
var etcdClient storeadapter.StoreAdapter

var consulPort int
var consulRunner consuladapter.ClusterRunner
var consulAdapter consuladapter.Adapter

func TestBulker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bulker Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	bulker, err := gexec.Build("github.com/cloudfoundry-incubator/nsync/cmd/nsync-bulker", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	receptor, err := gexec.Build("github.com/cloudfoundry-incubator/receptor/cmd/receptor", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"bulker":   bulker,
		"receptor": receptor,
	})
	Ω(err).ShouldNot(HaveOccurred())

	return payload
}, func(payload []byte) {
	binaries := map[string]string{}

	err := json.Unmarshal(payload, &binaries)
	Ω(err).ShouldNot(HaveOccurred())

	etcdPort := 5001 + GinkgoParallelNode()
	receptorPort = 6001 + GinkgoParallelNode()

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)

	consulPort = 9001 + config.GinkgoConfig.ParallelNode*consuladapter.PortOffsetLength
	consulRunner = consuladapter.NewClusterRunner(
		consulPort,
		1,
		"http",
	)

	bulkerPath = string(binaries["bulker"])
	receptorPath = string(binaries["receptor"])
	etcdClient = etcdRunner.Adapter()
})

var _ = BeforeEach(func() {
	consulRunner.Start()
	etcdRunner.Start()
	consulAdapter = consulRunner.NewAdapter()
})

var _ = AfterEach(func() {
	consulRunner.Stop()
	etcdRunner.Stop()
})

var _ = SynchronizedAfterSuite(func() {
}, func() {
	gexec.CleanupBuildArtifacts()
})
