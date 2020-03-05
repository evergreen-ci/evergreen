package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/pkg/errors"
)

type APIEventLogEntry struct {
	ID           *string        `bson:"_id" json:"-"`
	ResourceType *string        `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	ProcessedAt  *time.Time     `bson:"processed_at" json:"processed_at"`
	Timestamp    *time.Time     `bson:"ts" json:"timestamp"`
	ResourceId   *string        `bson:"r_id" json:"resource_id"`
	EventType    *string        `bson:"e_type" json:"event_type"`
	Data         *TaskEventData `bson:"data" json:"data"`
}

type TaskEventData struct {
	Execution int     `bson:"execution" json:"execution"`
	HostId    *string `bson:"h_id,omitempty" json:"host_id,omitempty"`
	UserId    *string `bson:"u_id,omitempty" json:"user_id,omitempty"`
	Status    *string `bson:"s,omitempty" json:"status,omitempty"`
	JiraIssue *string `bson:"jira,omitempty" json:"jira,omitempty"`

	Timestamp *time.Time `bson:"ts,omitempty" json:"timestamp,omitempty"`
	Priority  int64      `bson:"pri,omitempty" json:"priority,omitempty"`
}

func (el *TaskEventData) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case *event.TaskEventData:
		el = &TaskEventData{
			Execution: v.Execution,
			HostId:    ToStringPtr(v.HostId),
			UserId:    ToStringPtr(v.UserId),
			JiraIssue: ToStringPtr(v.JiraIssue),
			Status:    ToStringPtr(v.Status),
			Timestamp: ToTimePtr(v.Timestamp),
			Priority:  v.Priority,
		}
	default:
		return errors.New(fmt.Sprintf("Incorrect type %T when unmarshalling TaskEventData", t))
	}
	return nil
}

// ToService is not implemented for TaskEventData.
func (el *TaskEventData) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for TaskEventData")
}

func (el *APIEventLogEntry) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case *event.EventLogEntry:
		d, ok := v.Data.(*event.TaskEventData)
		if ok == false {
			return errors.New(fmt.Sprintf("Incorrect type for data field when unmarshalling EventLogEntry"))
		}
		(*el) = APIEventLogEntry{
			ID:           ToStringPtr(v.ID),
			ResourceType: ToStringPtr(v.ResourceType),
			ProcessedAt:  ToTimePtr(v.ProcessedAt),
			Timestamp:    ToTimePtr(v.Timestamp),
			ResourceId:   ToStringPtr(v.ResourceId),
			EventType:    ToStringPtr(v.EventType),
			Data: &TaskEventData{
				Execution: d.Execution,
				HostId:    ToStringPtr(d.HostId),
				UserId:    ToStringPtr(d.UserId),
				JiraIssue: ToStringPtr(d.JiraIssue),
				Timestamp: ToTimePtr(d.Timestamp),
				Priority:  d.Priority,
				Status:    ToStringPtr(d.Status),
			},
		}
	default:
		return errors.New(fmt.Sprintf("Incorrect type %T when unmarshalling EventLogEntry", t))
	}
	return nil
}

// ToService is not implemented for APITestStats.
func (el *APIEventLogEntry) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIEventLogEntry")
}
