package taskrunner

import (
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const distroSleep = 10 * time.Second

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

func (tr *TaskRunner) Run() error {
	// Find all hosts that are running and have a LCT (last communication time)
	// of 0 or ones that haven't been communicated in MaxLCT time.
	// These are the hosts that need to have agents dispatched
	freeHosts, err := host.Find(host.ByRunningWithTimedOutLCT(time.Now()))
	if err != nil {
		return errors.Wrap(err, "error finding free hosts")
	}

	// update host's agent revision
	if err := h.SetAgentRevision(revision); err != nil {
		return errors.Wrapf(err, "error setting new agent revision %s on host %s", revision, h.Id)
	}

	return nil
}
