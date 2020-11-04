package model

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	TaskLogLinkFormat  = "%s/task_log_raw/%s/%d?type=%s"
	EventLogLinkFormat = "%s/event_log/task/%s"
)

// APITask is the model to be returned by the API whenever tasks are fetched.
type APITask struct {
	Id                 *string             `json:"task_id"`
	ProjectId          *string             `json:"project_id"`
	CreateTime         *time.Time          `json:"create_time"`
	DispatchTime       *time.Time          `json:"dispatch_time"`
	ScheduledTime      *time.Time          `json:"scheduled_time"`
	StartTime          *time.Time          `json:"start_time"`
	FinishTime         *time.Time          `json:"finish_time"`
	IngestTime         *time.Time          `json:"ingest_time"`
	ActivatedTime      *time.Time          `json:"activated_time"`
	Version            *string             `json:"version_id"`
	Revision           *string             `json:"revision"`
	Priority           int64               `json:"priority"`
	Activated          bool                `json:"activated"`
	ActivatedBy        *string             `json:"activated_by"`
	BuildId            *string             `json:"build_id"`
	DistroId           *string             `json:"distro_id"`
	BuildVariant       *string             `json:"build_variant"`
	DependsOn          []APIDependency     `json:"depends_on"`
	DisplayName        *string             `json:"display_name"`
	HostId             *string             `json:"host_id"`
	Restarts           int                 `json:"restarts"`
	Execution          int                 `json:"execution"`
	Order              int                 `json:"order"`
	Status             *string             `json:"status"`
	DisplayStatus      *string             `json:"display_status"`
	Details            ApiTaskEndDetail    `json:"status_details"`
	Logs               LogLinks            `json:"logs"`
	TimeTaken          APIDuration         `json:"time_taken_ms"`
	ExpectedDuration   APIDuration         `json:"expected_duration_ms"`
	EstimatedStart     APIDuration         `json:"est_wait_to_start_ms"`
	EstimatedCost      float64             `json:"estimated_cost"`
	PreviousExecutions []APITask           `json:"previous_executions,omitempty"`
	GenerateTask       bool                `json:"generate_task"`
	GeneratedBy        string              `json:"generated_by"`
	Artifacts          []APIFile           `json:"artifacts"`
	DisplayOnly        bool                `json:"display_only"`
	ExecutionTasks     []*string           `json:"execution_tasks,omitempty"`
	Mainline           bool                `json:"mainline"`
	TaskGroup          string              `json:"task_group,omitempty"`
	TaskGroupMaxHosts  int                 `json:"task_group_max_hosts,omitempty"`
	Blocked            bool                `json:"blocked"`
	Requester          *string             `json:"requester"`
	TestResults        []APITest           `json:"test_results"`
	Aborted            bool                `json:"aborted"`
	AbortInfo          APIAbortInfo        `json:abort_info`
	CanSync            bool                `json:"can_sync,omitempty"`
	SyncAtEndOpts      APISyncAtEndOptions `json:"sync_at_end_opts"`
	Ami                *string             `json:"ami"`
	MustHaveResults    bool                `json:"must_have_test_results"`
}

type APIAbortInfo struct {
	User       string `json:"user,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	NewVersion string `json:"new_version,omitempty"`
	PRClosed   bool   `json:"pr_closed,omitempty"`
}

type LogLinks struct {
	AllLogLink    *string `json:"all_log"`
	TaskLogLink   *string `json:"task_log"`
	AgentLogLink  *string `json:"agent_log"`
	SystemLogLink *string `json:"system_log"`
	EventLogLink  *string `json:"event_log"`
}

type ApiTaskEndDetail struct {
	Status      *string           `json:"status"`
	Type        *string           `json:"type"`
	Description *string           `json:"desc"`
	TimedOut    bool              `json:"timed_out"`
	OOMTracker  APIOomTrackerInfo `json:"oom_tracker_info"`
}

func (at *ApiTaskEndDetail) BuildFromService(t interface{}) error {
	v, ok := t.(apimodels.TaskEndDetail)
	if !ok {
		return errors.Errorf("Incorrect type %T when unmarshalling TaskEndDetail", t)
	}
	at.Status = ToStringPtr(v.Status)
	at.Type = ToStringPtr(v.Type)
	at.Description = ToStringPtr(v.Description)
	at.TimedOut = v.TimedOut

	apiOomTracker := APIOomTrackerInfo{}
	if err := apiOomTracker.BuildFromService(v.OOMTracker); err != nil {
		return errors.Wrap(err, "can't build oom tracker from service")
	}
	at.OOMTracker = apiOomTracker

	return nil
}

func (ad *ApiTaskEndDetail) ToService() (interface{}, error) {
	detail := apimodels.TaskEndDetail{
		Status:      FromStringPtr(ad.Status),
		Type:        FromStringPtr(ad.Type),
		Description: FromStringPtr(ad.Description),
		TimedOut:    ad.TimedOut,
	}
	oomTrackerIface, err := ad.OOMTracker.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "can't convert OOMTrackerInfo to service")
	}
	detail.OOMTracker = oomTrackerIface.(*apimodels.OOMTrackerInfo)

	return detail, nil
}

type APIOomTrackerInfo struct {
	Detected bool  `json:"detected"`
	Pids     []int `json:"pids"`
}

func (at *APIOomTrackerInfo) BuildFromService(t interface{}) error {
	v, ok := t.(*apimodels.OOMTrackerInfo)
	if !ok {
		return errors.Errorf("Incorrect type %T when unmarshalling OOMTrackerInfo", t)
	}
	if v != nil {
		at.Detected = v.Detected
		at.Pids = v.Pids
	}

	return nil
}

func (ad *APIOomTrackerInfo) ToService() (interface{}, error) {
	return &apimodels.OOMTrackerInfo{
		Detected: ad.Detected,
		Pids:     ad.Pids,
	}, nil
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
		id := v.Id
		// Old tasks are stored in a separate collection with ID set to
		// "old_task_ID" + "_" + "execution_number". This ID is not exposed to the user,
		// however. Instead in the UI executions are represented with a "/" and could be
		// represented in other ways elsewhere. The correct way to represent an old task is
		// with the same ID as the last execution, since semantically the tasks differ in
		// their execution number, not in their ID.
		if v.OldTaskId != "" {
			id = v.OldTaskId
		}
		(*at) = APITask{
			Id:                ToStringPtr(id),
			ProjectId:         ToStringPtr(v.Project),
			CreateTime:        ToTimePtr(v.CreateTime),
			DispatchTime:      ToTimePtr(v.DispatchTime),
			ScheduledTime:     ToTimePtr(v.ScheduledTime),
			StartTime:         ToTimePtr(v.StartTime),
			FinishTime:        ToTimePtr(v.FinishTime),
			IngestTime:        ToTimePtr(v.IngestTime),
			ActivatedTime:     ToTimePtr(v.ActivatedTime),
			Version:           ToStringPtr(v.Version),
			Revision:          ToStringPtr(v.Revision),
			Priority:          v.Priority,
			Activated:         v.Activated,
			ActivatedBy:       ToStringPtr(v.ActivatedBy),
			BuildId:           ToStringPtr(v.BuildId),
			DistroId:          ToStringPtr(v.DistroId),
			BuildVariant:      ToStringPtr(v.BuildVariant),
			DisplayName:       ToStringPtr(v.DisplayName),
			HostId:            ToStringPtr(v.HostId),
			Restarts:          v.Restarts,
			Execution:         v.Execution,
			Order:             v.RevisionOrderNumber,
			Status:            ToStringPtr(v.Status),
			DisplayStatus:     ToStringPtr(v.GetDisplayStatus()),
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
			CanSync:           v.CanSync,
			MustHaveResults:   v.MustHaveResults,
			SyncAtEndOpts: APISyncAtEndOptions{
				Enabled:  v.SyncAtEndOpts.Enabled,
				Statuses: v.SyncAtEndOpts.Statuses,
				Timeout:  v.SyncAtEndOpts.Timeout,
			},
			AbortInfo: APIAbortInfo{
				NewVersion: v.AbortInfo.NewVersion,
				TaskID:     v.AbortInfo.TaskID,
				User:       v.AbortInfo.User,
				PRClosed:   v.AbortInfo.PRClosed,
			},
		}

		if err := at.Details.BuildFromService(v.Details); err != nil {
			return errors.Wrap(err, "can't build TaskEndDetail from service")
		}

		if v.HostId != "" {
			h, err := host.FindOneId(v.HostId)
			if err != nil {
				return errors.Wrapf(err, "error finding host '%s' for task", v.HostId)
			}
			if h != nil && len(h.Distro.ProviderSettingsList) == 1 {
				ami, ok := h.Distro.ProviderSettingsList[0].Lookup("ami").StringValueOK()
				if ok {
					at.Ami = ToStringPtr(ami)
				}
			}
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
			AllLogLink:    ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "ALL")),
			TaskLogLink:   ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "T")),
			AgentLogLink:  ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "E")),
			SystemLogLink: ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, v, FromStringPtr(at.Id), at.Execution, "S")),
			EventLogLink:  ToStringPtr(fmt.Sprintf(EventLogLinkFormat, v, FromStringPtr(at.Id))),
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
		Status:              FromStringPtr(ad.Status),
		DisplayStatus:       FromStringPtr(ad.DisplayStatus),
		TimeTaken:           ad.TimeTaken.ToDuration(),
		ExpectedDuration:    ad.ExpectedDuration.ToDuration(),
		Cost:                ad.EstimatedCost,
		GenerateTask:        ad.GenerateTask,
		GeneratedBy:         ad.GeneratedBy,
		DisplayOnly:         ad.DisplayOnly,
		Requester:           FromStringPtr(ad.Requester),
		CanSync:             ad.CanSync,
		MustHaveResults:     ad.MustHaveResults,
		SyncAtEndOpts: task.SyncAtEndOptions{
			Enabled:  ad.SyncAtEndOpts.Enabled,
			Statuses: ad.SyncAtEndOpts.Statuses,
			Timeout:  ad.SyncAtEndOpts.Timeout,
		},
	}
	catcher := grip.NewBasicCatcher()
	serviceDetails, err := ad.Details.ToService()
	catcher.Add(err)
	st.Details = serviceDetails.(apimodels.TaskEndDetail)
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
		ets := []artifact.TaskIDAndExecution{}
		for _, t := range at.ExecutionTasks {
			ets = append(ets, artifact.TaskIDAndExecution{TaskID: *t, Execution: at.Execution})
		}
		if len(ets) > 0 {
			entries, err = artifact.FindAll(artifact.ByTaskIdsAndExecutions(ets))
		}
	} else {
		entries, err = artifact.FindAll(artifact.ByTaskIdAndExecution(FromStringPtr(at.Id), at.Execution))
	}
	if err != nil {
		return errors.Wrap(err, "error retrieving artifacts")
	}
	for _, entry := range entries {
		var strippedFiles []artifact.File
		// The route requires a user, so hasUser is always true.
		strippedFiles, err = artifact.StripHiddenFiles(entry.Files, true)
		if err != nil {
			return err
		}
		for _, file := range strippedFiles {
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

type APISyncAtEndOptions struct {
	Enabled  bool          `json:"enabled"`
	Statuses []string      `json:"statuses"`
	Timeout  time.Duration `json:"timeout"`
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
	return nil, errors.New("ToService() is not implemented for APITaskCost")
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
	return nil, errors.New("ToService() is not implemented for APIDependency")
}
