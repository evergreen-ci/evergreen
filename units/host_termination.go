package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const HostTerminationJobName = "host-termination-job"

func init() {
	registry.AddJobType(HostTerminationJobName, func() amboy.Job {
		return makeHostTerminationJob()
	})
}

type hostTerminationJob struct {
	HostID          string `bson:"host_id" json:"host_id"`
	Reason          string `bson:"reason,omitempty" json:"reason,omitempty"`
	TerminateIfBusy bool   `bson:"terminate_if_busy" json:"terminate_if_busy"`
	job.Base        `bson:"metadata" json:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeHostTerminationJob() *hostTerminationJob {
	j := &hostTerminationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    HostTerminationJobName,
				Version: 0,
			},
		},
	}
	return j
}

func NewHostTerminationJob(env evergreen.Environment, h *host.Host, terminateIfBusy bool, reason string) amboy.Job {
	j := makeHostTerminationJob()
	j.host = h
	j.HostID = h.Id
	j.env = env
	j.TerminateIfBusy = terminateIfBusy
	j.Reason = reason
	ts := utility.RoundPartOfHour(2).Format(TSFormat)
	j.SetID(fmt.Sprintf("%s.%s.%s", HostTerminationJobName, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", HostTerminationJobName, h.Id)})
	j.SetEnqueueAllScopes(true)

	return j
}

func (j *hostTerminationJob) Run(ctx context.Context) {
	var err error
	defer j.MarkComplete()

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "error finding host '%s'", j.HostID))
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if !j.host.IsEphemeral() {
		grip.Notice(message.Fields{
			"job":      j.ID(),
			"host_id":  j.HostID,
			"job_type": j.Type().Name,
			"status":   j.host.Status,
			"provider": j.host.Distro.Provider,
			"message":  "host termination for a non-spawnable distro",
			"cause":    "programmer error",
		})
		return
	}

	if j.host.HasContainers && !j.TerminateIfBusy {
		var idle bool
		idle, err = j.host.IsIdleParent()
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem checking if host is an idle parent",
				"host_id": j.host.Id,
				"job":     j.ID(),
			}))
		}
		if !idle {
			grip.Info(message.Fields{
				"job":      j.ID(),
				"host_id":  j.HostID,
				"job_type": j.Type().Name,
				"status":   j.host.Status,
				"provider": j.host.Distro.Provider,
				"reason":   j.Reason,
				"message":  "attempted to terminate a non-idle parent",
			})
			return
		}
	}

	if err = j.host.DeleteJasperCredentials(ctx, j.env); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem deleting Jasper credentials",
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job":      j.ID(),
		}))
		return
	}

	// we may be running these jobs on hosts that are already
	// terminated.
	grip.InfoWhen(j.host.Status == evergreen.HostTerminated, message.Fields{
		"host_id":  j.host.Id,
		"provider": j.host.Distro.Provider,
		"job_type": j.Type().Name,
		"job":      j.ID(),
		"message":  "terminating host already marked terminated in the db",
	})

	// Intent hosts are just marked terminated in the DB without further
	// processing, because it's not possible for an intent host to run tasks,
	// nor is the intent host associated with any instance in the cloud that
	// we're aware of.
	switch j.host.Status {
	case evergreen.HostUninitialized, evergreen.HostBuildingFailed:
		if err := j.host.Terminate(evergreen.User, j.Reason); err != nil {
			j.AddError(errors.Wrap(err, "terminating intent host in DB"))
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "problem terminating intent host in DB",
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
			}))
		}
		return
	case evergreen.HostTerminated:
		if host.IsIntentHostId(j.host.Id) {
			return
		}
	default:
		grip.WarningWhen(host.IsIntentHostId(j.host.Id), message.Fields{
			"message":     "Intent host has a status that should not be possible when preparing to terminate it. This is potentially a host lifecycle logical error.",
			"host_id":     j.host.Id,
			"host_status": j.host.Status,
			"provider":    j.host.Distro.Provider,
			"job_type":    j.Type().Name,
			"job":         j.ID(),
		})
	}

	// clear the running task of the host in case one has been assigned.
	if j.host.RunningTask != "" {
		if j.TerminateIfBusy {
			grip.Warning(message.Fields{
				"message":  "Host has running task; clearing before terminating",
				"job":      j.ID(),
				"job_type": j.Type().Name,
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"task":     j.host.RunningTask,
			})

			j.AddError(model.ClearAndResetStrandedTask(j.host))
		} else {
			return
		}
	} else {
		// Consider if the host is in between running a single-host task group
		if j.host.LastGroup != "" {
			lastTask, err := task.FindOneId(j.host.LastTask)
			if err != nil {
				j.AddError(errors.Wrapf(err, "error finding last task '%s'", j.host.LastTask))
			}
			// Only try to restart the task group if it was successful and should have continued executing.
			if lastTask != nil && lastTask.IsPartOfSingleHostTaskGroup() && lastTask.Status == evergreen.TaskSucceeded {
				tasks, err := task.FindTaskGroupFromBuild(lastTask.BuildId, lastTask.TaskGroup)
				if err != nil {
					j.AddError(errors.Wrapf(err, "can't get task group for task '%s'", lastTask.Id))
					return
				}
				if len(tasks) == 0 {
					j.AddError(errors.Errorf("no tasks found in task group for task '%s'", lastTask.Id))
					return
				}
				if tasks[len(tasks)-1].Id != lastTask.Id {
					// If we aren't looking at the last task in the group, then we should mark the whole thing for restart,
					// because later tasks in the group need to run on the same host as the earlier ones.
					j.AddError(errors.Wrap(model.TryResetTask(lastTask.Id, evergreen.User, evergreen.MonitorPackage, nil), "problem resetting task"))
				}
			}
		}

	}
	// set host as decommissioned in DB so no new task will be assigned
	prevStatus := j.host.Status
	if prevStatus != evergreen.HostTerminated {
		if err = j.host.SetDecommissioned(evergreen.User, "host will be terminated shortly, preventing task dispatch"); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem decommissioning host",
			}))
		}
	}

	j.host, err = host.FindOneId(j.HostID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "error finding host '%s'", j.HostID))
		return
	}
	if j.host == nil {
		j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
		return
	}

	// check if running task has been assigned since status changed
	if j.host.RunningTask != "" {
		if j.TerminateIfBusy {
			grip.Warning(message.Fields{
				"message":  "Host has running task; clearing before terminating",
				"job":      j.ID(),
				"job_type": j.Type().Name,
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"task":     j.host.RunningTask,
			})

			j.AddError(model.ClearAndResetStrandedTask(j.host))
		} else {
			return
		}
	}

	// terminate containers in DB if parent already terminated
	if j.host.ParentID != "" {
		var parent *host.Host
		parent, err = j.host.GetParent()
		if err != nil {
			if err.Error() != host.ErrorParentNotFound {
				j.AddError(errors.Wrapf(err, "problem finding parent of '%s'", j.host.Id))
				return
			}
		}
		if parent == nil || parent.Status == evergreen.HostTerminated {
			if err = j.host.Terminate(evergreen.User, "parent was already terminated"); err != nil {
				j.AddError(errors.Wrap(err, "problem terminating container in db"))
				grip.Error(message.WrapError(err, message.Fields{
					"host_id":  j.host.Id,
					"provider": j.host.Distro.Provider,
					"job_type": j.Type().Name,
					"job":      j.ID(),
					"message":  "problem terminating container in db",
				}))
			}
			return
		}
	} else if prevStatus == evergreen.HostBuilding {
		// If the host is not a container and is building, this means the host is an intent
		// host, and should be terminated in the database, and not in the cloud manager.
		if err = j.host.Terminate(evergreen.User, "host was never started"); err != nil {
			// It is possible that the provisioning-create-host job has removed the
			// intent host from the database before this job got to it. If so, there is
			// nothing to terminate with a cloud manager, since if there is a
			// cloud-managed host, it has a different ID.
			if adb.ResultsNotFound(err) {
				return
			}
			j.AddError(errors.Wrap(err, "problem terminating intent host in db"))
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem terminating intent host in db",
			}))
		}

		return
	}

	idleTimeStartsAt := j.host.LastTaskCompletedTime
	if idleTimeStartsAt.IsZero() || idleTimeStartsAt == utility.ZeroTime {
		idleTimeStartsAt = j.host.StartTime
	}

	if err := j.checkAndTerminateCloudHost(ctx, prevStatus); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "failed to check and terminate cloud host",
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
		}))
		return
	}

	grip.Info(message.Fields{
		"message":           "host successfully terminated",
		"host_id":           j.host.Id,
		"distro":            j.host.Distro.Id,
		"job":               j.ID(),
		"reason":            j.Reason,
		"total_idle_secs":   j.host.TotalIdleTime.Seconds(),
		"total_uptime_secs": j.host.TerminationTime.Sub(j.host.CreationTime).Seconds(),
		"termination_time":  j.host.TerminationTime,
		"creation_time":     j.host.CreationTime,
	})

	if utility.StringSliceContains(evergreen.ProvisioningHostStatus, prevStatus) && j.host.TaskCount == 0 {
		event.LogHostProvisionFailed(j.HostID, fmt.Sprintf("terminating host in status '%s'", prevStatus))
		grip.Info(message.Fields{
			"message":     "provisioning failure",
			"status":      prevStatus,
			"host_id":     j.HostID,
			"distro":      j.host.Distro.Id,
			"uptime_secs": time.Since(j.host.StartTime).Seconds(),
			"provider":    j.host.Provider,
			"spawn_host":  j.host.StartedBy != evergreen.User,
		})
	}
}

// checkAndTerminateCloudHost checks if the host is still up according to the
// cloud provider. If so, it will attempt to terminate it in the cloud and mark
// the host as terminated. If not, it will just mark the host as terminated.
func (j *hostTerminationJob) checkAndTerminateCloudHost(ctx context.Context, oldStatus string) error {
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
	if err != nil {
		return errors.Wrapf(err, "getting cloud host for host '%s'", j.HostID)
	}

	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		if utility.IsContextError(errors.Cause(err)) {
			return errors.Wrap(err, "checking cloud host status")
		}
		if err == cloud.ErrInstanceNotFound {
			return j.host.Terminate(evergreen.User, "corresponding cloud host instance does not exist")
		}

		catcher := grip.NewBasicCatcher()
		catcher.Add(errors.Wrap(err, "getting cloud host instance status"))
		if !utility.StringSliceContains(evergreen.UpHostStatus, oldStatus) {
			catcher.Wrap(j.host.Terminate(evergreen.User, "unable to get cloud status for host"), "marking host as terminated")
		}
		return catcher.Resolve()
	}

	if cloudStatus == cloud.StatusTerminated {
		grip.Warning(message.Fields{
			"message":  "attempted to terminate an already terminated host",
			"theory":   "external termination",
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
		})
		catcher := grip.NewBasicCatcher()
		catcher.New("host is already terminated in the cloud")
		catcher.Wrap(j.host.Terminate(evergreen.User, "cloud provider indicated that host was already terminated"), "marking host as terminated")
		return catcher.Resolve()
	}

	if err := cloudHost.TerminateInstance(ctx, evergreen.User, j.Reason); err != nil {
		return errors.Wrap(err, "terminating cloud host")
	}

	return nil
}
