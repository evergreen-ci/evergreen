package model

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	TaskLogLinkFormat        = "%s/task_log_raw/%s/%d?type=%s"
	ParsleyTaskLogLinkFormat = "%s/evergreen/%s/%d/%s"
)

// APITask is the model to be returned by the API whenever tasks are fetched.
type APITask struct {
	// Unique identifier of this task
	Id                *string `json:"task_id"`
	ProjectId         *string `json:"project_id"`
	ProjectIdentifier *string `json:"project_identifier"`
	// Time that this task was first created
	CreateTime *time.Time `json:"create_time"`
	// Time that this time was dispatched
	DispatchTime *time.Time `json:"dispatch_time"`
	// Time that this task is scheduled to begin
	ScheduledTime *time.Time `json:"scheduled_time"`
	// Time that this task began execution
	StartTime *time.Time `json:"start_time"`
	// Time that this task finished execution
	FinishTime    *time.Time `json:"finish_time"`
	IngestTime    *time.Time `json:"ingest_time"`
	ActivatedTime *time.Time `json:"activated_time"`
	// An identifier of this task by its project and commit hash
	Version *string `json:"version_id"`
	// The version control identifier associated with this task
	Revision *string `json:"revision"`
	// The priority of this task to be run
	Priority int64 `json:"priority"`
	// Whether the task is currently active
	Activated bool `json:"activated"`
	// The information, if any, about stepback
	StepbackInfo *APIStepbackInfo `json:"stepback_info"`
	// Identifier of the process or user that activated this task
	ActivatedBy *string `json:"activated_by"`
	// Identifier of the build that this task is part of
	BuildId *string `json:"build_id"`
	// Identifier of the distro that this task runs on
	DistroId *string `json:"distro_id"`
	// Name of the buildvariant that this task runs on
	BuildVariant            *string `json:"build_variant"`
	BuildVariantDisplayName *string `json:"build_variant_display_name"`
	// List of task_ids of task that this task depends on before beginning
	DependsOn []APIDependency `json:"depends_on"`
	// Name of this task displayed in the UI
	DisplayName *string `json:"display_name"`
	// The ID of the host this task ran or is running on
	HostId *string `json:"host_id"`
	// The number of the execution of this particular task
	Execution int `json:"execution"`
	// For mainline commits, represents the position in the commit history of
	// commit this task is associated with. For patches, this represents the
	// number of total patches submitted by the user.
	Order int `json:"order"`
	// The current status of this task (possible values are "undispatched",
	// "dispatched", "started", "success", and "failed")
	Status *string `json:"status"`
	// The status of this task that is displayed in the UI (possible values are
	// "will-run", "unscheduled", "blocked", "dispatched", "started", "success",
	// "failed", "aborted", "system-failed", "system-unresponsive",
	// "system-timed-out", "task-timed-out", "known-issue")
	DisplayStatus *string `json:"display_status"`
	// Object containing additional information about the status
	Details ApiTaskEndDetail `json:"status_details"`
	// Object containing raw and event logs for this task
	Logs LogLinks `json:"logs"`
	// Object containing parsley logs for this task
	ParsleyLogs LogLinks `json:"parsley_logs"`
	// Number of milliseconds this task took during execution
	TimeTaken APIDuration `json:"time_taken_ms"`
	// Cost breakdown for running this task
	TaskCost *cost.Cost `json:"task_cost,omitempty"`
	// Predicted cost breakdown based on historical task costs
	PredictedTaskCost *cost.Cost `json:"predicted_task_cost,omitempty"`
	// Number of milliseconds expected for this task to execute
	ExpectedDuration APIDuration `json:"expected_duration_ms"`
	EstimatedStart   APIDuration `json:"est_wait_to_start_ms"`
	// Contains previous executions of the task if they were requested, and
	// available. May be empty
	PreviousExecutions []APITask `json:"previous_executions,omitempty"`
	GenerateTask       bool      `json:"generate_task"`
	GeneratedBy        string    `json:"generated_by"`
	// The list of artifacts associated with the task.
	Artifacts   []APIFile `json:"artifacts"`
	DisplayOnly bool      `json:"display_only"`
	// The ID of the task's parent display task, if requested and available
	ParentTaskId   string    `json:"parent_task_id"`
	ExecutionTasks []*string `json:"execution_tasks,omitempty"`
	// List of tags defined for the task, if any
	Tags              []*string `json:"tags,omitempty"`
	Mainline          bool      `json:"mainline"`
	TaskGroup         string    `json:"task_group,omitempty"`
	TaskGroupMaxHosts int       `json:"task_group_max_hosts,omitempty"`
	Blocked           bool      `json:"blocked"`
	// Version created by one of patch_request", "github_pull_request",
	// "gitter_request" (caused by git commit, aka the repotracker requester),
	// "trigger_request" (Project Trigger versions) , "merge_test" (commit queue
	// patches), "ad_hoc" (periodic builds)
	Requester            *string         `json:"requester"`
	TestResults          []APITest       `json:"test_results"`
	Aborted              bool            `json:"aborted"`
	AbortInfo            APIAbortInfo    `json:"abort_info"`
	AMI                  *string         `json:"ami"`
	MustHaveResults      bool            `json:"must_have_test_results"`
	BaseTask             APIBaseTaskInfo `json:"base_task"`
	ResetWhenFinished    bool            `json:"reset_when_finished"`
	HasAnnotations       bool            `json:"has_annotations"`
	TestSelectionEnabled bool            `json:"test_selection_enabled"`
	// These fields are used by graphql gen, but do not need to be exposed
	// via Evergreen's user-facing API.
	OverrideDependencies bool `json:"-"`
	Archived             bool `json:"archived"`
	HasTestResults       bool `json:"-"`
	ResultsFailed        bool `json:"-"`
}

type APIStepbackInfo struct {
	LastFailingStepbackTaskId string `json:"last_failing_stepback_task_id"`
	LastPassingStepbackTaskId string `json:"last_passing_stepback_task_id"`
	NextStepbackTaskId        string `json:"next_stepback_task_id"`
	PreviousStepbackTaskId    string `json:"previous_stepback_task_id"`
}

type APIAbortInfo struct {
	User       string `json:"user,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	NewVersion string `json:"new_version,omitempty"`
	PRClosed   bool   `json:"pr_closed,omitempty"`
}

type LogLinks struct {
	// Link to logs containing merged copy of all other logs
	AllLogLink *string `json:"all_log"`
	// Link to logs created by the task execution
	TaskLogLink *string `json:"task_log"`
	// Link to logs created by the agent process
	AgentLogLink *string `json:"agent_log"`
	// Link to logs created by the machine running the task
	SystemLogLink *string `json:"system_log"`
}

type ApiTaskEndDetail struct {
	// The status of the completed task.
	Status *string `json:"status"`
	// The method by which the task failed.
	Type *string `json:"type"`
	// Description of the final status of this task.
	Description *string `json:"desc"`
	// Command which indiciates the task failure.
	FailingCommand *string `json:"failing_command"`
	// PostErrored is true when the post command errored.
	PostErrored bool `json:"post_errored"`
	// FailureMetadataTags contains the metadata tags associated with the
	// command that caused the task to fail. These are not set if the task
	// succeeded.
	FailureMetadataTags []string `json:"failure_metadata_tags"`
	// OtherFailingCommands contain information about commands that failed but
	// did not cause the task to fail.
	OtherFailingCommands []APIFailingCommand `json:"other_failing_commands,omitempty"`
	// Whether this task ended in a timeout.
	TimedOut    bool              `json:"timed_out"`
	TimeoutType *string           `json:"timeout_type"`
	OOMTracker  APIOomTrackerInfo `json:"oom_tracker_info"`
	TraceID     *string           `json:"trace_id"`
	DiskDevices []string          `json:"disk_devices"`
}

func (at *ApiTaskEndDetail) BuildFromService(t apimodels.TaskEndDetail) error {
	at.Status = utility.ToStringPtr(t.Status)
	at.Type = utility.ToStringPtr(t.Type)
	// TODO DEVPROD-9694: Stop storing failing command in Description
	if t.Description == "" {
		at.Description = utility.ToStringPtr(t.FailingCommand)
	} else {
		at.Description = utility.ToStringPtr(t.Description)
	}
	at.FailingCommand = utility.ToStringPtr(t.FailingCommand)
	at.PostErrored = t.PostErrored
	at.TimedOut = t.TimedOut
	at.TimeoutType = utility.ToStringPtr(t.TimeoutType)
	at.FailureMetadataTags = t.FailureMetadataTags
	for _, failingCmd := range t.OtherFailingCommands {
		var apiFailingCmd APIFailingCommand
		apiFailingCmd.BuildFromService(failingCmd)
		at.OtherFailingCommands = append(at.OtherFailingCommands, apiFailingCmd)
	}

	apiOomTracker := APIOomTrackerInfo{}
	apiOomTracker.BuildFromService(t.OOMTracker)
	at.OOMTracker = apiOomTracker
	at.TraceID = utility.ToStringPtr(t.TraceID)
	at.DiskDevices = t.DiskDevices

	return nil
}

func (ad *ApiTaskEndDetail) ToService() apimodels.TaskEndDetail {
	failingCmds := make([]apimodels.FailingCommand, 0, len(ad.OtherFailingCommands))
	for _, failingCmd := range ad.OtherFailingCommands {
		failingCmds = append(failingCmds, failingCmd.ToService())
	}
	return apimodels.TaskEndDetail{
		Status:               utility.FromStringPtr(ad.Status),
		Type:                 utility.FromStringPtr(ad.Type),
		Description:          utility.FromStringPtr(ad.Description),
		FailingCommand:       utility.FromStringPtr(ad.FailingCommand),
		PostErrored:          ad.PostErrored,
		FailureMetadataTags:  ad.FailureMetadataTags,
		OtherFailingCommands: failingCmds,
		TimedOut:             ad.TimedOut,
		TimeoutType:          utility.FromStringPtr(ad.TimeoutType),
		OOMTracker:           ad.OOMTracker.ToService(),
		TraceID:              utility.FromStringPtr(ad.TraceID),
		DiskDevices:          ad.DiskDevices,
	}
}

// APIFailingCommand represents information about a command that failed in a
// task.
type APIFailingCommand struct {
	// FullDisplayName is the full display name of the failing command.
	FullDisplayName *string `json:"full_display_name,omitempty"`
	// FailureMetadataTags are tags associated with the failing command.
	FailureMetadataTags []string `json:"failure_metadata_tags,omitempty"`
}

func (afc *APIFailingCommand) BuildFromService(fc apimodels.FailingCommand) {
	afc.FullDisplayName = utility.ToStringPtr(fc.FullDisplayName)
	afc.FailureMetadataTags = fc.FailureMetadataTags
}

func (afc *APIFailingCommand) ToService() apimodels.FailingCommand {
	return apimodels.FailingCommand{
		FullDisplayName:     utility.FromStringPtr(afc.FullDisplayName),
		FailureMetadataTags: afc.FailureMetadataTags,
	}
}

type APIOomTrackerInfo struct {
	Detected bool  `json:"detected"`
	Pids     []int `json:"pids"`
}

func (at *APIOomTrackerInfo) BuildFromService(t *apimodels.OOMTrackerInfo) {
	if t != nil {
		at.Detected = t.Detected
		at.Pids = t.Pids
	}
}

func (ad *APIOomTrackerInfo) ToService() *apimodels.OOMTrackerInfo {
	return &apimodels.OOMTrackerInfo{
		Detected: ad.Detected,
		Pids:     ad.Pids,
	}
}

// BuildPreviousExecutions adds the given previous executions to the given API task.
func (at *APITask) BuildPreviousExecutions(ctx context.Context, tasks []task.Task, logURL, parsleyURL string) error {
	at.PreviousExecutions = make([]APITask, len(tasks))
	for i := range at.PreviousExecutions {
		if err := at.PreviousExecutions[i].BuildFromService(ctx, &tasks[i], &APITaskArgs{
			IncludeProjectIdentifier: true,
			IncludeAMI:               true,
			IncludeArtifacts:         true,
			LogURL:                   logURL,
			ParsleyLogURL:            parsleyURL,
		}); err != nil {
			return errors.Wrapf(err, "converting previous task execution at index %d to API model", i)
		}
	}

	return nil
}

// buildTask converts from a service level task by loading the data
// into the appropriate fields of the APITask.
func (at *APITask) buildTask(t *task.Task) error {
	id := t.Id
	// Old tasks are stored in a separate collection with ID set to
	// "old_task_ID" + "_" + "execution_number". This ID is not exposed to the user,
	// however. Instead in the UI executions are represented with a "/" and could be
	// represented in other ways elsewhere. The correct way to represent an old task is
	// with the same ID as the last execution, since semantically the tasks differ in
	// their execution number, not in their ID.
	if t.OldTaskId != "" {
		id = t.OldTaskId
	}
	*at = APITask{
		Id:                      utility.ToStringPtr(id),
		ProjectId:               utility.ToStringPtr(t.Project),
		CreateTime:              ToTimePtr(t.CreateTime),
		DispatchTime:            ToTimePtr(t.DispatchTime),
		ScheduledTime:           ToTimePtr(t.ScheduledTime),
		StartTime:               ToTimePtr(t.StartTime),
		FinishTime:              ToTimePtr(t.FinishTime),
		IngestTime:              ToTimePtr(t.IngestTime),
		ActivatedTime:           ToTimePtr(t.ActivatedTime),
		Version:                 utility.ToStringPtr(t.Version),
		Revision:                utility.ToStringPtr(t.Revision),
		Priority:                t.Priority,
		Activated:               t.Activated,
		ActivatedBy:             utility.ToStringPtr(t.ActivatedBy),
		BuildId:                 utility.ToStringPtr(t.BuildId),
		DistroId:                utility.ToStringPtr(t.DistroId),
		BuildVariant:            utility.ToStringPtr(t.BuildVariant),
		BuildVariantDisplayName: utility.ToStringPtr(t.BuildVariantDisplayName),
		DisplayName:             utility.ToStringPtr(t.DisplayName),
		HostId:                  utility.ToStringPtr(t.HostId),
		Tags:                    utility.ToStringPtrSlice(t.Tags),
		Execution:               t.Execution,
		Order:                   t.RevisionOrderNumber,
		Status:                  utility.ToStringPtr(t.Status),
		DisplayStatus:           utility.ToStringPtr(t.GetDisplayStatus()),
		ExpectedDuration:        NewAPIDuration(t.ExpectedDuration),
		GenerateTask:            t.GenerateTask,
		GeneratedBy:             t.GeneratedBy,
		DisplayOnly:             t.DisplayOnly,
		Mainline:                t.Requester == evergreen.RepotrackerVersionRequester,
		TaskGroup:               t.TaskGroup,
		TaskGroupMaxHosts:       t.TaskGroupMaxHosts,
		Blocked:                 t.Blocked(),
		Requester:               utility.ToStringPtr(t.Requester),
		Aborted:                 t.Aborted,
		HasTestResults:          t.HasTestResults,
		ResultsFailed:           t.ResultsFailed,
		MustHaveResults:         t.MustHaveResults,
		ResetWhenFinished:       t.ResetWhenFinished,
		ParentTaskId:            utility.FromStringPtr(t.DisplayTaskId),
		AbortInfo: APIAbortInfo{
			NewVersion: t.AbortInfo.NewVersion,
			TaskID:     t.AbortInfo.TaskID,
			User:       t.AbortInfo.User,
			PRClosed:   t.AbortInfo.PRClosed,
		},
		HasAnnotations:       t.HasAnnotations,
		TestSelectionEnabled: t.TestSelectionEnabled,
	}

	if t.BaseTask.Id != "" {
		at.BaseTask = APIBaseTaskInfo{
			Id:     utility.ToStringPtr(t.BaseTask.Id),
			Status: utility.ToStringPtr(t.BaseTask.Status),
		}
	}

	if t.TimeTaken != 0 {
		at.TimeTaken = NewAPIDuration(t.TimeTaken)
	} else if t.Status == evergreen.TaskStarted {
		at.TimeTaken = NewAPIDuration(time.Since(t.StartTime))
	}

	if !t.TaskCost.IsZero() {
		taskCost := t.TaskCost
		at.TaskCost = &taskCost
	}

	// Populate expected cost fields if they exist (not zero)
	if !t.PredictedTaskCost.IsZero() {
		predictedCost := t.PredictedTaskCost
		at.PredictedTaskCost = &predictedCost
	}

	if t.ParentPatchID != "" {
		at.Version = utility.ToStringPtr(t.ParentPatchID)
		if t.ParentPatchNumber != 0 {
			at.Order = t.ParentPatchNumber
		}
	}

	if t.StepbackInfo != nil {
		at.StepbackInfo = &APIStepbackInfo{
			LastFailingStepbackTaskId: t.StepbackInfo.LastFailingStepbackTaskId,
			LastPassingStepbackTaskId: t.StepbackInfo.LastPassingStepbackTaskId,
			NextStepbackTaskId:        t.StepbackInfo.NextStepbackTaskId,
			PreviousStepbackTaskId:    t.StepbackInfo.PreviousStepbackTaskId,
		}
	}

	if err := at.Details.BuildFromService(t.Details); err != nil {
		return errors.Wrap(err, "converting task end details to API model")
	}

	if len(t.ExecutionTasks) > 0 {
		ets := []*string{}
		for _, t := range t.ExecutionTasks {
			ets = append(ets, utility.ToStringPtr(t))
		}
		at.ExecutionTasks = ets
	}

	if len(t.DependsOn) > 0 {
		dependsOn := make([]APIDependency, len(t.DependsOn))
		for i, dep := range t.DependsOn {
			apiDep := APIDependency{}
			apiDep.BuildFromService(dep)
			dependsOn[i] = apiDep
		}
		at.DependsOn = dependsOn
	}

	at.OverrideDependencies = t.OverrideDependencies
	at.Archived = t.Archived

	return nil
}

type APITaskArgs struct {
	IncludeProjectIdentifier bool
	IncludeAMI               bool
	IncludeArtifacts         bool
	LogURL                   string
	ParsleyLogURL            string
}

// BuildFromService converts from a service level task by loading the data
// into the appropriate fields of the APITask. It takes optional arguments to populate
// additional fields.
func (at *APITask) BuildFromService(ctx context.Context, t *task.Task, args *APITaskArgs) error {
	err := at.buildTask(t)
	if err != nil {
		return err
	}
	if args == nil {
		return nil
	}
	baseTaskID := t.Id
	if t.OldTaskId != "" {
		baseTaskID = t.OldTaskId
	}
	if args.LogURL != "" {
		ll := LogLinks{
			AllLogLink:    utility.ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, args.LogURL, baseTaskID, t.Execution, "ALL")),
			TaskLogLink:   utility.ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, args.LogURL, baseTaskID, t.Execution, "T")),
			AgentLogLink:  utility.ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, args.LogURL, baseTaskID, t.Execution, "E")),
			SystemLogLink: utility.ToStringPtr(fmt.Sprintf(TaskLogLinkFormat, args.LogURL, baseTaskID, t.Execution, "S")),
		}
		at.Logs = ll
	}
	if args.ParsleyLogURL != "" {
		ll := LogLinks{
			AllLogLink:    utility.ToStringPtr(fmt.Sprintf(ParsleyTaskLogLinkFormat, args.ParsleyLogURL, baseTaskID, t.Execution, "all")),
			TaskLogLink:   utility.ToStringPtr(fmt.Sprintf(ParsleyTaskLogLinkFormat, args.ParsleyLogURL, baseTaskID, t.Execution, "task")),
			AgentLogLink:  utility.ToStringPtr(fmt.Sprintf(ParsleyTaskLogLinkFormat, args.ParsleyLogURL, baseTaskID, t.Execution, "agent")),
			SystemLogLink: utility.ToStringPtr(fmt.Sprintf(ParsleyTaskLogLinkFormat, args.ParsleyLogURL, baseTaskID, t.Execution, "system")),
		}
		at.ParsleyLogs = ll
	}
	if args.IncludeAMI {
		if err := at.GetAMI(ctx); err != nil {
			return errors.Wrap(err, "getting AMI")
		}
	}
	if args.IncludeArtifacts {
		if err := at.getArtifacts(ctx); err != nil {
			return errors.Wrap(err, "getting artifacts")
		}
	}
	if args.IncludeProjectIdentifier {
		at.GetProjectIdentifier(ctx)
	}

	return nil
}

func (at *APITask) GetAMI(ctx context.Context) error {
	if at.AMI != nil {
		return nil
	}
	if utility.FromStringPtr(at.HostId) != "" {
		h, err := host.FindOneId(ctx, utility.FromStringPtr(at.HostId))
		if err != nil {
			return errors.Wrapf(err, "finding host '%s' for task", utility.FromStringPtr(at.HostId))
		}
		if h != nil {
			ami := h.GetAMI()
			if ami != "" {
				at.AMI = utility.ToStringPtr(ami)
			}
		}
	}
	return nil
}

func (at *APITask) GetProjectIdentifier(ctx context.Context) {
	if at.ProjectIdentifier != nil {
		return
	}
	if utility.FromStringPtr(at.ProjectId) != "" {
		identifier, err := model.GetIdentifierForProject(ctx, utility.FromStringPtr(at.ProjectId))
		if err == nil {
			at.ProjectIdentifier = utility.ToStringPtr(identifier)
		}
	}
}

// ToService returns a service layer task using the data from the APITask.
// Wraps ToServiceTask to maintain the model interface.
func (at *APITask) ToService() (*task.Task, error) {
	st := &task.Task{
		Id:                      utility.FromStringPtr(at.Id),
		Project:                 utility.FromStringPtr(at.ProjectId),
		Version:                 utility.FromStringPtr(at.Version),
		Revision:                utility.FromStringPtr(at.Revision),
		Priority:                at.Priority,
		Activated:               at.Activated,
		ActivatedBy:             utility.FromStringPtr(at.ActivatedBy),
		BuildId:                 utility.FromStringPtr(at.BuildId),
		DistroId:                utility.FromStringPtr(at.DistroId),
		BuildVariant:            utility.FromStringPtr(at.BuildVariant),
		BuildVariantDisplayName: utility.FromStringPtr(at.BuildVariantDisplayName),
		DisplayName:             utility.FromStringPtr(at.DisplayName),
		HostId:                  utility.FromStringPtr(at.HostId),
		Execution:               at.Execution,
		RevisionOrderNumber:     at.Order,
		Status:                  utility.FromStringPtr(at.Status),
		DisplayStatus:           utility.FromStringPtr(at.DisplayStatus),
		TimeTaken:               at.TimeTaken.ToDuration(),
		ExpectedDuration:        at.ExpectedDuration.ToDuration(),
		GenerateTask:            at.GenerateTask,
		GeneratedBy:             at.GeneratedBy,
		DisplayOnly:             at.DisplayOnly,
		Requester:               utility.FromStringPtr(at.Requester),
		HasTestResults:          at.HasTestResults,
		ResultsFailed:           at.ResultsFailed,
		MustHaveResults:         at.MustHaveResults,
		BaseTask: task.BaseTaskInfo{
			Id:     utility.FromStringPtr(at.BaseTask.Id),
			Status: utility.FromStringPtr(at.BaseTask.Status),
		},
		DisplayTaskId:        utility.ToStringPtr(at.ParentTaskId),
		Aborted:              at.Aborted,
		Details:              at.Details.ToService(),
		Archived:             at.Archived,
		OverrideDependencies: at.OverrideDependencies,
		HasAnnotations:       at.HasAnnotations,
		TestSelectionEnabled: at.TestSelectionEnabled,
	}

	if at.TaskCost != nil {
		st.TaskCost = *at.TaskCost
	}

	catcher := grip.NewBasicCatcher()
	var err error
	st.CreateTime, err = FromTimePtr(at.CreateTime)
	catcher.Add(err)
	st.DispatchTime, err = FromTimePtr(at.DispatchTime)
	catcher.Add(err)
	st.ScheduledTime, err = FromTimePtr(at.ScheduledTime)
	catcher.Add(err)
	st.StartTime, err = FromTimePtr(at.StartTime)
	catcher.Add(err)
	st.FinishTime, err = FromTimePtr(at.FinishTime)
	catcher.Add(err)
	st.IngestTime, err = FromTimePtr(at.IngestTime)
	catcher.Add(err)
	st.ActivatedTime, err = FromTimePtr(at.ActivatedTime)
	catcher.Add(err)
	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	if at.StepbackInfo != nil {
		st.StepbackInfo = &task.StepbackInfo{
			LastFailingStepbackTaskId: at.StepbackInfo.LastFailingStepbackTaskId,
			LastPassingStepbackTaskId: at.StepbackInfo.LastPassingStepbackTaskId,
			NextStepbackTaskId:        at.StepbackInfo.NextStepbackTaskId,
			PreviousStepbackTaskId:    at.StepbackInfo.PreviousStepbackTaskId,
		}
	}

	if len(at.ExecutionTasks) > 0 {
		ets := []string{}
		for _, t := range at.ExecutionTasks {
			ets = append(ets, utility.FromStringPtr(t))
		}
		st.ExecutionTasks = ets
	}

	dependsOn := make([]task.Dependency, len(at.DependsOn))
	for i, dep := range at.DependsOn {
		dependsOn[i].TaskId = dep.TaskId
		dependsOn[i].Status = dep.Status
	}
	st.DependsOn = dependsOn
	return st, nil
}

func (at *APITask) getArtifacts(ctx context.Context) error {
	var err error
	var entries []artifact.Entry
	if at.DisplayOnly {
		ets := []artifact.TaskIDAndExecution{}
		for _, t := range at.ExecutionTasks {
			ets = append(ets, artifact.TaskIDAndExecution{TaskID: *t, Execution: at.Execution})
		}
		if len(ets) > 0 {
			entries, err = artifact.FindAll(ctx, artifact.ByTaskIdsAndExecutions(ets))
		}
	} else {
		entries, err = artifact.FindAll(ctx, artifact.ByTaskIdAndExecution(utility.FromStringPtr(at.Id), at.Execution))
	}
	if err != nil {
		return errors.Wrap(err, "retrieving artifacts")
	}
	env := evergreen.GetEnvironment()
	for _, entry := range entries {
		var strippedFiles []artifact.File
		// The route requires a user, so hasUser is always true.
		strippedFiles, err = artifact.StripHiddenFiles(ctx, entry.Files, true)
		if err != nil {
			return err
		}
		for _, file := range strippedFiles {
			apiFile := APIFile{}
			apiFile.BuildFromService(file)
			apiFile.GetLogURL(env, utility.FromStringPtr(at.Id), at.Execution)
			at.Artifacts = append(at.Artifacts, apiFile)
		}
	}

	return nil
}

type APIDependency struct {
	TaskId string `bson:"_id" json:"id"`
	Status string `bson:"status" json:"status"`
}

func (ad *APIDependency) BuildFromService(dep task.Dependency) {
	ad.TaskId = dep.TaskId
	ad.Status = dep.Status
}

// APIGeneratedTaskInfo contains basic information about a generated task.
type APIGeneratedTaskInfo struct {
	// The unique identifier of the task
	TaskID string `json:"task_id"`
	// The display name of the task
	TaskName string `json:"task_name"`
	// The unique identifier of the build
	BuildID string `json:"build_id"`
	// The name of the build variant
	BuildVariant string `json:"build_variant"`
	// The display name of the build variant
	BuildVariantDisplayName string `json:"build_variant_display_name"`
}

func (i *APIGeneratedTaskInfo) BuildFromService(dbInfo task.GeneratedTaskInfo) {
	i.TaskID = dbInfo.TaskID
	i.TaskName = dbInfo.TaskName
	i.BuildID = dbInfo.BuildID
	i.BuildVariant = dbInfo.BuildVariant
	i.BuildVariantDisplayName = dbInfo.BuildVariantDisplayName
}

// APINumTasksToFinalize contains information on the number of tasks a generator is set to finalize.
type APINumTasksToFinalize struct {
	NumTasksToFinalize *int `json:"num_tasks_to_finalize"`
}
