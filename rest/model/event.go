package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type TaskAPIEventLogEntry struct {
	ID           *string        `bson:"_id" json:"-"`
	ResourceType *string        `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  *time.Time     `bson:"processed_at" json:"processed_at"`
	Timestamp    *time.Time     `bson:"ts" json:"timestamp"`
	ResourceId   *string        `bson:"r_id" json:"resource_id"`
	EventType    *string        `bson:"e_type" json:"event_type"`
	Data         *TaskEventData `bson:"data" json:"data"`
}

type TaskEventData struct {
	Execution int        `bson:"execution" json:"execution"`
	HostId    *string    `bson:"h_id,omitempty" json:"host_id,omitempty"`
	UserId    *string    `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status    *string    `bson:"s,omitempty" json:"status,omitempty"`
	JiraIssue *string    `bson:"jira,omitempty" json:"jira,omitempty"`
	JiraLink  *string    `bson:"jira_link,omitempty" json:"jira_link,omitempty"`
	Timestamp *time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
	Priority  int64      `bson:"pri,omitempty" json:"priority,omitempty"`
}

type HostAPIEventLogEntry struct {
	ID           *string           `bson:"_id" json:"-"`
	ResourceType *string           `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  *time.Time        `bson:"processed_at" json:"processed_at"`
	Timestamp    *time.Time        `bson:"ts" json:"timestamp"`
	ResourceId   *string           `bson:"r_id" json:"resource_id"`
	EventType    *string           `bson:"e_type" json:"event_type"`
	Data         *HostAPIEventData `bson:"data" json:"data"`
}

type HostAPIEventData struct {
	AgentRevision      *string     `bson:"a_rev,omitempty" json:"agent_revision,omitempty"`
	AgentBuild         *string     `bson:"a_build,omitempty" json:"agent_build,omitempty"`
	JasperRevision     *string     `bson:"j_rev,omitempty" json:"jasper_revision,omitempty"`
	OldStatus          *string     `bson:"o_s,omitempty" json:"old_status,omitempty"`
	NewStatus          *string     `bson:"n_s,omitempty" json:"new_status,omitempty"`
	Logs               *string     `bson:"log,omitempty" json:"logs,omitempty"`
	Hostname           *string     `bson:"hn,omitempty" json:"hostname,omitempty"`
	ProvisioningMethod *string     `bson:"prov_method" json:"provisioning_method,omitempty"`
	TaskId             *string     `bson:"t_id,omitempty" json:"task_id,omitempty"`
	TaskPid            *string     `bson:"t_pid,omitempty" json:"task_pid,omitempty"`
	TaskStatus         *string     `bson:"t_st,omitempty" json:"task_status,omitempty"`
	Execution          *string     `bson:"execution,omitempty" json:"execution,omitempty"`
	MonitorOp          *string     `bson:"monitor_op,omitempty" json:"monitor,omitempty"`
	User               *string     `bson:"usr" json:"user,omitempty"`
	Successful         bool        `bson:"successful,omitempty" json:"successful"`
	Duration           APIDuration `bson:"duration,omitempty" json:"duration"`
}

func (el *TaskEventData) BuildFromService(v *event.TaskEventData) {
	settings, err := evergreen.GetConfig()
	grip.Error(message.WrapError(err, message.Fields{
		"message": "error getting settings",
	}))
	jiraHost := settings.Jira.GetHostURL()
	jiraLink := ""
	if len(v.JiraIssue) != 0 {
		jiraLink = "https://" + jiraHost + "/browse/" + v.JiraIssue
	}
	el.Execution = v.Execution
	el.HostId = utility.ToStringPtr(v.HostId)
	el.UserId = utility.ToStringPtr(v.UserId)
	el.JiraIssue = utility.ToStringPtr(v.JiraIssue)
	el.JiraLink = utility.ToStringPtr(jiraLink)
	el.Status = utility.ToStringPtr(v.Status)
	el.Timestamp = ToTimePtr(v.Timestamp)
	el.Priority = v.Priority
}

// ToService is not implemented for TaskEventData.
func (el *TaskEventData) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for TaskEventData")
}

func (el *TaskAPIEventLogEntry) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case *event.EventLogEntry:
		d, ok := v.Data.(*event.TaskEventData)
		if ok == false {
			return errors.New(fmt.Sprintf("Incorrect type for data field when unmarshalling EventLogEntry"))
		}
		taskEventData := TaskEventData{}
		taskEventData.BuildFromService(d)
		el.ID = utility.ToStringPtr(v.ID)
		el.ResourceType = utility.ToStringPtr(v.ResourceType)
		el.ProcessedAt = ToTimePtr(v.ProcessedAt)
		el.Timestamp = ToTimePtr(v.Timestamp)
		el.ResourceId = utility.ToStringPtr(v.ResourceId)
		el.EventType = utility.ToStringPtr(v.EventType)
		el.Data = &taskEventData
	default:
		return errors.New(fmt.Sprintf("Incorrect type %T when unmarshalling EventLogEntry", t))
	}
	return nil
}

// ToService is not implemented for APITestStats.
func (el *TaskAPIEventLogEntry) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for TaskAPIEventLogEntry")
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

// ToService is not implemented for TaskEventData.
func (el *HostAPIEventData) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for HostAPIEventData")
}

func (el *HostAPIEventLogEntry) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case *event.EventLogEntry:
		d, ok := v.Data.(*event.HostEventData)
		if ok == false {
			return errors.New(fmt.Sprintf("Incorrect type for data field when unmarshalling EventLogEntry"))
		}
		hostAPIEventData := HostAPIEventData{}
		hostAPIEventData.BuildFromService(d)
		el.ID = utility.ToStringPtr(v.ID)
		el.ResourceType = utility.ToStringPtr(v.ResourceType)
		el.ProcessedAt = ToTimePtr(v.ProcessedAt)
		el.Timestamp = ToTimePtr(v.Timestamp)
		el.ResourceId = utility.ToStringPtr(v.ResourceId)
		el.EventType = utility.ToStringPtr(v.EventType)
		el.Data = &hostAPIEventData
	default:
		return errors.New(fmt.Sprintf("Incorrect type %T when unmarshalling EventLogEntry", t))
	}
	return nil
}

// ToService is not implemented for APITestStats.
func (el *HostAPIEventLogEntry) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for HostAPIEventLogEntry")
}
