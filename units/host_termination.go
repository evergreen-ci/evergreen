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
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const hostTerminationJobName = "host-termination-job"

func init() {
	registry.AddJobType(hostTerminationJobName, func() amboy.Job {
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
				Name:    hostTerminationJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostTerminationJob(env evergreen.Environment, h *host.Host, terminateIfBusy bool, reason string) amboy.Job {
	j := makeHostTerminationJob()
	j.host = h
	j.HostID = h.Id
	j.env = env
	j.TerminateIfBusy = terminateIfBusy
	j.Reason = reason
	j.SetPriority(2)
	ts := utility.RoundPartOfHour(2).Format(TSFormat)
	j.SetID(fmt.Sprintf("%s.%s.%s", hostTerminationJobName, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", hostTerminationJobName, h.Id)})
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
	grip.InfoWhen(j.host.Status == evergreen.HostTerminated,
		message.Fields{
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "terminating host already marked terminated in the db",
			"theory":   "job collision",
			"outcome":  "investigate-spurious-host-termination",
		})

	// host may still be an intent host
	if j.host.Status == evergreen.HostUninitialized {
		if err = j.host.Terminate(evergreen.User, j.Reason); err != nil {
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
	if err = j.host.SetDecommissioned(evergreen.User, "host will be terminated shortly, preventing task dispatch"); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem decommissioning host",
		}))
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

	// convert the host to a cloud host
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
	if err != nil {
		err = errors.Wrapf(err, "error getting cloud host for %s", j.HostID)
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem getting cloud host instance, aborting termination",
		}))
		return
	}

	cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
	if err != nil {
		// other problem getting cloud status
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "problem getting cloud host instance status",
		}))

		if !utility.StringSliceContains(evergreen.UpHostStatus, prevStatus) {
			if err := j.host.Terminate(evergreen.User, "unable to get cloud status for host"); err != nil {
				j.AddError(err)
			}
			return
		}

	}

	if cloudStatus == cloud.StatusTerminated {
		j.AddError(errors.New("host is already terminated"))
		grip.Warning(message.Fields{
			"host_id":  j.host.Id,
			"provider": j.host.Distro.Provider,
			"job_type": j.Type().Name,
			"job":      j.ID(),
			"message":  "attempted to terminated an already terminated host",
			"theory":   "external termination",
		})
		if err := j.host.Terminate(evergreen.User, "cloud provider indicated host was terminated"); err != nil {
			j.AddError(errors.Wrap(err, "problem terminating host in db"))
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem terminating host in db",
			}))
		}

		return
	}

	if err := cloudHost.TerminateInstance(ctx, evergreen.User, j.Reason); err != nil {
		j.AddError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "problem terminating host",
			"host_id":  j.host.Id,
			"job":      j.ID(),
			"job_type": j.Type().Name,
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
