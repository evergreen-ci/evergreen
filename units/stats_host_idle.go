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
	"github.com/pkg/errors"
)

const collectHostIdleDataJobName = "collect-host-idle-data"

func init() {
	registry.AddJobType(collectHostIdleDataJobName,
		func() amboy.Job { return newHostIdleJob() })
}

type collectHostIdleDataJob struct {
	HostID     string    `bson:"host_id" json:"host_id" yaml:"host_id"`
	TaskID     string    `bson:"task_id" json:"task_id" yaml:"task_id"`
	StartTime  time.Time `bson:"start_time" json:"start_time" yaml:"start_time"`
	FinishTime time.Time `bson:"finish_time" json:"finish_time" yaml:"finish_time"`
	*job.Base  `bson:"metadata" json:"metadata" yaml:"metadata"`

	// internal cache
	host     *host.Host
	task     *task.Task
	settings *evergreen.Settings
	manager  cloud.Manager
	env      evergreen.Environment
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

func newHostIdleJobForTermination(env evergreen.Environment, settings *evergreen.Settings, manager cloud.Manager,
	h *host.Host, startTime, finishTime time.Time) amboy.Job {

	j := NewCollectHostIdleDataJob(h, nil, startTime, finishTime).(*collectHostIdleDataJob)
	j.env = env
	j.settings = settings
	j.manager = manager

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
	j.SetID(fmt.Sprintf("%s.%s.%s.%d", collectHostIdleDataJobName, j.HostID, finishTime.Format(TSFormat), job.GetNumber()))
	j.SetPriority(-2)
	return j
}

func (j *collectHostIdleDataJob) Run(ctx context.Context) {
	///////////////////////////////////
	//
	// set up job data, as needed

	defer j.MarkComplete()
	var err error

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		j.AddError(err)
		if j.host == nil {
			j.AddError(errors.Errorf("unable to retrieve host %s", j.HostID))
		}
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

	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	///////////////////////////////////
	//
	// collect data

	if j.TaskID != "" && j.host.Provider != evergreen.ProviderNameStatic {
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

	grip.Info(j.getHostStatsMessage(idleTime))

	// post task start data
	if j.TaskID != "" {
		grip.Info(j.getTaskStartStatsMessage())
	}
}

func (j *collectHostIdleDataJob) getHostStatsMessage(idleTime time.Duration) message.Composer {
	runsTasks := j.host.StartedBy == evergreen.User && !j.host.HasContainers

	// post host idle time message
	msg := message.Fields{
		"stat":            "host-idle",
		"distro":          j.host.Distro.Id,
		"provider":        j.host.Distro.Provider,
		"provisioning":    j.host.Distro.BootstrapSettings.Method,
		"host_id":         j.host.Id,
		"status":          j.host.Status,
		"idle_secs":       idleTime.Seconds(),
		"spawn_host":      j.host.StartedBy != evergreen.User && !j.host.SpawnOptions.SpawnedByTask,
		"task_spawn_host": j.host.SpawnOptions.SpawnedByTask,
		"has_containers":  j.host.HasContainers,
		"task_host":       runsTasks,
	}

	if strings.HasPrefix(j.host.Distro.Provider, "ec2") {
		msg["provider"] = "ec2"
	}

	if j.host.Provider != evergreen.ProviderNameStatic {
		msg["host_task_count"] = j.host.TaskCount
	}

	if j.host.TaskCount == 1 && j.TaskID != "" {
		msg["startup_secs"] = j.task.StartTime.Sub(j.host.CreationTime).Seconds()
	}

	return message.ConvertToComposer(level.Info, msg)
}

func (j *collectHostIdleDataJob) getTaskStartStatsMessage() message.Composer {
	msg := message.Fields{
		"activated_latency_secs": j.task.StartTime.Sub(j.task.ActivatedTime).Seconds(),
		"display_task":           j.task.DisplayOnly,
		"distro":                 j.host.Distro.Id,
		"execution":              j.task.Execution,
		"generator":              j.task.GenerateTask,
		"group":                  j.task.TaskGroup,
		"group_max_hosts":        j.task.TaskGroupMaxHosts,
		"host_id":                j.host.Id,
		"project":                j.task.Project,
		"provider":               j.host.Distro.Provider,
		"provisioning":           j.host.Distro.BootstrapSettings.Method,
		"requester":              j.task.Requester,
		"scheduled_latency_secs": j.task.StartTime.Sub(j.task.ScheduledTime).Seconds(),
		"stat":                   "task-start-stats",
		"task":                   j.task.DisplayName,
		"task_id":                j.task.Id,
		"variant":                j.task.BuildVariant,
	}

	if strings.HasPrefix(j.host.Distro.Provider, "ec2") {
		msg["provider"] = "ec2"
	}

	if j.task.ActivatedBy != "" {
		msg["activated_by"] = j.task.ActivatedBy
	}

	if j.host.Provider != evergreen.ProviderNameStatic {
		msg["host_task_count"] = j.host.TaskCount

		if j.host.TaskCount == 1 {
			msg["host_provision_time"] = j.host.TotalIdleTime.Seconds()
		}
	}

	return message.ConvertToComposer(level.Info, msg)
}
