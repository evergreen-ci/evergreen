package taskrunner

import (
	"context"
	"math/rand"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// TODO: take out task queue finder and host finder once transition is complete
type TaskRunner struct {
	*evergreen.Settings
	HostGateway
}

func NewTaskRunner(settings *evergreen.Settings) *TaskRunner {
	// get mci home, and set the source and destination for the agent
	// executables
	evgHome := evergreen.FindEvergreenHome()

	return &TaskRunner{
		settings,
		&AgentHostGateway{
			ExecutablesDir: filepath.Join(evgHome, settings.ClientBinariesDir),
		},
	}
}

// agentStartData is the information needed to start an agent on a host
type agentStartData struct {
	Host     host.Host
	Settings *evergreen.Settings
}

func (tr *TaskRunner) Run() error {
	// Find all hosts that are running and have a LCT (last communication time)
	// of 0 or ones that haven't been communicated in MaxLCT time.
	// These are the hosts that need to have agents dispatched
	freeHosts, err := host.Find(host.NeedsNewAgent(time.Now()))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	grip.Info(message.Fields{
		"runner":     RunnerName,
		"free_hosts": len(freeHosts),
		"message":    "found hosts without agents",
	})

	freeHostChan := make(chan agentStartData, len(freeHosts))

	// put all of the information needed about the host in a channel in a shuffled order
	for _, r := range rand.Perm(len(freeHosts)) {
		freeHostChan <- agentStartData{
			Host:     freeHosts[r],
			Settings: tr.Settings,
		}
	}
	// close the free hosts channel
	close(freeHostChan)
	wg := sync.WaitGroup{}
	workers := runtime.NumCPU() * 2
	wg.Add(workers)

	// for each worker create a new goroutine
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for input := range freeHostChan {
				errorCollector.add(&input.Host, tr.StartAgentOnHost(ctx, input.Settings, input.Host))
			}
		}()
	}
	wg.Wait()

	return errorCollector.report()
}
