package main_test

import (
	"testing"

	"github.com/cloudfoundry/gunk/natsrunner"
	"github.com/cloudfoundry/storeadapter/storerunner/etcdstorerunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var listenerPath string

var etcdRunner *etcdstorerunner.ETCDClusterRunner
var natsRunner *natsrunner.NATSRunner
var natsPort int

func TestListener(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Listener Suite")
}

var _ = BeforeSuite(func() {
	var err error

	listenerPath, err = gexec.Build("github.com/cloudfoundry-incubator/nsync/listener", "-race")
	Ω(err).ShouldNot(HaveOccurred())

	etcdPort := 5001 + GinkgoParallelNode()
	natsPort = 4001 + GinkgoParallelNode()

	etcdRunner = etcdstorerunner.NewETCDClusterRunner(etcdPort, 1)
	natsRunner = natsrunner.NewNATSRunner(natsPort)
})

var _ = BeforeEach(func() {
	etcdRunner.Start()
	natsRunner.Start()
})

var _ = AfterEach(func() {
	etcdRunner.Stop()
	natsRunner.Stop()
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
