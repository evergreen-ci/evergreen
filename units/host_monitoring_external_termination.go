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

	_, err = HandleExternallyTerminatedHost(ctx, j.ID(), j.env, j.host)
	j.AddError(err)
}

// HandleExternallyTerminatedHost will check if a host from a dynamic provider has been termimated
// and clean up the host if it has. Returns true if the host has been externally terminated
func HandleExternallyTerminatedHost(ctx context.Context, id string, env evergreen.Environment, h *host.Host) (bool, error) {
	if h.Provider == evergreen.ProviderNameStatic {
		return false, nil
	}

	settings := env.Settings()
	cloudHost, err := cloud.GetCloudHost(ctx, h, settings)
	if err != nil {
		return false, errors.Wrapf(err, "error getting cloud host for host %s", h.Id)
	}
	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "error getting cloud status for host %s", h.Id)
	}

	switch cloudStatus {
	case cloud.StatusRunning:
		if h.Status != evergreen.HostRunning {
			grip.Info(message.Fields{
				"op_id":   id,
				"message": "found running host with incorrect status",
				"status":  h.Status,
				"host":    h.Id,
				"distro":  h.Distro.Id,
			})
			return false, errors.Wrapf(h.MarkReachable(), "error updating reachability for host %s", h.Id)
		}
		return false, nil
	case cloud.StatusTerminated:
		grip.Info(message.Fields{
			"op_id":   id,
			"message": "host terminated externally",
			"host":    h.Id,
			"distro":  h.Distro.Id,
		})

		event.LogHostTerminatedExternally(h.Id)

		// the instance was terminated from outside our control
		catcher := grip.NewBasicCatcher()
		err = h.SetTerminated(evergreen.HostExternalUserName)
		catcher.Add(err)
		grip.Error(message.WrapError(err, message.Fields{
			"op_id":   id,
			"message": "error setting host status to terminated in db",
			"host":    h.Id,
			"distro":  h.Distro.Id,
		}))

		err = model.ClearAndResetStrandedTask(h)
		catcher.Add(errors.Wrap(err, "can't clear stranded tasks"))
		grip.Error(message.WrapError(err, message.Fields{
			"op_id":   id,
			"message": "can't clear stranded tasks",
			"host":    h.Id,
			"distro":  h.Distro.Id,
		}))

		return true, catcher.Resolve()
	default:
		grip.Warning(message.Fields{
			"message":      "host found with unexpected status",
			"op_id":        id,
			"host":         h.Id,
			"distro":       h.Distro.Id,
			"host_status":  h.Status,
			"cloud_status": cloudStatus,
		})
		return false, errors.Errorf("unexpected host status '%s'", cloudStatus)
	}
}
