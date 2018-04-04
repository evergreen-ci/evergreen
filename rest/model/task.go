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
	Id                 APIString        `json:"task_id"`
	CreateTime         APITime          `json:"create_time"`
	DispatchTime       APITime          `json:"dispatch_time"`
	ScheduledTime      APITime          `json:"scheduled_time"`
	StartTime          APITime          `json:"start_time"`
	FinishTime         APITime          `json:"finish_time"`
	Version            APIString        `json:"version_id"`
	Branch             APIString        `json:"branch"`
	Revision           APIString        `json:"revision"`
	Priority           int64            `json:"priority"`
	Activated          bool             `json:"activated"`
	ActivatedBy        APIString        `json:"activated_by"`
	BuildId            APIString        `json:"build_id"`
	DistroId           APIString        `json:"distro_id"`
	BuildVariant       APIString        `json:"build_variant"`
	DependsOn          []string         `json:"depends_on"`
	DisplayName        APIString        `json:"display_name"`
	HostId             APIString        `json:"host_id"`
	Restarts           int              `json:"restarts"`
	Execution          int              `json:"execution"`
	Order              int              `json:"order"`
	Status             APIString        `json:"status"`
	Details            apiTaskEndDetail `json:"status_details"`
	Logs               logLinks         `json:"logs"`
	TimeTaken          APIDuration      `json:"time_taken_ms"`
	ExpectedDuration   APIDuration      `json:"expected_duration_ms"`
	EstimatedCost      float64          `json:"estimated_cost"`
	PreviousExecutions []APITask        `json:"previous_executions,omitempty"`
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

func (at *APITask) BuildPreviousExecutions(tasks []task.Task) error {
	at.PreviousExecutions = make([]APITask, len(tasks))
	for i := range at.PreviousExecutions {
		if err := at.PreviousExecutions[i].BuildFromService(&tasks[i]); err != nil {
			return errors.Wrap(err, "error marshalling previous execution")
		}
	}

	return nil
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APITask.
func (at *APITask) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case *task.Task:
		(*at) = APITask{
			Id:            ToApiString(v.Id),
			CreateTime:    NewTime(v.CreateTime),
			DispatchTime:  NewTime(v.DispatchTime),
			ScheduledTime: NewTime(v.ScheduledTime),
			StartTime:     NewTime(v.StartTime),
			FinishTime:    NewTime(v.FinishTime),
			Version:       ToApiString(v.Version),
			Branch:        ToApiString(v.Project),
			Revision:      ToApiString(v.Revision),
			Priority:      v.Priority,
			Activated:     v.Activated,
			ActivatedBy:   ToApiString(v.ActivatedBy),
			BuildId:       ToApiString(v.BuildId),
			DistroId:      ToApiString(v.DistroId),
			BuildVariant:  ToApiString(v.BuildVariant),
			DisplayName:   ToApiString(v.DisplayName),
			HostId:        ToApiString(v.HostId),
			Restarts:      v.Restarts,
			Execution:     v.Execution,
			Order:         v.RevisionOrderNumber,
			Details: apiTaskEndDetail{
				Status:      ToApiString(v.Details.Status),
				Type:        ToApiString(v.Details.Type),
				Description: ToApiString(v.Details.Description),
				TimedOut:    v.Details.TimedOut,
			},
			Status:           ToApiString(v.Status),
			TimeTaken:        NewAPIDuration(v.TimeTaken),
			ExpectedDuration: NewAPIDuration(v.ExpectedDuration),
			EstimatedCost:    v.Cost,
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
			AllLogLink:    ToApiString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "ALL")),
			TaskLogLink:   ToApiString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "T")),
			AgentLogLink:  ToApiString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "E")),
			SystemLogLink: ToApiString(fmt.Sprintf(LogLinkFormat, v, at.Id, at.Execution, "S")),
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
		Id:                  FromApiString(ad.Id),
		Project:             FromApiString(ad.Branch),
		CreateTime:          time.Time(ad.CreateTime),
		DispatchTime:        time.Time(ad.DispatchTime),
		ScheduledTime:       time.Time(ad.ScheduledTime),
		StartTime:           time.Time(ad.StartTime),
		FinishTime:          time.Time(ad.FinishTime),
		Version:             FromApiString(ad.Version),
		Revision:            FromApiString(ad.Revision),
		Priority:            ad.Priority,
		Activated:           ad.Activated,
		ActivatedBy:         FromApiString(ad.ActivatedBy),
		BuildId:             FromApiString(ad.BuildId),
		DistroId:            FromApiString(ad.DistroId),
		BuildVariant:        FromApiString(ad.BuildVariant),
		DisplayName:         FromApiString(ad.DisplayName),
		HostId:              FromApiString(ad.HostId),
		Restarts:            ad.Restarts,
		Execution:           ad.Execution,
		RevisionOrderNumber: ad.Order,
		Details: apimodels.TaskEndDetail{
			Status:      FromApiString(ad.Details.Status),
			Type:        FromApiString(ad.Details.Type),
			Description: FromApiString(ad.Details.Description),
			TimedOut:    ad.Details.TimedOut,
		},
		Status:           FromApiString(ad.Status),
		TimeTaken:        ad.TimeTaken.ToDuration(),
		ExpectedDuration: ad.ExpectedDuration.ToDuration(),
		Cost:             ad.EstimatedCost,
	}
	dependsOn := make([]task.Dependency, len(ad.DependsOn))

	for i, depId := range ad.DependsOn {
		dependsOn[i].TaskId = depId
	}

	st.DependsOn = dependsOn
	return interface{}(st), nil
}

// APITaskCost is the model to be returned by the API whenever tasks
// for the cost route are fetched.
type APITaskCost struct {
	Id            APIString   `json:"task_id"`
	DisplayName   APIString   `json:"display_name"`
	DistroId      APIString   `json:"distro"`
	BuildVariant  APIString   `json:"build_variant"`
	TimeTaken     APIDuration `json:"time_taken"`
	Githash       APIString   `json:"githash"`
	EstimatedCost float64     `json:"estimated_cost"`
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APITaskCost. (It leaves out fields
// unnecessary for the route.)
func (atc *APITaskCost) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case task.Task:
		atc.Id = ToApiString(v.Id)
		atc.DisplayName = ToApiString(v.DisplayName)
		atc.DistroId = ToApiString(v.DistroId)
		atc.BuildVariant = ToApiString(v.BuildVariant)
		atc.TimeTaken = NewAPIDuration(v.TimeTaken)
		atc.Githash = ToApiString(v.Revision)
		atc.EstimatedCost = v.Cost
	default:
		return errors.New("Incorrect type when unmarshalling task")
	}
	return nil
}

// ToService returns a service layer version cost using the data from APIVersionCost.
func (atc *APITaskCost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APITaskCost")
}
