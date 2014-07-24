package main_test

import (
	"fmt"
	"os"
	"strings"

	Bbs "github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry/gunk/timeprovider"
	"github.com/cloudfoundry/yagnats"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/ifrit"

	"github.com/cloudfoundry-incubator/nsync/integration/runner"
)

var _ = Describe("Syncing desired state with CC", func() {
	var (
		natsClient yagnats.NATSClient
		bbs        *Bbs.BBS

		run     ifrit.Runner
		process ifrit.Process
	)

	BeforeEach(func() {
		natsClient = natsRunner.MessageBus

		bbs = Bbs.NewBBS(etcdRunner.Adapter(), timeprovider.NewTimeProvider(), lagertest.NewTestLogger("test"))

		run = runner.NewRunner(
			"nsync.listener.started",
			listenerPath,
			"-etcdCluster", strings.Join(etcdRunner.NodeURLS(), ","),
			"-natsAddresses", fmt.Sprintf("127.0.0.1:%d", natsPort),
		)

		process = ifrit.Envoke(run)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait(), 5).Should(Receive())
	})

	var publishDesireWithInstances = func(nInstances int) {
		err := natsClient.Publish("diego.desire.app", []byte(fmt.Sprintf(`
        {
          "process_guid": "the-guid",
          "droplet_uri": "http://the-droplet.uri.com",
          "start_command": "the-start-command",
          "memory_mb": 128,
          "disk_mb": 512,
          "file_descriptors": 32,
          "num_instances": %d,
          "stack": "some-stack",
          "log_guid": "the-log-guid"
        }
      `, nInstances)))
		Ω(err).ShouldNot(HaveOccurred())
	}

	Describe("when a 'diego.desire.app' message is recieved", func() {
		BeforeEach(func() {
			publishDesireWithInstances(3)
		})

		It("registers an app desire in etcd", func() {
			Eventually(bbs.GetAllDesiredLRPs, 10).Should(HaveLen(1))
		})

		Context("when an app is no longer desired", func() {
			BeforeEach(func() {
				Eventually(bbs.GetAllDesiredLRPs).Should(HaveLen(1))

				publishDesireWithInstances(0)
			})

			It("should remove the desired state from etcd", func() {
				Eventually(bbs.GetAllDesiredLRPs).Should(HaveLen(0))
			})
		})
	})
})
