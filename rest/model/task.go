package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

const (
	LogLinkFormat = "%s/task_log_raw/%s/%d?type=%s"
)

// APITask is the model to be returned by the API whenever tasks are fetched.
type APITask struct {
	Id                 APIString        `json:"task_id"`
	ProjectId          APIString        `json:"project_id"`
	CreateTime         APITime          `json:"create_time"`
	DispatchTime       APITime          `json:"dispatch_time"`
	ScheduledTime      APITime          `json:"scheduled_time"`
	StartTime          APITime          `json:"start_time"`
	FinishTime         APITime          `json:"finish_time"`
	IngestTime         APITime          `json:"ingest_time"`
	Version            APIString        `json:"version_id"`
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
	EstimatedStart     APIDuration      `json:"est_wait_to_start_ms"`
	EstimatedCost      float64          `json:"estimated_cost"`
	PreviousExecutions []APITask        `json:"previous_executions,omitempty"`
	GenerateTask       bool             `json:"generate_task"`
	GeneratedBy        string           `json:"generated_by"`
	Artifacts          []APIFile        `json:"artifacts"`
	DisplayOnly        bool             `json:"display_only"`
	ExecutionTasks     []APIString      `json:"execution_tasks,omitempty"`
	Mainline           bool             `json:"mainline"`
	TaskGroup          string           `json:"task_group,omitempty"`
	TaskGroupMaxHosts  int              `json:"task_group_max_hosts,omitempty"`
	Blocked            bool             `json:"blocked"`
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
			Id:            ToAPIString(v.Id),
			ProjectId:     ToAPIString(v.Project),
			CreateTime:    NewTime(v.CreateTime),
			DispatchTime:  NewTime(v.DispatchTime),
			ScheduledTime: NewTime(v.ScheduledTime),
			StartTime:     NewTime(v.StartTime),
			FinishTime:    NewTime(v.FinishTime),
			IngestTime:    NewTime(v.IngestTime),
			Version:       ToAPIString(v.Version),
			Revision:      ToAPIString(v.Revision),
			Priority:      v.Priority,
			Activated:     v.Activated,
			ActivatedBy:   ToAPIString(v.ActivatedBy),
			BuildId:       ToAPIString(v.BuildId),
			DistroId:      ToAPIString(v.DistroId),
			BuildVariant:  ToAPIString(v.BuildVariant),
			DisplayName:   ToAPIString(v.DisplayName),
			HostId:        ToAPIString(v.HostId),
			Restarts:      v.Restarts,
			Execution:     v.Execution,
			Order:         v.RevisionOrderNumber,
			Details: apiTaskEndDetail{
				Status:      ToAPIString(v.Details.Status),
				Type:        ToAPIString(v.Details.Type),
				Description: ToAPIString(v.Details.Description),
				TimedOut:    v.Details.TimedOut,
			},
			Status:            ToAPIString(v.Status),
			TimeTaken:         NewAPIDuration(v.TimeTaken),
			ExpectedDuration:  NewAPIDuration(v.ExpectedDuration),
			EstimatedCost:     v.Cost,
			GenerateTask:      v.GenerateTask,
			GeneratedBy:       v.GeneratedBy,
			DisplayOnly:       v.DisplayOnly,
			Mainline:          (v.Requester == evergreen.RepotrackerVersionRequester),
			TaskGroup:         v.TaskGroup,
			TaskGroupMaxHosts: v.TaskGroupMaxHosts,
			Blocked:           v.Blocked(),
		}
		if len(v.ExecutionTasks) > 0 {
			ets := []APIString{}
			for _, t := range v.ExecutionTasks {
				ets = append(ets, ToAPIString(t))
			}
			at.ExecutionTasks = ets
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
			AllLogLink:    ToAPIString(fmt.Sprintf(LogLinkFormat, v, FromAPIString(at.Id), at.Execution, "ALL")),
			TaskLogLink:   ToAPIString(fmt.Sprintf(LogLinkFormat, v, FromAPIString(at.Id), at.Execution, "T")),
			AgentLogLink:  ToAPIString(fmt.Sprintf(LogLinkFormat, v, FromAPIString(at.Id), at.Execution, "E")),
			SystemLogLink: ToAPIString(fmt.Sprintf(LogLinkFormat, v, FromAPIString(at.Id), at.Execution, "S")),
		}
		at.Logs = ll
	default:
		return errors.New(fmt.Sprintf("Incorrect type %T when unmarshalling task", t))
	}

	return nil
}

// ToService returns a service layer task using the data from the APITask.
func (ad *APITask) ToService() (interface{}, error) {
	st := &task.Task{
		Id:                  FromAPIString(ad.Id),
		Project:             FromAPIString(ad.ProjectId),
		CreateTime:          time.Time(ad.CreateTime),
		DispatchTime:        time.Time(ad.DispatchTime),
		ScheduledTime:       time.Time(ad.ScheduledTime),
		StartTime:           time.Time(ad.StartTime),
		FinishTime:          time.Time(ad.FinishTime),
		IngestTime:          time.Time(ad.IngestTime),
		Version:             FromAPIString(ad.Version),
		Revision:            FromAPIString(ad.Revision),
		Priority:            ad.Priority,
		Activated:           ad.Activated,
		ActivatedBy:         FromAPIString(ad.ActivatedBy),
		BuildId:             FromAPIString(ad.BuildId),
		DistroId:            FromAPIString(ad.DistroId),
		BuildVariant:        FromAPIString(ad.BuildVariant),
		DisplayName:         FromAPIString(ad.DisplayName),
		HostId:              FromAPIString(ad.HostId),
		Restarts:            ad.Restarts,
		Execution:           ad.Execution,
		RevisionOrderNumber: ad.Order,
		Details: apimodels.TaskEndDetail{
			Status:      FromAPIString(ad.Details.Status),
			Type:        FromAPIString(ad.Details.Type),
			Description: FromAPIString(ad.Details.Description),
			TimedOut:    ad.Details.TimedOut,
		},
		Status:           FromAPIString(ad.Status),
		TimeTaken:        ad.TimeTaken.ToDuration(),
		ExpectedDuration: ad.ExpectedDuration.ToDuration(),
		Cost:             ad.EstimatedCost,
		GenerateTask:     ad.GenerateTask,
		GeneratedBy:      ad.GeneratedBy,
		DisplayOnly:      ad.DisplayOnly,
	}
	if len(ad.ExecutionTasks) > 0 {
		ets := []string{}
		for _, t := range ad.ExecutionTasks {
			ets = append(ets, FromAPIString(t))
		}
		st.ExecutionTasks = ets
	}

	dependsOn := make([]task.Dependency, len(ad.DependsOn))

	for i, depId := range ad.DependsOn {
		dependsOn[i].TaskId = depId
	}

	st.DependsOn = dependsOn
	return interface{}(st), nil
}

func (at *APITask) GetArtifacts() error {
	var err error
	var entries []artifact.Entry
	if at.DisplayOnly {
		ets := []string{}
		for _, t := range at.ExecutionTasks {
			ets = append(ets, FromAPIString(t))
		}
		entries, err = artifact.FindAll(artifact.ByTaskIds(ets))
	} else {
		entries, err = artifact.FindAll(artifact.ByTaskId(FromAPIString(at.Id)))
	}
	if err != nil {
		return errors.Wrap(err, "error retrieving artifacts")
	}
	for _, entry := range entries {
		for _, file := range entry.Files {
			apiFile := APIFile{}
			err := apiFile.BuildFromService(file)
			if err != nil {
				return err
			}
			at.Artifacts = append(at.Artifacts, apiFile)
		}
	}

	return nil
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
		atc.Id = ToAPIString(v.Id)
		atc.DisplayName = ToAPIString(v.DisplayName)
		atc.DistroId = ToAPIString(v.DistroId)
		atc.BuildVariant = ToAPIString(v.BuildVariant)
		atc.TimeTaken = NewAPIDuration(v.TimeTaken)
		atc.Githash = ToAPIString(v.Revision)
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
