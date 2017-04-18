package taskrunner

import (
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TODO: take out task queue finder and host finder once transition is complete
type TaskRunner struct {
	*evergreen.Settings
	HostFinder
	TaskQueueFinder
	HostGateway
}

func NewTaskRunner(settings *evergreen.Settings) *TaskRunner {
	// get mci home, and set the source and destination for the agent
	// executables
	evgHome := evergreen.FindEvergreenHome()

	return &TaskRunner{
		settings,
		&DBHostFinder{},
		&DBTaskQueueFinder{},
		&AgentHostGateway{
			ExecutablesDir: filepath.Join(evgHome, settings.AgentExecutablesDir),
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
	freeHosts, err := host.Find(host.ByRunningWithTimedOutLCT(time.Now()))
	if err != nil {
		return err
	}

	grip.Infof("Found %d hosts that need agents dispatched", len(freeHosts))

	freeHostChan := make(chan agentStartData, len(freeHosts))

	// put all of the information needed about the host in a channel
	for _, h := range freeHosts {
		freeHostChan <- agentStartData{
			Host:     h,
			Settings: tr.Settings,
		}
	}
	// close the free hosts channel
	close(freeHostChan)
	wg := sync.WaitGroup{}
	workers := runtime.NumCPU()
	wg.Add(workers)

	catcher := grip.NewCatcher()
	// for each worker create a new goroutine
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for input := range freeHostChan {
				catcher.Add(errors.Wrapf(tr.StartAgentOnHost(input.Settings, input.Host),
					"problem starting agent on host %s", input.Host))
			}
		}()
	}
	wg.Wait()
	return catcher.Resolve()
}
