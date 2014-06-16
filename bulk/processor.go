package bulk

import (
	"os"
	"time"

	"github.com/cloudfoundry-incubator/runtime-schema/bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	"github.com/cloudfoundry/gosteno"
)

type Processor struct {
	bbs             bbs.NsyncBBS
	pollingInterval time.Duration
	bulkBatchSize   uint
	logger          *gosteno.Logger
	fetcher         Fetcher
}

func NewProcessor(
	bbs bbs.NsyncBBS,
	pollingInterval time.Duration,
	bulkBatchSize uint,
	logger *gosteno.Logger,
	fetcher Fetcher) *Processor {
	return &Processor{
		bbs:             bbs,
		pollingInterval: pollingInterval,
		bulkBatchSize:   bulkBatchSize,
		logger:          logger,
		fetcher:         fetcher,
	}
}

func (p *Processor) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	for {
		existing, err := p.bbs.GetAllDesiredLRPs()
		if err != nil {
			select {
			case <-signals:
				return nil
			case <-time.After(p.pollingInterval):
				continue
			}
		}

		fromCC := make(chan models.DesiredLRP)
		go p.fetcher.Fetch(fromCC)

		changes := Diff(existing, fromCC)

	dance:
		for {
			select {
			case change, ok := <-changes:
				if !ok {
					changes = nil
					break
				}

				p.bbs.ChangeDesiredLRP(change)
			case <-signals:
				return nil
			case <-time.After(p.pollingInterval):
				break dance
			}
		}
	}

	return nil
}
