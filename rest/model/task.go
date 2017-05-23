package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

const (
	LogLinkFormat = "%s/task_log_raw/%s/%d?type=%s"
)

// APITask is the model to be returned by the API whenever tasks are fetched.
type APITask struct {
	Id               APIString        `json:"task_id"`
	CreateTime       APITime          `json:"create_time"`
	DispatchTime     APITime          `json:"dispatch_time"`
	PushTime         APITime          `json:"push_time"`
	ScheduledTime    APITime          `json:"scheduled_time"`
	StartTime        APITime          `json:"start_time"`
	FinishTime       APITime          `json:"finish_time"`
	Version          APIString        `json:"version_id"`
	Branch           APIString        `json:"branch"`
	Revision         APIString        `json:"revision"`
	Priority         int64            `json:"priority"`
	Activated        bool             `json:"activated"`
	ActivatedBy      APIString        `json:"activated_by"`
	BuildId          APIString        `json:"build_id"`
	DistroId         APIString        `json:"distro_id"`
	BuildVariant     APIString        `json:"build_variant"`
	DependsOn        []string         `json:"depends_on"`
	DisplayName      APIString        `json:"display_name"`
	HostId           APIString        `json:"host_id"`
	Restarts         int              `json:"restarts"`
	Execution        int              `json:"execution"`
	Order            int              `json:"order"`
	Status           APIString        `json:"status"`
	Details          apiTaskEndDetail `json:"status_details"`
	Logs             logLinks         `json:"logs"`
	TimeTaken        time.Duration    `json:"time_taken_ms"`
	ExpectedDuration time.Duration    `json:"expected_duration_ms"`
}

type logLinks struct {
	AllLogLink    APIString `json:"all_log"`
	TaskLogLink   APIString `json:"task_log"`
	AgentLogLink  APIString `json:"agent_log"`
	SystemLogLink APIString `json:"system_log"`
}
type apiTaskEndDetail struct {
	Status      APIString `json:"status"`
	Type        APIString `json:"type"`
	Description APIString `json:"desc"`
	TimedOut    bool      `json:"timed_out"`
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APITask.
func (at *APITask) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case *task.Task:
		(*at) = APITask{
			Id:            APIString(v.Id),
			CreateTime:    APITime(v.CreateTime),
			DispatchTime:  APITime(v.DispatchTime),
			PushTime:      APITime(v.PushTime),
			ScheduledTime: APITime(v.ScheduledTime),
			StartTime:     APITime(v.StartTime),
			FinishTime:    APITime(v.FinishTime),
			Version:       APIString(v.Version),
			Branch:        APIString(v.Project),
			Revision:      APIString(v.Revision),
			Priority:      v.Priority,
			Activated:     v.Activated,
			ActivatedBy:   APIString(v.ActivatedBy),
			BuildId:       APIString(v.BuildId),
			DistroId:      APIString(v.DistroId),
			BuildVariant:  APIString(v.BuildVariant),
			DisplayName:   APIString(v.DisplayName),
			HostId:        APIString(v.HostId),
			Restarts:      v.Restarts,
			Execution:     v.Execution,
			Order:         v.RevisionOrderNumber,
			Details: apiTaskEndDetail{
				Status:      APIString(v.Details.Status),
				Type:        APIString(v.Details.Type),
				Description: APIString(v.Details.Description),
				TimedOut:    v.Details.TimedOut,
			},
			Status:           APIString(v.Status),
			TimeTaken:        v.TimeTaken,
			ExpectedDuration: v.ExpectedDuration,
		}

		if len(v.DependsOn) > 0 {
			dependsOn := make([]string, len(v.DependsOn))
			for i, dep := range v.DependsOn {
				dependsOn[i] = dep.TaskId
			}
			at.DependsOn = dependsOn
		}
	case string:
		ll := logLinks{
			AllLogLink:    APIString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "ALL")),
			TaskLogLink:   APIString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "T")),
			AgentLogLink:  APIString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "E")),
			SystemLogLink: APIString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "S")),
		}
		at.Logs = ll
	default:
		return errors.New("Incorrect type when unmarshalling task")
	}

	return nil
}

// ToService returns a service layer task using the data from the APITask.
func (ad *APITask) ToService() (interface{}, error) {
	st := &task.Task{
		Id:                  string(ad.Id),
		Project:             string(ad.Branch),
		CreateTime:          time.Time(ad.CreateTime),
		DispatchTime:        time.Time(ad.DispatchTime),
		PushTime:            time.Time(ad.PushTime),
		ScheduledTime:       time.Time(ad.ScheduledTime),
		StartTime:           time.Time(ad.StartTime),
		FinishTime:          time.Time(ad.FinishTime),
		Version:             string(ad.Version),
		Revision:            string(ad.Revision),
		Priority:            ad.Priority,
		Activated:           ad.Activated,
		ActivatedBy:         string(ad.ActivatedBy),
		BuildId:             string(ad.BuildId),
		DistroId:            string(ad.DistroId),
		BuildVariant:        string(ad.BuildVariant),
		DisplayName:         string(ad.DisplayName),
		HostId:              string(ad.HostId),
		Restarts:            ad.Restarts,
		Execution:           ad.Execution,
		RevisionOrderNumber: ad.Order,
		Details: apimodels.TaskEndDetail{
			Status:      string(ad.Details.Status),
			Type:        string(ad.Details.Type),
			Description: string(ad.Details.Description),
			TimedOut:    ad.Details.TimedOut,
		},
		Status:           string(ad.Status),
		TimeTaken:        ad.TimeTaken,
		ExpectedDuration: ad.ExpectedDuration,
	}
	dependsOn := make([]task.Dependency, len(ad.DependsOn))

	for i, depId := range ad.DependsOn {
		dependsOn[i].TaskId = depId
	}

	st.DependsOn = dependsOn
	return interface{}(st), nil
}
