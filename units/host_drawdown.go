package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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
	DistroID     string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
	NewCapTarget int    `bson:"new_cap_target" json:"new_cap_target" yaml:"new_cap_target"`
}

type hostDrawdownJob struct {
	job.Base        `bson:"metadata" json:"metadata" yaml:"metadata"`
	Terminated      int      `bson:"terminated" json:"terminated" yaml:"terminated"`
	TerminatedHosts []string `bson:"terminated_hosts" json:"terminated_hosts" yaml:"terminated_hosts"`

	env  evergreen.Environment
	host *host.Host

	DrawdownInfo DrawdownInfo `bson:"drawdowninfo" json:"drawdowninfo" yaml:"drawdowninfo"`
}

func makeHostDrawdownJob() *hostDrawdownJob {
	j := &hostDrawdownJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostDrawdownJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	j.SetPriority(2)

	return j
}

func NewHostDrawdownJob(env evergreen.Environment, drawdownInfo DrawdownInfo, ts string) amboy.Job {
	j := makeHostDrawdownJob()
	j.DrawdownInfo = drawdownInfo
	j.env = env
	jobID := fmt.Sprintf("%s.%s.%s", hostDrawdownJobName, drawdownInfo.DistroID, ts)
	j.SetID(jobID)
	j.SetScopes([]string{jobID})
	return j
}

func (j *hostDrawdownJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
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
		err = j.checkAndTerminateHost(ctx, &idleHost, &drawdownTarget)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"id":             j.ID(),
				"distro_id":      j.DrawdownInfo.DistroID,
				"idle_host_list": idleHosts,
				"message":        "terminate host drawdown error",
			}))
		}
	}

	grip.Info(message.Fields{
		"id":                   j.ID(),
		"job_type":             hostDrawdownJobName,
		"distro_id":            j.DrawdownInfo.DistroID,
		"new_cap_target":       j.DrawdownInfo.NewCapTarget,
		"existing_host_count":  existingHostCount,
		"num_idle_hosts":       len(idleHosts),
		"num_terminated_hosts": j.Terminated,
		"terminated_hosts":     j.TerminatedHosts,
	})
}

func (j *hostDrawdownJob) checkAndTerminateHost(ctx context.Context, h *host.Host, drawdownTarget *int) error {

	exitEarly, err := checkTerminationExemptions(ctx, h, j.env, j.Type().Name, j.ID())
	if exitEarly || err != nil {
		return err
	}

	// don't drawdown hosts that are running task groups
	if h.RunningTaskGroup != "" {
		t, err := task.FindOneId(h.RunningTask)
		if err != nil {
			return errors.Wrap(err, "error finding running task")
		}
		if t == nil {
			return errors.Errorf("could not find running task '%s'", h.RunningTask)
		}
		if t.IsPartOfSingleHostTaskGroup() {
			return nil
		}
	} else if h.LastGroup != "" && h.RunningTask == "" { // if we're currently running a task not in a group, then we already know the group is finished running.
		t, err := task.FindOneId(h.LastTask)
		if err != nil {
			return errors.Wrap(err, "error finding last task")
		}
		if t == nil {
			return errors.Errorf("could not find last task '%s'", h.LastTask)
		}
		if t.IsPartOfSingleHostTaskGroup() && t.Status == evergreen.TaskSucceeded {
			return nil
		}
	}

	idleTime := h.IdleTime()

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
