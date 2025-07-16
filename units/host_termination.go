package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	HostTerminationJobName         = "host-termination-job"
	hostTerminationAttributePrefix = "evergreen.host_termination"
)

func init() {
	registry.AddJobType(HostTerminationJobName, func() amboy.Job {
		return makeHostTerminationJob()
	})
}

type hostTerminationJob struct {
	HostID                 string `bson:"host_id" json:"host_id"`
	HostTerminationOptions `bson:",inline" json:"host_termination_options"`
	job.Base               `bson:"metadata" json:"metadata"`

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

// HostTerminationOptions represent options to control how a host is terminated.
type HostTerminationOptions struct {
	// TerminateIfBusy, if set, will terminate a host even if it's currently
	// running a task. Otherwise, if it's running a task, termination will
	// either refuse to terminate the host or will reset the task.
	TerminateIfBusy bool `bson:"terminate_if_busy,omitempty" json:"terminate_if_busy,omitempty"`
	// SkipCloudHostTermination, if set, will skip terminating and cleaning up
	// the host in the cloud. The host will still be marked terminated in the
	// DB.
	SkipCloudHostTermination bool `bson:"skip_cloud_host_termination,omitempty" json:"skip_cloud_host_termination,omitempty"`
	// TerminationReason is the reason that the host was terminated.
	TerminationReason string `bson:"termination_reason,omitempty" json:"termination_reason,omitempty"`
}

func NewHostTerminationJob(env evergreen.Environment, h *host.Host, opts HostTerminationOptions) amboy.Job {
	j := makeHostTerminationJob()
	j.host = h
	j.HostID = h.Id
	j.env = env
	j.HostTerminationOptions = opts
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
		j.host, err = host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
			return
		}
		if j.host == nil {
			j.AddError(errors.Errorf("could not find host '%s'", j.HostID))
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
		})
		if err := j.host.Terminate(ctx, evergreen.User, j.TerminationReason); err != nil {
			j.AddError(errors.Wrapf(err, "terminating host '%s' in DB", j.host.Id))
		}
		return
	}

	if j.host.HasContainers && !j.TerminateIfBusy {
		var idle bool
		idle, err = j.host.IsIdleParent(ctx)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "checking if host is an idle parent",
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
				"reason":   j.TerminationReason,
				"message":  "attempted to terminate a non-idle parent",
			})
			return
		}
	}

	if err = j.host.DeleteJasperCredentials(ctx, j.env); err != nil {
		j.AddError(errors.Wrapf(err, "deleting Jasper credentials for host '%s'", j.host.Id))
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
	case evergreen.HostUninitialized, evergreen.HostBuilding, evergreen.HostBuildingFailed, evergreen.HostDecommissioned:
		// If the host never successfully started, this means the host is an
		// intent host, and should be marked terminated. There's no host
		// associated with it in the cloud provider.
		if host.IsIntentHostId(j.host.Id) {
			j.AddError(errors.Wrapf(j.cleanupIntentHost(ctx), "cleaning up intent host '%s'", j.host.Id))
			return
		}
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
				"message":        "Host has running task; clearing before terminating",
				"job":            j.ID(),
				"job_type":       j.Type().Name,
				"host_id":        j.host.Id,
				"provider":       j.host.Distro.Provider,
				"task":           j.host.RunningTask,
				"task_execution": j.host.RunningTaskExecution,
			})

			j.AddError(model.ClearAndResetStrandedHostTask(ctx, j.env.Settings(), j.host))
		} else {
			return
		}
	} else {
		// Consider if the host is in between running a single-host task group
		if j.host.LastGroup != "" {
			latestTask, err := task.FindOneId(ctx, j.host.LastTask)
			if err != nil {
				j.AddError(errors.Wrapf(err, "finding last task '%s'", j.host.LastTask))
				return
			}
			// Only try to restart the task group if it was successful and should have continued executing.
			if latestTask != nil && latestTask.IsPartOfSingleHostTaskGroup() && latestTask.Status == evergreen.TaskSucceeded {
				tasks, err := task.FindTaskGroupFromBuild(ctx, latestTask.BuildId, latestTask.TaskGroup)
				if err != nil {
					j.AddError(errors.Wrapf(err, "getting task group for task '%s'", latestTask.Id))
					return
				}
				if len(tasks) == 0 {
					j.AddError(errors.Errorf("no tasks found in task group for task '%s'", latestTask.Id))
					return
				}
				// Check for the last task in the task group that we have activated, running, or completed.
				var lastActivatedTaskGroupTask task.Task
				for _, t := range tasks {
					if t.Activated {
						lastActivatedTaskGroupTask = t
					}
				}
				if lastActivatedTaskGroupTask.Id != latestTask.Id {
					// If the host was in-between running a single host task group, the group should start from scratch.
					// Since single host task groups only restart when the whole group is finished, we all block subsequent task group
					// tasks from running regardless of what status they were waiting on, so that we can force the task group to restart immediately.
					j.AddError(errors.Wrapf(model.UpdateBlockedDependencies(ctx, []task.Task{*latestTask}, true), "updating blocked dependencies for task '%s'", latestTask.Id))
					// If we aren't looking at the last task in the group, then we should mark the whole thing for restart,
					// because later tasks in the group need to run on the same host as the earlier ones.
					j.AddError(errors.Wrapf(model.TryResetTask(ctx, j.env.Settings(), latestTask.Id, evergreen.User, evergreen.MonitorPackage, nil), "resetting task '%s'", latestTask.Id))
				}
			}
		}
	}
	// set host as decommissioned in DB so no new task will be assigned
	prevStatus := j.host.Status
	if prevStatus != evergreen.HostTerminated {
		if err = j.host.SetDecommissioned(ctx, evergreen.User, j.TerminateIfBusy, "host will be terminated shortly, preventing task dispatch"); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"host_id":  j.host.Id,
				"provider": j.host.Distro.Provider,
				"job_type": j.Type().Name,
				"job":      j.ID(),
				"message":  "problem decommissioning host",
			}))
		}
	}

	j.host, err = host.FindOneId(ctx, j.HostID)
	if err != nil {
		j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
		return
	}
	if j.host == nil {
		j.AddError(errors.Errorf("host '%s' not found", j.HostID))
		return
	}

	// check if running task has been assigned since status changed
	if j.host.RunningTask != "" {
		if j.TerminateIfBusy {
			grip.Warning(message.Fields{
				"message":        "Host has running task; clearing before terminating",
				"job":            j.ID(),
				"job_type":       j.Type().Name,
				"host_id":        j.host.Id,
				"provider":       j.host.Distro.Provider,
				"task":           j.host.RunningTask,
				"task_execution": j.host.RunningTaskExecution,
			})

			j.AddError(errors.Wrapf(model.ClearAndResetStrandedHostTask(ctx, j.env.Settings(), j.host), "fixing stranded task '%s' execution '%d'", j.host.RunningTask, j.host.RunningTaskExecution))
		} else {
			return
		}
	}

	// terminate containers in DB if parent already terminated
	if j.host.ParentID != "" {
		var parent *host.Host
		parent, err = j.host.GetParent(ctx)
		if err != nil {
			if err.Error() != host.ErrorParentNotFound {
				j.AddError(errors.Wrapf(err, "finding parent for container '%s'", j.host.Id))
				return
			}
		}
		if parent == nil || parent.Status == evergreen.HostTerminated {
			if err = j.host.Terminate(ctx, evergreen.User, "parent was already terminated"); err != nil {
				j.AddError(errors.Wrapf(err, "terminating container '%s' in DB", j.host.Id))
			}
			return
		}
	}

	if err := j.checkAndTerminateCloudHost(ctx); err != nil {
		j.AddError(err)
		return
	}

	j.AddError(j.incrementIdleTime(ctx))

	terminationMessage := message.Fields{
		"message":            "host successfully terminated",
		"host_id":            j.host.Id,
		"distro":             j.host.Distro.Id,
		"single_task_distro": j.host.Distro.SingleTaskDistro,
		"ip_allocation_id":   j.host.IPAllocationID,
		"ip_association_id":  j.host.IPAssociationID,
		"job":                j.ID(),
		"reason":             j.TerminationReason,
		"total_idle_secs":    j.host.TotalIdleTime.Seconds(),
		"total_started_secs": j.host.TerminationTime.Sub(j.host.StartTime).Seconds(),
		"total_uptime_secs":  j.host.TerminationTime.Sub(j.host.CreationTime).Seconds(),
		"termination_time":   j.host.TerminationTime,
		"creation_time":      j.host.CreationTime,
		"started_by":         j.host.StartedBy,
		"user_host":          j.host.UserHost,
	}
	if !utility.IsZeroTime(j.host.BillingStartTime) {
		terminationMessage["total_billable_secs"] = j.host.TerminationTime.Sub(j.host.BillingStartTime).Seconds()
	}
	var instanceType string
	if evergreen.IsEc2Provider(j.host.Distro.Provider) && len(j.host.Distro.ProviderSettingsList) > 0 {
		var ok bool
		instanceType, ok = j.host.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
		if ok {
			terminationMessage["instance_type"] = instanceType
		}
	}
	grip.Info(terminationMessage)

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String(evergreen.DistroIDOtelAttribute, j.host.Distro.Id),
		attribute.String(evergreen.HostIDOtelAttribute, j.host.Id),
		attribute.Float64(fmt.Sprintf("%s.idle_secs", hostTerminationAttributePrefix), j.host.TotalIdleTime.Seconds()),
		attribute.Float64(fmt.Sprintf("%s.running_secs", hostTerminationAttributePrefix), j.host.TerminationTime.Sub(j.host.StartTime).Seconds()),
		attribute.String(fmt.Sprintf("%s.ip_allocation_id", hostTerminationAttributePrefix), j.host.IPAllocationID),
		attribute.String(fmt.Sprintf("%s.ip_association_id", hostTerminationAttributePrefix), j.host.IPAssociationID),
		attribute.String(fmt.Sprintf("%s.started_by", hostTerminationAttributePrefix), j.host.StartedBy),
		attribute.Bool(fmt.Sprintf("%s.user_host", hostTerminationAttributePrefix), j.host.UserHost),
		attribute.Int(fmt.Sprintf("%s.task_count", hostTerminationAttributePrefix), j.host.TaskCount),
	)
	if !utility.IsZeroTime(j.host.BillingStartTime) {
		span.SetAttributes(attribute.Float64(fmt.Sprintf("%s.billable_secs", hostTerminationAttributePrefix), j.host.TerminationTime.Sub(j.host.BillingStartTime).Seconds()))
	}
	if instanceType != "" {
		span.SetAttributes(attribute.String(fmt.Sprintf("%s.instance_type", hostTerminationAttributePrefix), instanceType))
	}

	if j.host.StartedBy == evergreen.User && j.host.TaskCount == 0 {
		grip.Info(message.Fields{
			"message":          "task host ran no tasks before it was terminated",
			"status":           prevStatus,
			"host_id":          j.HostID,
			"host_tag":         j.host.Tag,
			"distro":           j.host.Distro.Id,
			"uptime_secs":      time.Since(j.host.StartTime).Seconds(),
			"provider":         j.host.Provider,
			"spawn_host":       j.host.StartedBy != evergreen.User,
			"is_ec2_host":      cloud.IsEC2InstanceID(j.HostID),
			"agent_start_time": j.host.AgentStartTime,
		})
	}
}

func (j *hostTerminationJob) incrementIdleTime(ctx context.Context) error {
	idleTime := j.host.WastedComputeTime()

	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
	if err != nil {
		return errors.Wrapf(err, "getting cloud host for host '%s'", j.HostID)
	}
	if pad := cloudHost.CloudMgr.TimeTilNextPayment(j.host); pad > time.Second {
		idleTime += pad
	}

	return j.host.IncIdleTime(ctx, idleTime)
}

func (j *hostTerminationJob) cleanupIntentHost(ctx context.Context) error {
	if j.SkipCloudHostTermination {
		return errors.Wrap(j.host.Terminate(ctx, evergreen.User, j.TerminationReason), "marking DB host terminated")
	}

	if j.host.IPAllocationID != "" || j.host.IPAssociationID != "" {
		cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
		if err != nil {
			return errors.Wrap(err, "getting cloud host for intent host")
		}
		grip.Error(message.WrapError(cloudHost.CleanupIP(ctx), message.Fields{
			"message":        "could not clean up IP resources for intent host",
			"host_id":        j.host.Id,
			"allocation_id":  j.host.IPAllocationID,
			"association_id": j.host.IPAssociationID,
		}))
	}

	if err := j.host.Terminate(ctx, evergreen.User, j.TerminationReason); err != nil {
		return errors.Wrapf(err, "terminating intent host in DB")
	}
	return nil
}

// checkAndTerminateCloudHost checks if the host is still up according to the
// cloud provider. If so, it will attempt to terminate it in the cloud and mark
// the host as terminated. If not, it will just mark the host as terminated.
//
// If this job is set to skip cloud host termination, it will ignore the cloud
// host and only mark the host as terminated in the DB .
func (j *hostTerminationJob) checkAndTerminateCloudHost(ctx context.Context) error {
	if j.SkipCloudHostTermination {
		return errors.Wrap(j.host.Terminate(ctx, evergreen.User, j.TerminationReason), "marking DB host terminated")
	}

	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
	if err != nil {
		return errors.Wrapf(err, "getting cloud host for host '%s'", j.HostID)
	}

	cloudInfo, err := cloudHost.GetInstanceState(ctx)
	if err != nil {
		if utility.IsContextError(errors.Cause(err)) {
			return errors.Wrap(err, "checking cloud host status")
		}

		return errors.Wrap(err, "getting cloud host instance status")
	}
	if cloudInfo.Status == cloud.StatusNonExistent {
		return errors.Wrap(j.host.Terminate(ctx, evergreen.User, j.TerminationReason), "marking nonexistent host as terminated")
	}

	if cloudInfo.Status == cloud.StatusTerminated {
		grip.Info(message.Fields{
			"message":             "host is already terminated in the cloud, setting the host status to terminated",
			"cloud_status":        cloudInfo.Status,
			"cloud_status_reason": cloudInfo.StateReason,
		})
		return j.host.Terminate(ctx, evergreen.User, "cloud provider indicated that the host was already terminated")
	}

	if err := cloudHost.TerminateInstance(ctx, evergreen.User, j.TerminationReason); err != nil {
		return errors.Wrap(err, "terminating cloud host")
	}

	return nil
}
