package model

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type TaskAPIEventLogEntry struct {
	ID           *string        `bson:"_id" json:"-"`
	ResourceType *string        `bson:"r_type,omitempty" json:"resource_type"`
	ProcessedAt  *time.Time     `bson:"processed_at" json:"processed_at"`
	Timestamp    *time.Time     `bson:"ts" json:"timestamp"`
	ResourceId   *string        `bson:"r_id" json:"resource_id"`
	EventType    *string        `bson:"e_type" json:"event_type"`
	Data         *TaskEventData `bson:"data" json:"data"`
}

type TaskEventData struct {
	Execution int        `bson:"execution" json:"execution"`
	HostId    *string    `bson:"h_id,omitempty" json:"host_id"`
	PodId     *string    `bson:"pod_id,omitempty" json:"pod_id"`
	UserId    *string    `bson:"u_id,omitempty" json:"user_id"`
	Status    *string    `bson:"s,omitempty" json:"status"`
	JiraIssue *string    `bson:"jira,omitempty" json:"jira"`
	JiraLink  *string    `bson:"jira_link,omitempty" json:"jira_link"`
	BlockedOn *string    `bson:"blocked_on,omitempty" json:"blocked_on"`
	Timestamp *time.Time `bson:"ts,omitempty" json:"timestamp"`
	Priority  int64      `bson:"pri,omitempty" json:"priority"`
}

type HostAPIEventLogEntry struct {
	ID           *string           `bson:"_id" json:"-"`
	ResourceType *string           `bson:"r_type,omitempty" json:"resource_type"`
	ProcessedAt  *time.Time        `bson:"processed_at" json:"processed_at"`
	Timestamp    *time.Time        `bson:"ts" json:"timestamp"`
	ResourceId   *string           `bson:"r_id" json:"resource_id"`
	EventType    *string           `bson:"e_type" json:"event_type"`
	Data         *HostAPIEventData `bson:"data" json:"data"`
}

type HostAPIEventData struct {
	AgentRevision      *string     `bson:"a_rev,omitempty" json:"agent_revision"`
	AgentBuild         *string     `bson:"a_build,omitempty" json:"agent_build"`
	JasperRevision     *string     `bson:"j_rev,omitempty" json:"jasper_revision"`
	OldStatus          *string     `bson:"o_s,omitempty" json:"old_status"`
	NewStatus          *string     `bson:"n_s,omitempty" json:"new_status"`
	Logs               *string     `bson:"log,omitempty" json:"logs"`
	Hostname           *string     `bson:"hn,omitempty" json:"hostname"`
	ProvisioningMethod *string     `bson:"prov_method" json:"provisioning_method"`
	TaskId             *string     `bson:"t_id,omitempty" json:"task_id"`
	TaskPid            *string     `bson:"t_pid,omitempty" json:"task_pid"`
	TaskStatus         *string     `bson:"t_st,omitempty" json:"task_status"`
	Execution          *string     `bson:"execution,omitempty" json:"execution"`
	MonitorOp          *string     `bson:"monitor_op,omitempty" json:"monitor"`
	User               *string     `bson:"usr" json:"user"`
	Successful         bool        `bson:"successful,omitempty" json:"successful"`
	Duration           APIDuration `bson:"duration,omitempty" json:"duration"`
}

type PodAPIEventLogEntry struct {
	ID           *string          `bson:"_id" json:"-"`
	ResourceType *string          `bson:"r_type,omitempty" json:"resource_type"`
	ProcessedAt  *time.Time       `bson:"processed_at" json:"processed_at"`
	Timestamp    *time.Time       `bson:"ts" json:"timestamp"`
	ResourceId   *string          `bson:"r_id" json:"resource_id"`
	EventType    *string          `bson:"e_type" json:"event_type"`
	Data         *PodAPIEventData `bson:"data" json:"data"`
}

type PodAPIEventData struct {
	OldStatus *string `bson:"old_status,omitempty" json:"old_status"`
	NewStatus *string `bson:"new_status,omitempty" json:"new_status"`
	Reason    *string `bson:"reason,omitempty" json:"reason"`

	// Fields related to pods running tasks
	TaskID        *string `bson:"task_id,omitempty" json:"task_id"`
	TaskExecution *int    `bson:"task_execution,omitempty" json:"task_execution"`
	TaskStatus    *string `bson:"task_status,omitempty" json:"task_status"`
}

func (el *TaskEventData) BuildFromService(ctx context.Context, v *event.TaskEventData) error {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting admin settings")
	}
	jiraHost := settings.Jira.GetHostURL()
	jiraLink := ""
	if len(v.JiraIssue) != 0 {
		jiraLink = jiraHost + "/browse/" + v.JiraIssue
	}
	el.Execution = v.Execution
	el.HostId = utility.ToStringPtr(v.HostId)
	el.PodId = utility.ToStringPtr(v.PodID)
	el.UserId = utility.ToStringPtr(v.UserId)
	el.JiraIssue = utility.ToStringPtr(v.JiraIssue)
	el.JiraLink = utility.ToStringPtr(jiraLink)
	el.Status = utility.ToStringPtr(v.Status)
	el.BlockedOn = utility.ToStringPtr(v.BlockedOn)
	el.Timestamp = ToTimePtr(v.Timestamp)
	el.Priority = v.Priority
	return nil
}

func (el *TaskAPIEventLogEntry) BuildFromService(ctx context.Context, v event.EventLogEntry) error {
	d, ok := v.Data.(*event.TaskEventData)
	if !ok {
		return errors.Errorf("programmatic error: expected task event data but got type %T", v.Data)
	}
	taskEventData := TaskEventData{}
	if err := taskEventData.BuildFromService(ctx, d); err != nil {
		return errors.Wrap(err, "converting task event data to API model")
	}
	el.ID = utility.ToStringPtr(v.ID)
	el.ResourceType = utility.ToStringPtr(v.ResourceType)
	el.ProcessedAt = ToTimePtr(v.ProcessedAt)
	el.Timestamp = ToTimePtr(v.Timestamp)
	el.ResourceId = utility.ToStringPtr(v.ResourceId)
	el.EventType = utility.ToStringPtr(v.EventType)
	el.Data = &taskEventData
	return nil
}

//HostEvent functions

func (el *HostAPIEventData) BuildFromService(v *event.HostEventData) {

	el.AgentRevision = utility.ToStringPtr(v.AgentRevision)
	el.AgentBuild = utility.ToStringPtr(v.AgentBuild)
	el.JasperRevision = utility.ToStringPtr(v.JasperRevision)
	el.OldStatus = utility.ToStringPtr(v.OldStatus)
	el.NewStatus = utility.ToStringPtr(v.NewStatus)
	el.Logs = utility.ToStringPtr(v.Logs)
	el.Hostname = utility.ToStringPtr(v.Hostname)
	el.ProvisioningMethod = utility.ToStringPtr(v.ProvisioningMethod)
	el.TaskId = utility.ToStringPtr(v.TaskId)
	el.TaskPid = utility.ToStringPtr(v.TaskPid)
	el.TaskStatus = utility.ToStringPtr(v.TaskStatus)
	el.Execution = utility.ToStringPtr(v.Execution)
	el.MonitorOp = utility.ToStringPtr(v.MonitorOp)
	el.User = utility.ToStringPtr(v.User)
	el.Successful = v.Successful
	el.Duration = NewAPIDuration(v.Duration)
}

func (el *HostAPIEventLogEntry) BuildFromService(entry event.EventLogEntry) error {
	d, ok := entry.Data.(*event.HostEventData)
	if !ok {
		return errors.Errorf("programmatic error: expected host event data but got type %T", entry.Data)
	}
	hostAPIEventData := HostAPIEventData{}
	hostAPIEventData.BuildFromService(d)
	el.ID = utility.ToStringPtr(entry.ID)
	el.ResourceType = utility.ToStringPtr(entry.ResourceType)
	el.ProcessedAt = ToTimePtr(entry.ProcessedAt)
	el.Timestamp = ToTimePtr(entry.Timestamp)
	el.ResourceId = utility.ToStringPtr(entry.ResourceId)
	el.EventType = utility.ToStringPtr(entry.EventType)
	el.Data = &hostAPIEventData
	return nil
}

func (el *PodAPIEventLogEntry) BuildFromService(entry event.EventLogEntry) error {
	d, ok := entry.Data.(*event.PodData)
	if !ok {
		return errors.Errorf("programmatic error: expected pod event data but got type %T", entry.Data)
	}
	podAPIEventData := PodAPIEventData{}
	podAPIEventData.BuildFromService(d)
	el.ID = utility.ToStringPtr(entry.ID)
	el.ResourceType = utility.ToStringPtr(entry.ResourceType)
	el.ProcessedAt = ToTimePtr(entry.ProcessedAt)
	el.Timestamp = ToTimePtr(entry.Timestamp)
	el.ResourceId = utility.ToStringPtr(entry.ResourceId)
	el.EventType = utility.ToStringPtr(entry.EventType)
	el.Data = &podAPIEventData
	return nil
}

func (el *PodAPIEventData) BuildFromService(v *event.PodData) {
	el.OldStatus = utility.ToStringPtr(v.OldStatus)
	el.NewStatus = utility.ToStringPtr(v.NewStatus)
	el.Reason = utility.ToStringPtr(v.Reason)
	el.TaskID = utility.ToStringPtr(v.TaskID)
	el.TaskExecution = utility.ToIntPtr(v.TaskExecution)
	el.TaskStatus = utility.ToStringPtr(v.TaskStatus)
}
