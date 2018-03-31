package monitor

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// responsible for running regular monitoring of hosts
type HostMonitor struct {
	// will be used to determine what hosts need to be terminated
	flaggingFuncs []hostFlagger
}

// run through the list of host flagging functions, finding all hosts that
// need to be terminated and terminating them
func (hm *HostMonitor) CleanupHosts(ctx context.Context, distros []distro.Distro, settings *evergreen.Settings) error {
	env := evergreen.GetEnvironment()
	queue := env.RemoteQueue()
	catcher := grip.NewBasicCatcher()

	for _, flagger := range hm.flaggingFuncs {
		if ctx.Err() != nil {
			catcher.Add(errors.New("host monitor canceled"))
			break
		}

		// find the next batch of hosts to terminate
		flaggedHosts, err := flagger.hostFlaggingFunc(ctx, distros, settings)

		// continuing on error so that one wonky flagging function doesn't
		// stop others from running
		if err != nil {
			catcher.Add(errors.Wrapf(err, "error flagging [%s] hosts to be terminated", flagger.Reason))
			continue
		}

		for _, h := range flaggedHosts {
			if ctx.Err() != nil {
				catcher.Add(errors.New("host monitor canceled"))
				break
			}

			catcher.Add(errors.WithStack(queue.Put(units.NewHostTerminationJob(h))))
		}
	}

	return catcher.Resolve()
}
