package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	hostDrawdownJobName = "host-drawdown"

	// if we need to drawdown hosts, we want to catch as many hosts as we can that are between jobs
	idleTimeDrawdownCutoff = 5 * time.Second
)

func init() {
	registry.AddJobType(hostDrawdownJobName, func() amboy.Job {
		return makeHostDrawdownJob()
	})
}

type DrawdownInfo struct {
	DistroID     string
	NewCapTarget int
}

type hostDrawdownJob struct {
	idleHostJob

	DrawdownInfo DrawdownInfo
}

func makeHostDrawdownJob() *hostDrawdownJob {
	j := &hostDrawdownJob{
		idleHostJob{
			Base: job.Base{
				JobType: amboy.JobType{
					Name:    hostDrawdownJobName,
					Version: 0,
				},
			},
		},
		DrawdownInfo{},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(2)

	return j
}

func NewHostDrawdownJob(env evergreen.Environment, drawdownInfo DrawdownInfo, id string) amboy.Job {
	j := makeHostDrawdownJob()
	j.DrawdownInfo = drawdownInfo
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s", hostDrawdownJobName, id))
	return j
}

func (j *hostDrawdownJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	// get currently existing hosts, in case some hosts have already been terminated elsewhere
	existingHostCount, err := host.CountRunningHosts(j.DrawdownInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrap(err, "database error getting existing hosts by Distro.Id"))
		return
	}
	idleHosts, err := host.IdleHostsWithDistroID(j.DrawdownInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrap(err, "database error getting idle hosts by Distro.Id"))
		return
	}
	drawdownTarget := existingHostCount - j.DrawdownInfo.NewCapTarget

	for _, idleHost := range idleHosts {
		if drawdownTarget <= 0 {
			break
		}
		j.AddError(j.checkAndTerminateHost(ctx, &idleHost, &drawdownTarget))
	}

	grip.Info(message.Fields{
		"id":                   j.ID(),
		"job_type":             hostDrawdownJobName,
		"distro_id":            j.DrawdownInfo.DistroID,
		"num_idle_hosts":       len(idleHosts),
		"num_terminated_hosts": j.Terminated,
		"terminated_hosts":     j.TerminatedHosts,
	})
}

func (j *hostDrawdownJob) checkAndTerminateHost(ctx context.Context, h *host.Host, drawdownTarget *int) error {

	_, idleTime, err := j.getTimesFromHost(ctx, h)

	if err != nil {
		return errors.Wrapf(err, "error getting communication and idle time from '%s'", h.Id)
	}

	idleThreshold := idleTimeDrawdownCutoff
	if h.RunningTaskGroup != "" {
		idleThreshold = idleTaskGroupHostCutoff
	}

	if idleTime > idleThreshold {
		(*drawdownTarget)--
		j.Terminated++
		j.TerminatedHosts = append(j.TerminatedHosts, h.Id)
		if err = h.SetDecommissioned(evergreen.User, "host decommissioned due to overallocation"); err != nil {
			return errors.Wrapf(err, "problem decommissioning host %s", h.Id)
		}
		return nil
	}
	return nil
}
