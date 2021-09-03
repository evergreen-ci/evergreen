package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
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

	job.env = env

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

	_, err = handleExternallyTerminatedHost(ctx, j.ID(), j.env, j.host)
	j.AddError(err)
}

// handleExternallyTerminatedHost will check if a host from a dynamic provider
// has been terminated or stopped by a source external to Evergreen itself. If
// so, clean up the host. Returns true if the host has been externally terminated
// or stopped.
func handleExternallyTerminatedHost(ctx context.Context, id string, env evergreen.Environment, h *host.Host) (bool, error) {
	if h.Provider == evergreen.ProviderNameStatic {
		return false, nil
	}

	cloudHost, err := cloud.GetCloudHost(ctx, h, env)
	if err != nil {
		return false, errors.Wrapf(err, "error getting cloud host for host %s", h.Id)
	}
	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "error getting cloud status for host %s", h.Id)
	}

	switch cloudStatus {
	case cloud.StatusRunning:
		userDataProvisioning := h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData && h.Status == evergreen.HostStarting
		if h.Status != evergreen.HostRunning && !userDataProvisioning {
			grip.Info(message.Fields{
				"op_id":   id,
				"message": "found running host with incorrect status",
				"status":  h.Status,
				"host_id": h.Id,
				"distro":  h.Distro.Id,
			})
			return false, errors.Wrapf(h.MarkReachable(), "error updating reachability for host %s", h.Id)
		}
		return false, nil
	case cloud.StatusStopping, cloud.StatusStopped, cloud.StatusTerminated:
		// Avoid accidentally terminating non-agent hosts that are stopped (e.g.
		// spawn hosts).
		if cloudStatus != cloud.StatusTerminated && (h.UserHost || h.StartedBy != evergreen.User) {
			return false, errors.New("non-agent host is not already terminated and should not be terminated")
		}
		if h.SpawnOptions.SpawnedByTask {
			if err := task.AddHostCreateDetails(h.SpawnOptions.TaskID, h.Id, h.SpawnOptions.TaskExecutionNumber, errors.New("host was externally terminated")); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":      "error adding host create error details",
					"cloud_status": cloudStatus.String(),
					"host_id":      h.Id,
					"task_id":      h.StartedBy,
				}))
			}
		}
		event.LogHostTerminatedExternally(h.Id, h.Status)

		err = amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewHostTerminationJob(env, h, true, fmt.Sprintf("host was found in %s state", cloudStatus.String())))
		grip.Error(message.WrapError(err, message.Fields{
			"message":      "could not enqueue job to terminate externally-modified host",
			"cloud_status": cloudStatus.String(),
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"op_id":        id,
		}))
		return true, err
	default:
		grip.Warning(message.Fields{
			"message":      "host found with unexpected status",
			"op_id":        id,
			"host_id":      h.Id,
			"distro":       h.Distro.Id,
			"host_status":  h.Status,
			"cloud_status": cloudStatus.String(),
		})
		return false, errors.Errorf("unexpected host status '%s'", cloudStatus)
	}
}
