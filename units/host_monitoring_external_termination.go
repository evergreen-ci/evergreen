package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostMonitorExternalStateCheckName = "host-monitoring-external-state-check"

func init() {
	registry.AddJobType(hostMonitorExternalStateCheckName, func() amboy.Job {
		return makeHostMonitorExternalState()
	})
}

type hostMonitorExternalStateCheckJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"base" json:"base" yaml:"base"`

	// cache
	host *host.Host
	env  evergreen.Environment
}

func makeHostMonitorExternalState() *hostMonitorExternalStateCheckJob {
	j := &hostMonitorExternalStateCheckJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostMonitorExternalStateCheckName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostMonitorExternalStateJob(env evergreen.Environment, h *host.Host, id string) amboy.Job {
	job := makeHostMonitorExternalState()

	job.host = h
	job.HostID = h.Id

	job.SetID(fmt.Sprintf("%s.%s.%s", hostMonitorExternalStateCheckName, job.HostID, id))

	return job
}

func (j *hostMonitorExternalStateCheckJob) Run(ctx context.Context) {
	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.MonitorDisabled {
		j.AddError(errors.New("monitor is disabled"))
		return
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		} else if j.host == nil {
			j.AddError(errors.Errorf("unable to retrieve host %s", j.HostID))
			return
		}
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	cloudHost, err := cloud.GetCloudHost(ctx, j.host, settings)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting cloud host for host %s", j.host.Id))
		return
	}

	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error getting cloud status for host %s", j.HostID))
		return
	}

	switch cloudStatus {
	case cloud.StatusRunning:
		if j.host.Status != evergreen.HostRunning {
			grip.Info(message.Fields{
				"op":      hostMonitorExternalStateCheckName,
				"op_id":   j.ID(),
				"message": "found running host, with incorrect status ",
				"status":  j.host.Status,
				"host":    j.HostID,
				"distro":  j.host.Distro.Id,
			})

			j.AddError(errors.Wrapf(j.host.MarkReachable(), "error updating reachability for host %s", j.HostID))
		}
	case cloud.StatusTerminated:
		grip.Info(message.Fields{
			"op":      hostMonitorExternalStateCheckName,
			"op_id":   j.ID(),
			"message": "host terminated externally",
			"host":    j.HostID,
			"distro":  j.host.Distro.Id,
		})

		j.AddError(model.ClearAndResetStrandedTask(j.host))

		event.LogHostTerminatedExternally(j.HostID)

		// the instance was terminated from outside our control
		j.AddError(errors.Wrapf(j.host.SetTerminated(evergreen.HostExternalUserName), "error setting host %s terminated", j.HostID))
	default:
		grip.Warning(message.Fields{
			"message":      "host found with unexpected status",
			"op":           hostMonitorExternalStateCheckName,
			"op_id":        j.ID(),
			"host":         j.HostID,
			"distro":       j.host.Distro.Id,
			"host_status":  j.host.Status,
			"cloud_status": cloudStatus,
		})
	}
}
