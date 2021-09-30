package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
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

const collectTaskEndDataJobName = "collect-task-end-data"

func init() {
	registry.AddJobType(collectTaskEndDataJobName,
		func() amboy.Job { return newTaskEndJob() })
}

type collectTaskEndDataJob struct {
	TaskID    string `bson:"task_id" json:"task_id" yaml:"task_id"`
	Execution int    `bson:"execution" json:"execution" yaml:"execution"`
	HostID    string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base  `bson:"metadata" json:"metadata" yaml:"metadata"`

	// internal cache
	task *task.Task
	host *host.Host
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
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewCollectTaskEndDataJob logs information a task after it
// completes. It also logs information about the total runtime and instance
// type, which can be used to measure the cost of running a task.
func NewCollectTaskEndDataJob(t *task.Task, h *host.Host) amboy.Job {
	j := newTaskEndJob()
	j.TaskID = t.Id
	j.Execution = t.Execution
	j.HostID = h.Id
	j.task = t
	j.host = h
	j.SetID(fmt.Sprintf("%s.%s.%s.%d", collectTaskEndDataJobName, j.TaskID, j.HostID, job.GetNumber()))
	j.SetPriority(-2)
	return j
}

func (j *collectTaskEndDataJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	var err error
	if j.task == nil {
		j.task, err = task.FindOneIdAndExecution(j.TaskID, j.Execution)
		j.AddError(err)
		if err != nil {
			return
		}
	}
	// The task was restarted before the job ran.
	if j.task == nil {
		j.task, err = task.FindOneOldByIdAndExecution(j.TaskID, j.Execution)
		j.AddError(err)
		if err != nil {
			return
		}
	}
	if j.task == nil {
		j.AddError(errors.Errorf("Could not find task '%s'", j.TaskID))
		return
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
		if err != nil {
			return
		}
	}
	if j.host == nil {
		j.AddError(errors.Errorf("Could not find host '%s'", j.HostID))
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
		"distro":               j.host.Distro.Id,
		"execution":            j.task.Execution,
		"generator":            j.task.GenerateTask,
		"group":                j.task.TaskGroup,
		"group_max_hosts":      j.task.TaskGroupMaxHosts,
		"host_id":              j.host.Id,
		"priority":             j.task.Priority,
		"project":              j.task.Project,
		"provider":             j.host.Distro.Provider,
		"requester":            j.task.Requester,
		"stat":                 "task-end-stats",
		"status":               j.task.ResultStatus(),
		"task":                 j.task.DisplayName,
		"task_id":              j.task.Id,
		"total_wait_secs":      j.task.FinishTime.Sub(j.task.ActivatedTime).Seconds(),
		"start_time":           j.task.StartTime,
		"scheduled_time":       j.task.ScheduledTime,
		"variant":              j.task.BuildVariant,
		"version":              j.task.Version,
	}

	if j.task.DisplayTask != nil {
		msg["display_task_id"] = j.task.DisplayTask.Id
	}

	pRef, err := model.FindBranchProjectRef(j.task.Project)
	if pRef != nil {
		msg["project_identifier"] = pRef.Identifier
	}
	j.AddError(err)

	if cloud.IsEc2Provider(j.host.Distro.Provider) && len(j.host.Distro.ProviderSettingsList) > 0 {
		instanceType, ok := j.host.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
		if ok {
			msg["instance_type"] = instanceType
		}
	}

	if !j.task.DependenciesMetTime.IsZero() {
		msg["dependencies_met_time"] = j.task.DependenciesMetTime
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
