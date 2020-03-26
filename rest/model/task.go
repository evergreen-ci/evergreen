package model

import (
	"fmt"
	"time"

	"github.com/mongodb/grip"

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
	Id                 *string          `json:"task_id"`
	ProjectId          *string          `json:"project_id"`
	CreateTime         *time.Time       `json:"create_time"`
	DispatchTime       *time.Time       `json:"dispatch_time"`
	ScheduledTime      *time.Time       `json:"scheduled_time"`
	StartTime          *time.Time       `json:"start_time"`
	FinishTime         *time.Time       `json:"finish_time"`
	IngestTime         *time.Time       `json:"ingest_time"`
	ActivatedTime      *time.Time       `json:"activated_time"`
	Version            *string          `json:"version_id"`
	Revision           *string          `json:"revision"`
	Priority           int64            `json:"priority"`
	Activated          bool             `json:"activated"`
	ActivatedBy        *string          `json:"activated_by"`
	BuildId            *string          `json:"build_id"`
	DistroId           *string          `json:"distro_id"`
	BuildVariant       *string          `json:"build_variant"`
	DependsOn          []APIDependency  `json:"depends_on"`
	DisplayName        *string          `json:"display_name"`
	HostId             *string          `json:"host_id"`
	Restarts           int              `json:"restarts"`
	Execution          int              `json:"execution"`
	Order              int              `json:"order"`
	Status             *string          `json:"status"`
	Details            ApiTaskEndDetail `json:"status_details"`
	Logs               LogLinks         `json:"logs"`
	TimeTaken          APIDuration      `json:"time_taken_ms"`
	ExpectedDuration   APIDuration      `json:"expected_duration_ms"`
	EstimatedStart     APIDuration      `json:"est_wait_to_start_ms"`
	EstimatedCost      float64          `json:"estimated_cost"`
	PreviousExecutions []APITask        `json:"previous_executions,omitempty"`
	GenerateTask       bool             `json:"generate_task"`
	GeneratedBy        string           `json:"generated_by"`
	Artifacts          []APIFile        `json:"artifacts"`
	DisplayOnly        bool             `json:"display_only"`
	ExecutionTasks     []*string        `json:"execution_tasks,omitempty"`
	Mainline           bool             `json:"mainline"`
	TaskGroup          string           `json:"task_group,omitempty"`
	TaskGroupMaxHosts  int              `json:"task_group_max_hosts,omitempty"`
	Blocked            bool             `json:"blocked"`
	Requester          *string          `json:"requester"`
	TestResults        []APITest        `json:"test_results"`
	Aborted            bool             `json:"aborted"`
}

type LogLinks struct {
	AllLogLink    *string `json:"all_log"`
	TaskLogLink   *string `json:"task_log"`
	AgentLogLink  *string `json:"agent_log"`
	SystemLogLink *string `json:"system_log"`
}

type ApiTaskEndDetail struct {
	Status      *string `json:"status"`
	Type        *string `json:"type"`
	Description *string `json:"desc"`
	TimedOut    bool    `json:"timed_out"`
}

func (at *APITask) BuildPreviousExecutions(tasks []task.Task, url string) error {
	at.PreviousExecutions = make([]APITask, len(tasks))
	for i := range at.PreviousExecutions {
		if err := at.PreviousExecutions[i].BuildFromService(&tasks[i]); err != nil {
			return errors.Wrap(err, "error marshalling previous execution")
		}
		if err := at.PreviousExecutions[i].BuildFromService(url); err != nil {
			return errors.Wrap(err, "failed to build logs for previous execution")
		}
		if err := at.PreviousExecutions[i].GetArtifacts(); err != nil {
			return errors.Wrap(err, "failed to fetch artifacts for previous executions")
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
			Id:            ToStringPtr(v.Id),
			ProjectId:     ToStringPtr(v.Project),
			CreateTime:    ToTimePtr(v.CreateTime),
			DispatchTime:  ToTimePtr(v.DispatchTime),
			ScheduledTime: ToTimePtr(v.ScheduledTime),
			StartTime:     ToTimePtr(v.StartTime),
			FinishTime:    ToTimePtr(v.FinishTime),
			IngestTime:    ToTimePtr(v.IngestTime),
			ActivatedTime: ToTimePtr(v.ActivatedTime),
			Version:       ToStringPtr(v.Version),
			Revision:      ToStringPtr(v.Revision),
			Priority:      v.Priority,
			Activated:     v.Activated,
			ActivatedBy:   ToStringPtr(v.ActivatedBy),
			BuildId:       ToStringPtr(v.BuildId),
			DistroId:      ToStringPtr(v.DistroId),
			BuildVariant:  ToStringPtr(v.BuildVariant),
			DisplayName:   ToStringPtr(v.DisplayName),
			HostId:        ToStringPtr(v.HostId),
			Restarts:      v.Restarts,
			Execution:     v.Execution,
			Order:         v.RevisionOrderNumber,
			Details: ApiTaskEndDetail{
				Status:      ToStringPtr(v.Details.Status),
				Type:        ToStringPtr(v.Details.Type),
				Description: ToStringPtr(v.Details.Description),
				TimedOut:    v.Details.TimedOut,
			},
			Status:            ToStringPtr(v.Status),
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
			Requester:         ToStringPtr(v.Requester),
			Aborted:           v.Aborted,
		}
		if len(v.ExecutionTasks) > 0 {
			ets := []*string{}
			for _, t := range v.ExecutionTasks {
				ets = append(ets, ToStringPtr(t))
			}
			at.ExecutionTasks = ets
		}

		if len(v.DependsOn) > 0 {
			dependsOn := make([]APIDependency, len(v.DependsOn))
			for i, dep := range v.DependsOn {
				apiDep := APIDependency{}
				apiDep.BuildFromService(dep)
				dependsOn[i] = apiDep
			}
			at.DependsOn = dependsOn
		}
	case string:
		ll := LogLinks{
			AllLogLink:    ToStringPtr(fmt.Sprintf(LogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "ALL")),
			TaskLogLink:   ToStringPtr(fmt.Sprintf(LogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "T")),
			AgentLogLink:  ToStringPtr(fmt.Sprintf(LogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "E")),
			SystemLogLink: ToStringPtr(fmt.Sprintf(LogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "S")),
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
		Id:                  FromStringPtr(ad.Id),
		Project:             FromStringPtr(ad.ProjectId),
		Version:             FromStringPtr(ad.Version),
		Revision:            FromStringPtr(ad.Revision),
		Priority:            ad.Priority,
		Activated:           ad.Activated,
		ActivatedBy:         FromStringPtr(ad.ActivatedBy),
		BuildId:             FromStringPtr(ad.BuildId),
		DistroId:            FromStringPtr(ad.DistroId),
		BuildVariant:        FromStringPtr(ad.BuildVariant),
		DisplayName:         FromStringPtr(ad.DisplayName),
		HostId:              FromStringPtr(ad.HostId),
		Restarts:            ad.Restarts,
		Execution:           ad.Execution,
		RevisionOrderNumber: ad.Order,
		Details: apimodels.TaskEndDetail{
			Status:      FromStringPtr(ad.Details.Status),
			Type:        FromStringPtr(ad.Details.Type),
			Description: FromStringPtr(ad.Details.Description),
			TimedOut:    ad.Details.TimedOut,
		},
		Status:           FromStringPtr(ad.Status),
		TimeTaken:        ad.TimeTaken.ToDuration(),
		ExpectedDuration: ad.ExpectedDuration.ToDuration(),
		Cost:             ad.EstimatedCost,
		GenerateTask:     ad.GenerateTask,
		GeneratedBy:      ad.GeneratedBy,
		DisplayOnly:      ad.DisplayOnly,
		Requester:        FromStringPtr(ad.Requester),
	}
	catcher := grip.NewBasicCatcher()
	createTime, err := FromTimePtr(ad.CreateTime)
	catcher.Add(err)
	dispatchTime, err := FromTimePtr(ad.DispatchTime)
	catcher.Add(err)
	scheduledTime, err := FromTimePtr(ad.ScheduledTime)
	catcher.Add(err)
	startTime, err := FromTimePtr(ad.StartTime)
	catcher.Add(err)
	finishTime, err := FromTimePtr(ad.FinishTime)
	catcher.Add(err)
	ingestTime, err := FromTimePtr(ad.IngestTime)
	catcher.Add(err)
	activatedTime, err := FromTimePtr(ad.ActivatedTime)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	st.CreateTime = createTime
	st.DispatchTime = dispatchTime
	st.ScheduledTime = scheduledTime
	st.StartTime = startTime
	st.FinishTime = finishTime
	st.IngestTime = ingestTime
	st.ActivatedTime = activatedTime
	if len(ad.ExecutionTasks) > 0 {
		ets := []string{}
		for _, t := range ad.ExecutionTasks {
			ets = append(ets, FromStringPtr(t))
		}
		st.ExecutionTasks = ets
	}

	dependsOn := make([]task.Dependency, len(ad.DependsOn))

	for i, dep := range ad.DependsOn {
		dependsOn[i].TaskId = dep.TaskId
		dependsOn[i].Status = dep.Status
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
			ets = append(ets, FromStringPtr(t))
		}
		entries, err = artifact.FindAll(artifact.ByTaskIds(ets))
	} else {
		entries, err = artifact.FindAll(artifact.ByTaskId(FromStringPtr(at.Id)))
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
	Id            *string     `json:"task_id"`
	DisplayName   *string     `json:"display_name"`
	DistroId      *string     `json:"distro"`
	BuildVariant  *string     `json:"build_variant"`
	TimeTaken     APIDuration `json:"time_taken"`
	Githash       *string     `json:"githash"`
	EstimatedCost float64     `json:"estimated_cost"`
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APITaskCost. (It leaves out fields
// unnecessary for the route.)
func (atc *APITaskCost) BuildFromService(t interface{}) error {
	switch v := t.(type) {
	case task.Task:
		atc.Id = ToStringPtr(v.Id)
		atc.DisplayName = ToStringPtr(v.DisplayName)
		atc.DistroId = ToStringPtr(v.DistroId)
		atc.BuildVariant = ToStringPtr(v.BuildVariant)
		atc.TimeTaken = NewAPIDuration(v.TimeTaken)
		atc.Githash = ToStringPtr(v.Revision)
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

type APIDependency struct {
	TaskId string `bson:"_id" json:"id"`
	Status string `bson:"status" json:"status"`
}

func (ad *APIDependency) BuildFromService(dep task.Dependency) {
	ad.TaskId = dep.TaskId
	ad.Status = dep.Status
}
func (ad *APIDependency) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for APIDependency")
}
