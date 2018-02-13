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

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	var cost float64

	manager, err := cloud.GetCloudManager(j.host.Provider, settings)
	if err != nil {
		j.AddError(err)
		grip.Error(message.WrapErrorf(err, "Error loading provider for host %s cost calculation", j.task.HostId))
	} else {
		if calc, ok := manager.(cloud.CloudCostCalculator); ok {
			cost, err = calc.CostForDuration(j.host, j.task.StartTime, j.task.FinishTime)
			if err != nil {
				j.AddError(err)
			} else {
				if err = j.task.SetCost(cost); err != nil {
					j.AddError(err)
				}
			}
		}
	}

	msg := message.Fields{
		"stat":         "task-end-stats",
		"task_id":      j.task.Id,
		"task":         j.task.DisplayName,
		"execution":    j.task.Execution,
		"requester":    j.task.Requester,
		"activated_by": j.task.ActivatedBy,
		"project":      j.task.Project,
		"variant":      j.task.BuildVariant,
		"distro":       j.host.Distro.Id,
		"provider":     j.host.Distro.Provider,
		"host":         j.host.Id,
	}

	if cost != 0 {
		msg["cost"] = cost
	}

	historicRuntime, err := j.task.GetHistoricRuntime()
	if err != nil {
		msg[message.FieldsMsgName] = "problem computing historic runtime"
		grip.Warning(message.WrapError(err, msg))
		j.AddError(err)
	} else {
		msg["average_runtime_secs"] = historicRuntime.Seconds()
		msg["current_runtime_secs"] = j.task.FinishTime.Sub(j.task.StartTime).Seconds()
		grip.Info(msg)
	}
}
