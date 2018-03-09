package units

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const collectTaskStartDataJobName = "collect-task-start-data"

func init() {
	registry.AddJobType(collectTaskStartDataJobName,
		func() amboy.Job { return newTaskStartJob() })
}

type collectTaskStartDataJob struct {
	TaskID   string `bson:"task_id" json:"task_id" yaml:"task_id"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	// internal cache
	task *task.Task
	host *host.Host
}

func newTaskStartJob() *collectTaskStartDataJob {
	j := &collectTaskStartDataJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    collectTaskStartDataJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewCollectTaskStartDataJob(t *task.Task, h *host.Host) amboy.Job {
	j := newTaskEndJob()
	j.TaskID = t.Id
	j.HostID = h.Id
	j.task = t
	j.host = h
	j.SetID(fmt.Sprintf("%s.%s.%s.%d", collectTaskStartDataJobName, j.TaskID, j.HostID, job.GetNumber()))
	return j
}

func (j *collectTaskStartDataJob) Run() {
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

	msg := message.Fields{
		"stat":         "task-start-stats",
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
		"latency_secs": j.task.GetTaskCreatedTime().Sub(j.task.StartTime).Seconds(),
	}

	if j.host.Provider != evergreen.ProviderNameStatic {
		if err = j.host.IncTaskCount(); err != nil {
			j.AddError(err)
		}

		msg["host_task_count"] = j.host.TaskCount
	}

	grip.Info(msg)
}
