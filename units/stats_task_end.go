package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const collectTaskEndDataJobName = "collect-task-end-data"

func init() {
	registry.AddJobType(collectTaskEndDataJobName,
		func() amboy.Job { return newTaskEndJob() })
}

type collectTaskEndDataJob struct {
	TaskID       string `bson:"task_id" json:"task_id" yaml:"task_id"`
	HostID       string `bson:"host_id" json:"host_id" yaml:"host_id"`
	Execution    int    `bson:"execution" json:"execution" yaml:"execution"`
	RuntimeEnvID string `bson:"runtime_env_id" json:"runtime_env_id" yaml:"runtime_env_id"`
	job.Base     `bson:"metadata" json:"metadata" yaml:"metadata"`

	// internal cache
	task *task.Task
	env  evergreen.Environment
}

func newTaskEndJob() *collectTaskEndDataJob {
	j := &collectTaskEndDataJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    collectTaskEndDataJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewCollectTaskEndDataJob logs information a task after it
// completes. It also logs information about the total runtime and instance
// type, which can be used to measure the cost of running a task.
func NewCollectTaskEndDataJob(t task.Task, id string) amboy.Job {
	j := newTaskEndJob()
	j.TaskID = t.Id
	j.Execution = t.Execution
	j.RuntimeEnvID = id
	j.task = &t
	j.SetID(fmt.Sprintf("%s.%s.%s.%d", collectTaskEndDataJobName, j.TaskID, j.RuntimeEnvID, job.GetNumber()))
	j.SetPriority(-2)
	return j
}

func (j *collectTaskEndDataJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	var err error
	if j.task == nil {
		j.task, err = task.FindOneIdAndExecution(j.TaskID, j.Execution)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding task '%s' with execution %d", j.TaskID, j.Execution))
			return
		}
	}

	if j.task == nil {
		j.AddError(errors.Errorf("task '%s' not found", j.TaskID))
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	msg := message.Fields{
		"abort":                j.task.Aborted,
		"activated_by":         j.task.ActivatedBy,
		"build":                j.task.BuildId,
		"current_runtime_secs": j.task.FinishTime.Sub(j.task.StartTime).Seconds(),
		"display_task":         j.task.DisplayOnly,
		"execution":            j.task.Execution,
		"generator":            j.task.GenerateTask,
		"group":                j.task.TaskGroup,
		"group_max_hosts":      j.task.TaskGroupMaxHosts,
		"priority":             j.task.Priority,
		"project":              j.task.Project,
		"requester":            j.task.Requester,
		"stat":                 "task-end-stats",
		"status":               j.task.GetDisplayStatus(),
		"task":                 j.task.DisplayName,
		"task_id":              j.task.Id,
		"total_wait_secs":      j.task.FinishTime.Sub(j.task.ActivatedTime).Seconds(),
		"start_time":           j.task.StartTime,
		"scheduled_time":       j.task.ScheduledTime,
		"variant":              j.task.BuildVariant,
		"version":              j.task.Version,
	}

	if j.task.IsPartOfDisplay() {
		msg["display_task_id"] = j.task.DisplayTaskId
	}

	pRef, err := model.FindBranchProjectRef(j.task.Project)
	if pRef != nil {
		msg["project_identifier"] = pRef.Identifier
	}
	j.AddError(err)

	isHostMode := j.task.IsHostTask()
	if isHostMode {
		taskHost, err := host.FindOneId(j.RuntimeEnvID)
		j.AddError(err)
		if err != nil {
			return
		}
		if taskHost == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.RuntimeEnvID))
			return
		}
		msg["host_id"] = taskHost.Id
		msg["distro"] = taskHost.Distro.Id
		msg["provider"] = taskHost.Distro.Provider
		if cloud.IsEc2Provider(taskHost.Distro.Provider) && len(taskHost.Distro.ProviderSettingsList) > 0 {
			instanceType, ok := taskHost.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
			if ok {
				msg["instance_type"] = instanceType
			}
		}
	} else {
		taskPod, err := pod.FindOneByID(j.RuntimeEnvID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding pod '%s'", j.RuntimeEnvID))
			return
		}
		if taskPod == nil {
			j.AddError(errors.Errorf("pod '%s' not found", j.RuntimeEnvID))
			return
		}
		msg["pod_id"] = taskPod.ID
		msg["pod_os"] = taskPod.TaskContainerCreationOpts.OS
		msg["pod_arch"] = taskPod.TaskContainerCreationOpts.Arch
		msg["cpu"] = taskPod.TaskContainerCreationOpts.CPU
		msg["memory_mb"] = taskPod.TaskContainerCreationOpts.MemoryMB
		if taskPod.TaskContainerCreationOpts.OS.Matches(evergreen.ECSOS(pod.OSWindows)) {
			msg["windows_version"] = taskPod.TaskContainerCreationOpts.WindowsVersion
		}
	}

	if !j.task.DependenciesMetTime.IsZero() {
		msg["dependencies_met_time"] = j.task.DependenciesMetTime
	}

	if !j.task.ContainerAllocatedTime.IsZero() {
		msg["container_allocated_time"] = j.task.ContainerAllocatedTime
	}

	historicRuntime, err := j.task.GetHistoricRuntime()
	if err != nil {
		msg[message.FieldsMsgName] = "problem computing historic runtime"
		grip.Warning(message.WrapError(err, msg))
	} else {
		msg["average_runtime_secs"] = historicRuntime.Seconds()
		grip.Info(msg)
	}
}
