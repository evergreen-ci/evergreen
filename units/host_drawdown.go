package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	hostDrawdownJobName = "host-drawdown"

	// if we need to drawdown hosts, we want to catch as many hosts as we can that are between jobs
	idleTimeDrawdownCutoff      = 5 * time.Second
	idleTaskGroupDrawdownCutoff = 10 * time.Minute
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
	job.Base            `bson:"metadata" json:"metadata" yaml:"metadata"`
	Decommissioned      int      `bson:"decommissioned" json:"decommissioned" yaml:"decommissioned"`
	DecommissionedHosts []string `bson:"decommissioned_hosts" json:"decommissioned_hosts" yaml:"decommissioned_hosts"`

	env evergreen.Environment

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

	existingHostCount, err := host.CountHostsCanOrWillRunTasksInDistro(ctx, j.DrawdownInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "counting running hosts in distro '%s'", j.DrawdownInfo.DistroID))
		return
	}
	idleHosts, err := host.IdleHostsWithDistroID(ctx, j.DrawdownInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding idle hosts in distro '%s'", j.DrawdownInfo.DistroID))
		return
	}
	taskQueue, err := model.FindDistroTaskQueue(ctx, j.DrawdownInfo.DistroID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding task queue for distro '%s'", j.DrawdownInfo.DistroID))
	}
	drawdownTarget := existingHostCount - j.DrawdownInfo.NewCapTarget

	for _, idleHost := range idleHosts {
		if drawdownTarget <= 0 {
			break
		}
		err = j.checkAndDecommission(ctx, &idleHost, taskQueue, &drawdownTarget)
		grip.Error(message.WrapError(err, message.Fields{
			"id":        j.ID(),
			"distro_id": j.DrawdownInfo.DistroID,
			"host":      idleHost.Id,
			"message":   "decommission host drawdown error",
		}))
	}

	grip.Info(message.Fields{
		"id":                       j.ID(),
		"job_type":                 hostDrawdownJobName,
		"distro_id":                j.DrawdownInfo.DistroID,
		"new_cap_target":           j.DrawdownInfo.NewCapTarget,
		"existing_host_count":      existingHostCount,
		"num_idle_hosts":           len(idleHosts),
		"num_decommissioned_hosts": j.Decommissioned,
		"decommissioned_hosts":     j.DecommissionedHosts,
	})
}

func (j *hostDrawdownJob) checkAndDecommission(ctx context.Context, h *host.Host, taskQueue model.TaskQueue, drawdownTarget *int) error {
	exitEarly, err := checkTerminationExemptions(ctx, h, j.env, j.Type().Name, j.ID())
	if exitEarly || err != nil {
		return err
	}

	// Don't drawdown hosts that are currently in the middle of tearing down a task group.
	if h.IsTearingDown() && !h.TeardownTimeExceededMax() {
		return nil
	}

	// Don't drawdown hosts that are running single host task groups.
	isRunningSingleHostTaskGroup, err := isAssignedSingleHostTaskGroup(ctx, h)
	if err != nil {
		return errors.Wrap(err, "checking if host is running single host task group")
	}
	if isRunningSingleHostTaskGroup {
		return nil
	}

	idleTime := h.IdleTime()

	idleThreshold := idleTimeDrawdownCutoff
	if h.RunningTaskGroup != "" {
		idleThreshold = idleTaskGroupDrawdownCutoff
	}

	if !h.LastTaskCompletedTime.IsZero() && taskQueue.Length() > 0 {
		idleThreshold = h.Distro.HostAllocatorSettings.AcceptableHostIdleTime

	}

	if idleTime > idleThreshold {
		(*drawdownTarget)--
		if err = h.SetDecommissioned(ctx, evergreen.User, false, "host decommissioned due to overallocation"); err != nil {
			return errors.Wrapf(err, "decommissioning host '%s'", h.Id)
		}
		j.Decommissioned++
		j.DecommissionedHosts = append(j.DecommissionedHosts, h.Id)
		return nil
	}
	return nil
}
