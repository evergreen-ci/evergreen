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
		j.AddError(errors.Wrap(err, "getting admin settings"))
		return
	}
	if flags.MonitorDisabled {
		j.AddError(errors.New("monitor is disabled"))
		return
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
			return
		} else if j.host == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.HostID))
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
		return false, errors.Wrapf(err, "getting cloud host for host '%s'", h.Id)
	}
	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "getting cloud status for host '%s'", h.Id)
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
			return false, errors.Wrapf(h.MarkReachable(), "updating reachability for host '%s'", h.Id)
		}
		return false, nil
	case cloud.StatusStopping, cloud.StatusStopped, cloud.StatusTerminated, cloud.StatusNonExistent:
		// The cloud provider could report the host as nonexistent if it's been
		// terminated for so long that the provider has no information about the
		// host anymore. Therefore, a nonexistent host is equivalent to one
		// that's terminated.
		isTerminated := cloudStatus == cloud.StatusTerminated || cloudStatus == cloud.StatusNonExistent

		// Avoid accidentally terminating non-agent hosts that are stopped (e.g.
		// spawn hosts).
		if !isTerminated && (h.UserHost || h.StartedBy != evergreen.User) {
			return false, errors.New("non-agent host is not already terminated and should not be terminated")
		}

		if err := handleTerminatedHostSpawnedByTask(h); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "handling prematurely terminated task host",
				"cloud_status": cloudStatus.String(),
				"host_id":      h.Id,
				"task_id":      h.StartedBy,
			}))
		}

		event.LogHostTerminatedExternally(h.Id, h.Status)
		grip.Info(message.Fields{
			"message":      "host terminated externally",
			"operation":    "handleExternallyTerminatedHost",
			"host_id":      h.Id,
			"host_tag":     h.Tag,
			"distro":       h.Distro.Id,
			"provider":     h.Provider,
			"status":       h.Status,
			"cloud_status": cloudStatus.String(),
		})

		err = amboy.EnqueueUniqueJob(ctx, env.RemoteQueue(), NewHostTerminationJob(env, h, HostTerminationOptions{
			TerminateIfBusy:          true,
			TerminationReason:        fmt.Sprintf("host was found in state '%s'", cloudStatus.String()),
			SkipCloudHostTermination: isTerminated,
		}))
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

// handleTerminatedHostSpawnedByTask re-creates a new intent host if possible when this host.create host fails.
// If it cannot create a new host, it will populate the reason that host.create failed.
func handleTerminatedHostSpawnedByTask(h *host.Host) error {
	if !h.SpawnOptions.SpawnedByTask {
		return nil
	}

	intent, err := insertNewHostForTask(h)
	if err != nil || intent == nil {
		grip.Info(message.Fields{
			"message":        "host was externally terminated",
			"action":         "adding host create details",
			"host_id":        h.Id,
			"task_id":        h.SpawnOptions.TaskID,
			"task_execution": h.SpawnOptions.TaskExecutionNumber,
		})

		catcher := grip.NewBasicCatcher()
		catcher.Wrap(err, "inserting new host for task")
		catcher.Wrap(task.AddHostCreateDetails(h.SpawnOptions.TaskID, h.Id, h.SpawnOptions.TaskExecutionNumber, errors.New("host was externally terminated")), "adding host create details")
		return catcher.Resolve()
	}

	grip.Info(message.Fields{
		"message":              "inserted a host intent to replace a terminated host for a task",
		"original_host_id":     h.Id,
		"original_host_status": h.Status,
		"new_host_id":          intent.Id,
		"task_id":              h.SpawnOptions.TaskID,
		"task_execution":       h.SpawnOptions.TaskExecutionNumber,
	})

	return nil
}

func insertNewHostForTask(h *host.Host) (*host.Host, error) {
	if h.SpawnOptions.Respawns == 0 {
		return nil, nil
	}

	t, err := task.FindOneIdAndExecution(h.SpawnOptions.TaskID, h.SpawnOptions.TaskExecutionNumber)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task '%s' with execution %d for host '%s'", h.SpawnOptions.TaskID, h.SpawnOptions.TaskExecutionNumber, h.Id)
	}
	if t == nil {
		return nil, errors.Errorf("host '%s' was created by task '%s' execution %d that does not exist", h.Id, h.SpawnOptions.TaskID, h.SpawnOptions.TaskExecutionNumber)
	}

	if h.Status != evergreen.HostStarting || t.Status != evergreen.TaskStarted || t.Aborted {
		return nil, nil
	}

	opts := h.GetCreateOptions()
	opts.SpawnOptions.Respawns--
	intent := host.NewIntent(opts)
	return intent, errors.Wrap(intent.Insert(), "inserting intent")
}
