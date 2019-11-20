package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
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

// collectTaskEndData determines a task's cost based on the host it ran on. Hosts that
// are unable to calculate their own costs will not set a task's Cost field. Errors
// are logged but not returned, since any number of API failures could happen and
// we shouldn't sacrifice a task's status for them.
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

// NewCollectTaskEndDataJob determines a task's cost based on the host it ran on. Hosts that
// are unable to calculate their own costs will not set a task's Cost field. Errors
// are logged but not returned, since any number of API failures could happen and
// we shouldn't sacrifice a task's status for them.
//
// It also logs historic task timing information.
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
		j.task, err = task.FindOneOldNoMergeByIdAndExecution(j.TaskID, j.Execution)
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

	var cost float64
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	catcher := grip.NewBasicCatcher()
	mgrOpts, err := GetManagerOptions(j.host.Distro)
	catcher.Add(errors.Wrapf(err, "can't get ManagerOpts for '%s'", j.host.Id))
	if !catcher.HasErrors() {
		manager, err := cloud.GetManager(ctx, j.env, mgrOpts)
		catcher.Add(errors.Wrapf(err, "Error loading provider for host %s cost calculation", j.task.HostId))
	}
	if !catcher.HasErrors() {
		if calc, ok := manager.(cloud.CostCalculator); ok {
			cost, err = calc.CostForDuration(ctx, j.host, j.task.StartTime, j.task.FinishTime)
			if err != nil {
				j.AddError(err)
			} else {
				if err = j.task.SetCost(cost); err != nil {
					j.AddError(err)
				}
				if err = j.host.IncCost(cost); err != nil {
					j.AddError(err)
				}
			}
		}
	}
	j.AddError(catcher.Resolve())

	msg := message.Fields{
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
		"variant":              j.task.BuildVariant,
		"version":              j.task.Version,
	}

	if cost != 0 {
		msg["cost"] = cost
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
