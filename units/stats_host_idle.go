package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

const collectHostIdleDataJobName = "collect-host-idle-data"

func init() {
	registry.AddJobType(collectHostIdleDataJobName,
		func() amboy.Job { return newHostIdleJob() })
}

type collectHostIdleDataJob struct {
	HostID     string    `bson:"host_id" json:"host_id" yaml:"host_id"`
	StartTime  time.Time `bson:"start_time" json:"start_time" yaml:"start_time"`
	FinishTime time.Time `bson:"finish_time" json:"finish_time" yaml:"finish_time"`
	*job.Base  `bson:"metadata" json:"metadata" yaml:"metadata"`

	// internal cache
	host *host.Host
	task *task.Task
	env  evergreen.Environment
}

func newHostIdleJob() *collectHostIdleDataJob {
	j := &collectHostIdleDataJob{
		Base: &job.Base{
			JobType: amboy.JobType{
				Name:    collectHostIdleDataJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func NewCollectHostIdleDataJob(h *host.Host, t *task.Task, startTime, finishTime time.Time) amboy.Job {
	j := newHostIdleJob()

	j.host = h
	j.HostID = h.Id

	if t != nil {
		j.task = t
		j.TaskID = t.Id
	}

	j.StartTime = startTime
	j.FinishTime = finishTime
	j.SetID(fmt.Sprintf("%s.%s.%d", collectHostIdleDataJobName, j.HostID, job.GetNumber()))
	j.SetPriority(-2)
	return j
}

func (j *collectHostIdleDataJob) Run() {
	///////////////////////////////////
	//
	// set up job data, as needed

	defer j.MarkComplete()
	var err error

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
	}

	if j.task == nil && j.TaskID != "" {
		j.task, err = task.FindOneId(j.TaskID)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	///////////////////////////////////
	//
	// collect data

	manager, err := cloud.GetCloudManager(ctx, j.host.Provider, settings)
	if err != nil {
		j.AddError(err)
		grip.Error(message.WrapErrorf(err, "Error loading provider for host %s cost calculation", j.HostID))
	} else {
		if calc, ok := manager.(cloud.CloudCostCalculator); ok {
			cost, err = calc.CostForDuration(ctx, j.host, j.StartTime, j.FinishTime)
			if err != nil {
				j.AddError(err)
			}
			if err = j.host.IncCost(cost); err != nil {
				j.AddError(err)
			}
		}
	}

	if j.host.Provider != evergreen.ProviderNameStatic {
		if err = j.host.IncTaskCount(); err != nil {
			j.AddError(err)
		}
	}

	idleTime := j.FinishTime.Sub(j.StartTime)

	if err = j.host.IncIdleTime(idleTime); err != nil {
		j.AddError(err)
	}

	///////////////////////////////////
	//
	// send the messages

	grip.Info(j.getHostStatsMessage(cost, idleTime))

	// post task start data
	if j.TaskID != "" {
		grip.Info(j.getTaskStartStatsMessage())
	}
}

func (j *collectHostIdleDataJob) getHostStatsMessage(cost float64, idleTime time.Duration) message.Composer {
	// post host idle time message
	msg := message.Fields{
		"stat":      "host-idle",
		"distro":    j.host.Distro.Id,
		"provider":  j.host.Distro.Provider,
		"host":      j.host.Id,
		"status":    j.host.Status,
		"idle_secs": idleTime.Seconds(),
	}

	if strings.HasPrefix(j.host.Distro.Provider, "ec2") {
		msg["provider"] = "ec2"
	}

	if cost != 0 {
		msg["cost"] = cost
	}

	if j.host.Status == evergreen.HostTerminated {
		msg["task_count"] = j.host.TaskCount
	}

	return message.ConvertToComposer(level.Info, msg)
}

func (j *collectHostIdleDataJob) getTaskStartStatsMessage() message.Composer {
	msg := message.Fields{
		"stat":         "task-start-stats",
		"task_id":      j.task.Id,
		"task":         j.task.DisplayName,
		"execution":    j.task.Execution,
		"requester":    j.task.Requester,
		"project":      j.task.Project,
		"variant":      j.task.BuildVariant,
		"distro":       j.host.Distro.Id,
		"provider":     j.host.Distro.Provider,
		"host":         j.host.Id,
		"latency_secs": j.task.StartTime.Sub(j.task.GetTaskCreatedTime()).Seconds(),
	}

	if strings.HasPrefix(j.host.Distro.Provider, "ec2") {
		msg["provider"] = "ec2"
	}

	if j.task.ActivatedBy != "" {
		msg["activated_by"] = j.task.ActivatedBy
	}

	if j.host.Provider != evergreen.ProviderNameStatic {
		msg["host_task_count"] = j.host.TaskCount
	}

	return message.ConvertToComposer(level.Info, msg)
}
