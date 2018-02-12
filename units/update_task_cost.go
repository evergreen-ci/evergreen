package units

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const collectTaskEndDataJobName = "collect-task-end-data"

// collectTaskEndData determines a task's cost based on the host it ran on. Hosts that
// are unable to calculate their own costs will not set a task's Cost field. Errors
// are logged but not returned, since any number of API failures could happen and
// we shouldn't sacrifice a task's status for them.
type collectTaskEndDataJob struct {
	TaskID     string    `bson:"task_id" json:"task_id" yaml:"task_id"`
	HostID     string    `bson:"host_id" json:"host_id" yaml:"host_id"`
	FinishTime time.Time `bson:"finish_time" json:"finish_time" yaml:"finish_time"`
	job.Base   `bson:"metadata" json:"metadata" yaml:"metadata"`

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
				Format:  amboy.BSON,
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
	j.HostID = h.Id
	j.task = t
	j.host = h
	j.SetID(fmt.Sprintf("%s.%s.%s.%d", collectTaskEndDataJobName, j.TaskID, j.HostID, job.GetNumber()))
	return j
}

func (j *collectTaskEndDataJob) Run() {
	var err error
	if j.task == nil {
		j.task, err = task.FindOneId(j.TaskID)
		j.AddError(err)
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
	}

	if j.HasErrors() {
		return
	}

	historicRuntime, err := j.task.GetHistoricRuntime()
	if err != nil {
		j.AddError(err)
		grip.Warning(message.WrapError(err, message.Fields{
			"message":      "problem computing historic runtime",
			"task_id":      j.task.Id,
			"task":         j.task.DisplayName,
			"execution":    j.task.Execution,
			"requester":    j.task.Requester,
			"activated_by": j.task.ActivatedBy,
			"project":      j.task.Project,
			"variant":      j.task.BuildVariant,
		}))
	} else {
		grip.Info(message.Fields{
			"stat":                 "average-task-runtime",
			"task_id":              j.task.Id,
			"task":                 j.task.DisplayName,
			"execution":            j.task.Execution,
			"requester":            j.task.Requester,
			"activated_by":         j.task.ActivatedBy,
			"project":              j.task.Project,
			"variant":              j.task.BuildVariant,
			"average_runtime_secs": historicRuntime.Seconds(),
			"current_runtime_secs": j.task.FinishTime.Sub(j.task.StartTime).Seconds(),
		})
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	manager, err := cloud.GetCloudManager(j.host.Provider, settings)
	if err != nil {
		j.AddError(err)
		grip.Error(message.WrapErrorf(err, "Error loading provider for host %s cost calculation", j.task.HostId))
		return
	}
	if calc, ok := manager.(cloud.CloudCostCalculator); ok {
		grip.Infoln("Calculating cost for task:", j.task.Id)
		cost, err := calc.CostForDuration(j.host, j.task.StartTime, j.task.FinishTime)
		if err != nil {
			j.AddError(err)
			grip.Errorf("calculating cost for task %s: %+v ", j.task.Id, err)
			return
		}
		if err := j.task.SetCost(cost); err != nil {
			j.AddError(err)
			grip.Errorf("Error updating cost for task %s: %+v ", j.task.Id, err)
			return
		}
	}
}
