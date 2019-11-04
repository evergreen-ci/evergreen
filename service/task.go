package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/gimlet/rolemanager"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var NumTestsToSearchForTestNames = 100

type uiTaskData struct {
	Id                   string                  `json:"id"`
	DisplayName          string                  `json:"display_name"`
	Revision             string                  `json:"gitspec"`
	BuildVariant         string                  `json:"build_variant"`
	Distro               string                  `json:"distro"`
	BuildId              string                  `json:"build_id"`
	Status               string                  `json:"status"`
	TaskWaiting          string                  `json:"task_waiting"`
	Activated            bool                    `json:"activated"`
	Restarts             int                     `json:"restarts"`
	Execution            int                     `json:"execution"`
	TotalExecutions      int                     `json:"total_executions"`
	StartTime            int64                   `json:"start_time"`
	DispatchTime         int64                   `json:"dispatch_time"`
	FinishTime           int64                   `json:"finish_time"`
	Requester            string                  `json:"r"`
	ExpectedDuration     time.Duration           `json:"expected_duration"`
	Priority             int64                   `json:"priority"`
	TimeTaken            time.Duration           `json:"time_taken"`
	TaskEndDetails       apimodels.TaskEndDetail `json:"task_end_details"`
	TestResults          []uiTestResult          `json:"test_results"`
	Aborted              bool                    `json:"abort"`
	MinQueuePos          int                     `json:"min_queue_pos"`
	DependsOn            []uiDep                 `json:"depends_on"`
	OverrideDependencies bool                    `json:"override_dependencies"`
	IngestTime           time.Time               `json:"ingest_time"`
	EstWaitTime          time.Duration           `json:"wait_time"`
	UpstreamData         *uiUpstreamData         `json:"upstream_data,omitempty"`
	Logs                 *apimodels.TaskLogs     `json:"logs,omitempty"`

	// from the host doc (the dns name)
	HostDNS string `json:"host_dns,omitempty"`
	// from the host doc (the host id)
	HostId string `json:"host_id,omitempty"`

	// for breadcrumb
	BuildVariantDisplay string `json:"build_variant_display"`

	// from version
	VersionId   string    `json:"version_id"`
	Message     string    `json:"message"`
	Project     string    `json:"branch"`
	Author      string    `json:"author"`
	AuthorEmail string    `json:"author_email"`
	CreateTime  time.Time `json:"create_time"`

	// from project
	RepoOwner string `json:"repo_owner"`
	Repo      string `json:"repo_name"`

	// to avoid time skew b/t browser and API server
	CurrentTime int64 `json:"current_time"`

	// flag to indicate whether this is the current execution of this task, or
	// a previous execution
	Archived bool `json:"archived"`

	PatchInfo *uiPatch `json:"patch_info"`

	// display task info
	DisplayOnly    bool         `json:"display_only"`
	ExecutionTasks []uiExecTask `json:"execution_tasks"`
	PartOfDisplay  bool         `json:"in_display"`
	DisplayTaskID  string       `json:"display_task,omitempty"`
}

type uiDep struct {
	Id             string                  `json:"id"`
	Name           string                  `json:"display_name"`
	Status         string                  `json:"status"`
	RequiredStatus string                  `json:"required"`
	Activated      bool                    `json:"activated"`
	BuildVariant   string                  `json:"build_variant"`
	Details        apimodels.TaskEndDetail `json:"task_end_details"`
	Recursive      bool                    `json:"recursive"`
	TaskWaiting    string                  `json:"task_waiting"`
}

type uiExecTask struct {
	Id        string        `json:"id"`
	Name      string        `json:"display_name"`
	Status    string        `json:"status"`
	TimeTaken time.Duration `json:"time_taken"`
}

type uiTestResult struct {
	TestResult task.TestResult `json:"test_result"`
	TaskId     string          `json:"task_id"`
	TaskName   string          `json:"task_name"`
}

func (uis *UIServer) taskPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if projCtx.Build == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("build not found"))
		return
	}

	if projCtx.Version == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("version not found"))
		return
	}

	if projCtx.ProjectRef == nil {
		grip.Error("Project ref is nil")
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("version not found"))
		return
	}

	executionStr := gimlet.GetVars(r)["execution"]
	archived := false

	// if there is an execution number, the task might be in the old_tasks collection, so we
	// query that collection and set projCtx.Task to the old task if it exists.
	var execution int
	var err error
	if executionStr != "" {
		execution, err = strconv.Atoi(executionStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Bad execution number: %v", executionStr), http.StatusBadRequest)
			return
		}
		// Construct the old task id.
		oldTaskId := fmt.Sprintf("%v_%v", projCtx.Task.Id, executionStr)

		// Try to find the task in the old_tasks collection.
		var taskFromDb *task.Task
		taskFromDb, err = task.FindOneOld(task.ById(oldTaskId))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		// If we found a task, set the task context. Otherwise, if taskFromDb is nil, check
		// that the execution matches the context's execution. If it does not, return an
		// error, since that means we are searching for a task that does not exist.
		if taskFromDb != nil {
			projCtx.Task = taskFromDb
			archived = true
		} else if execution != projCtx.Task.Execution {
			uis.LoggedError(w, r, http.StatusNotFound, errors.New("Error finding task or execution"))
			return
		}
	} else {
		execution = projCtx.Task.Execution
	}

	// Build a struct containing the subset of task data needed for display in the UI
	tId := projCtx.Task.Id
	totalExecutions := projCtx.Task.Execution

	if archived {
		tId = projCtx.Task.OldTaskId

		// Get total number of executions for executions drop down
		var mostRecentExecution *task.Task
		mostRecentExecution, err = task.FindOne(task.ById(tId))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error finding most recent execution by id %s", tId))
			return
		}
		totalExecutions = mostRecentExecution.Execution
	}
	if totalExecutions < 1 {
		totalExecutions = 1
	}

	uiTask := uiTaskData{
		Id:                   tId,
		DisplayName:          projCtx.Task.DisplayName,
		Revision:             projCtx.Task.Revision,
		Status:               projCtx.Task.Status,
		TaskEndDetails:       projCtx.Task.Details,
		Distro:               projCtx.Task.DistroId,
		BuildVariant:         projCtx.Task.BuildVariant,
		BuildId:              projCtx.Task.BuildId,
		Activated:            projCtx.Task.Activated,
		Restarts:             projCtx.Task.Restarts,
		Execution:            projCtx.Task.Execution,
		Requester:            projCtx.Task.Requester,
		CreateTime:           projCtx.Task.CreateTime,
		IngestTime:           projCtx.Task.IngestTime,
		StartTime:            projCtx.Task.StartTime.UnixNano(),
		DispatchTime:         projCtx.Task.DispatchTime.UnixNano(),
		FinishTime:           projCtx.Task.FinishTime.UnixNano(),
		ExpectedDuration:     projCtx.Task.ExpectedDuration,
		TimeTaken:            projCtx.Task.TimeTaken,
		Priority:             projCtx.Task.Priority,
		Aborted:              projCtx.Task.Aborted,
		DisplayOnly:          projCtx.Task.DisplayOnly,
		OverrideDependencies: projCtx.Task.OverrideDependencies,
		CurrentTime:          time.Now().UnixNano(),
		BuildVariantDisplay:  projCtx.Build.DisplayName,
		Message:              projCtx.Version.Message,
		Project:              projCtx.Version.Identifier,
		Author:               projCtx.Version.Author,
		AuthorEmail:          projCtx.Version.AuthorEmail,
		VersionId:            projCtx.Version.Id,
		RepoOwner:            projCtx.ProjectRef.Owner,
		Repo:                 projCtx.ProjectRef.Repo,
		Logs:                 projCtx.Task.Logs,
		Archived:             archived,
		TotalExecutions:      totalExecutions,
		PartOfDisplay:        projCtx.Task.IsPartOfDisplay(),
	}

	deps, taskWaiting, err := getTaskDependencies(projCtx.Task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	uiTask.DependsOn = deps
	uiTask.TaskWaiting = taskWaiting
	uiTask.MinQueuePos, err = model.FindMinimumQueuePositionForTask(uiTask.Id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if uiTask.MinQueuePos < 0 {
		uiTask.MinQueuePos = 0
	}
	if uiTask.Status == evergreen.TaskUndispatched {
		uiTask.EstWaitTime, err = model.GetEstimatedStartTime(*projCtx.Task)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	var taskHost *host.Host
	if projCtx.Task.HostId != "" {
		uiTask.HostDNS = projCtx.Task.HostId
		uiTask.HostId = projCtx.Task.HostId
		taskHost, err = host.FindOne(host.ById(projCtx.Task.HostId))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if taskHost != nil {
			uiTask.HostDNS = taskHost.Host
		}
	}

	if projCtx.Patch != nil {
		var taskOnBaseCommit *task.Task
		taskOnBaseCommit, err = projCtx.Task.FindTaskOnBaseCommit()
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		taskPatch := &uiPatch{Patch: *projCtx.Patch}
		if taskOnBaseCommit != nil {
			taskPatch.BaseTaskId = taskOnBaseCommit.Id
			taskPatch.BaseTimeTaken = taskOnBaseCommit.TimeTaken
		}
		taskPatch.StatusDiffs = model.StatusDiffTasks(taskOnBaseCommit, projCtx.Task).Tests
		uiTask.PatchInfo = taskPatch
	}

	if projCtx.Task.TriggerID != "" {
		var projectName string
		projectName, err = model.GetUpstreamProjectName(projCtx.Task.TriggerID, projCtx.Task.TriggerType)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		uiTask.UpstreamData = &uiUpstreamData{
			ProjectName: projectName,
			TriggerID:   projCtx.Task.TriggerID,
			TriggerType: projCtx.Task.TriggerType,
		}
	}

	if uiTask.DisplayOnly {
		uiTask.TestResults = []uiTestResult{}
		execTasks := []task.Task{}
		for _, t := range projCtx.Task.ExecutionTasks {
			var et *task.Task
			if archived {
				et, err = task.FindOneOldNoMergeByIdAndExecution(t, execution)
			} else {
				et, err = task.FindOneId(t)
			}
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
			if et == nil {
				grip.Error(message.Fields{
					"message": "execution task not found",
					"task":    t,
					"parent":  projCtx.Task.Id,
				})
				continue
			}
			execTasks = append(execTasks, *et)
		}
		execTasks, err = task.MergeTestResultsBulk(execTasks, nil)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		for _, execTask := range execTasks {
			uiTask.ExecutionTasks = append(uiTask.ExecutionTasks, uiExecTask{
				Id:        execTask.Id,
				Name:      execTask.DisplayName,
				TimeTaken: execTask.TimeTaken,
				Status:    execTask.ResultStatus(),
			})
			for _, tr := range execTask.LocalTestResults {
				uiTask.TestResults = append(uiTask.TestResults, uiTestResult{TestResult: tr, TaskId: execTask.Id, TaskName: execTask.DisplayName})
			}
		}
	} else {
		for _, tr := range projCtx.Context.Task.LocalTestResults {
			uiTask.TestResults = append(uiTask.TestResults, uiTestResult{TestResult: tr})
		}
		if uiTask.PartOfDisplay {
			uiTask.DisplayTaskID = projCtx.Task.DisplayTask.Id
		}
	}

	ctx := r.Context()
	usr := gimlet.GetUser(ctx)
	pluginContext := projCtx.ToPluginContext(uis.Settings, usr)
	pluginContent := getPluginDataAndHTML(uis, plugin.TaskPage, pluginContext)
	isAdmin := false
	if usr != nil {
		isAdmin = projCtx.ProjectRef.IsAdmin(usr.Username(), uis.Settings)
	}
	opts := gimlet.PermissionOpts{Resource: projCtx.ProjectRef.Identifier, ResourceType: evergreen.ProjectResourceType}
	permissions, err := rolemanager.HighestPermissionsForRoles(usr.Roles(), evergreen.GetEnvironment().RoleManager(), opts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Task           uiTaskData
		Host           *host.Host
		PluginContent  pluginData
		JiraHost       string
		IsProjectAdmin bool
		Permissions    gimlet.Permissions
		ViewData
	}{uiTask, taskHost, pluginContent, uis.Settings.Jira.Host, isAdmin, permissions, uis.GetCommonViewData(w, r, false, true)}, "base", "task.html", "base_angular.html", "menu.html")
}

type taskHistoryPageData struct {
	TaskName    string
	Tasks       []bson.M
	Variants    []string
	FailedTests map[string][]task.TestResult
	Versions    []model.Version

	// Flags that indicate whether the beginning/end of history has been reached
	ExhaustedBefore bool
	ExhaustedAfter  bool

	// The revision for which the surrounding history was requested
	SelectedRevision string
}

// the task's most recent log messages
const DefaultLogMessages = 100 // passed as a limit, so 0 means don't limit

const AllLogsType = "ALL"

func getTaskLogs(taskId string, execution int, limit int, logType string,
	loggedIn bool) ([]apimodels.LogMessage, error) {

	logTypeFilter := []string{}
	if logType != AllLogsType {
		logTypeFilter = []string{logType}
	}

	// auth stuff
	if !loggedIn {
		if logType == AllLogsType {
			logTypeFilter = []string{apimodels.TaskLogPrefix}
		}
		if logType == apimodels.AgentLogPrefix || logType == apimodels.SystemLogPrefix {
			return []apimodels.LogMessage{}, nil
		}
	}

	return model.FindMostRecentLogMessages(taskId, execution, limit, []string{},
		logTypeFilter)
}

// getTaskDependencies returns the uiDeps for the task and its status (either its original status,
// "blocked", or "pending")
func getTaskDependencies(t *task.Task) ([]uiDep, string, error) {
	depIds := []string{}
	for _, dep := range t.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}
	dependencies, err := task.Find(task.ByIds(depIds).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	if err != nil {
		return nil, "", err
	}
	taskMap := map[string]*task.Task{}
	for i := range dependencies {
		taskMap[dependencies[i].Id] = &dependencies[i]
	}

	uiDependencies := []uiDep{}
	for _, dep := range t.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		uiDependencies = append(uiDependencies, uiDep{
			Id:             dep.TaskId,
			Name:           depTask.DisplayName,
			Status:         depTask.Status,
			RequiredStatus: dep.Status,
			Activated:      depTask.Activated,
			BuildVariant:   depTask.BuildVariant,
			Details:        depTask.Details,
			//TODO EVG-614: add "Recursive: dep.Recursive," once Task.DependsOn includes all recursive dependencies
		})
	}

	if err = t.CircularDependencies(); err != nil {
		return nil, "", err
	}
	state, err := t.BlockedState()
	if err != nil {
		return nil, "", errors.Wrap(err, "can't get blocked state")
	}

	return uiDependencies, state, nil
}

// async handler for polling the task log
type taskLogsWrapper struct {
	LogMessages []apimodels.LogMessage
}

func (uis *UIServer) taskLog(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	execution, err := strconv.Atoi(gimlet.GetVars(r)["execution"])
	if err != nil {
		http.Error(w, "Invalid execution number", http.StatusBadRequest)
		return
	}
	logType := r.FormValue("type")

	wrapper := &taskLogsWrapper{}
	if logType == "EV" {
		var loggedEvents []event.EventLogEntry
		loggedEvents, err = event.Find(event.AllLogCollection, event.MostRecentTaskEvents(projCtx.Task.Id, DefaultLogMessages))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		gimlet.WriteJSON(w, loggedEvents)
		return
	}

	ctx := r.Context()
	usr := gimlet.GetUser(ctx)
	taskLogs, err := getTaskLogs(projCtx.Task.Id, execution, DefaultLogMessages, logType, usr != nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wrapper.LogMessages = taskLogs
	gimlet.WriteJSON(w, wrapper)
}

func (uis *UIServer) taskLogRaw(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	execution, err := strconv.Atoi(gimlet.GetVars(r)["execution"])
	grip.Warning(err)
	logType := r.FormValue("type")

	if logType == "" {
		logType = AllLogsType
	}

	logTypeFilter := []string{}
	if logType != AllLogsType {
		logTypeFilter = []string{logType}
	}

	// restrict access if the user is not logged in
	ctx := r.Context()
	usr := gimlet.GetUser(ctx)
	if usr == nil {
		if logType == AllLogsType {
			logTypeFilter = []string{apimodels.TaskLogPrefix}
		}
		if logType == apimodels.AgentLogPrefix || logType == apimodels.SystemLogPrefix {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	channel, err := model.GetRawTaskLogChannel(projCtx.Task.Id, execution, []string{}, logTypeFilter)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error getting log data"))
		return
	}

	type logTemplateData struct {
		Data chan apimodels.LogMessage
		User gimlet.User
	}

	if (r.FormValue("text") == "true") || (r.Header.Get("Content-Type") == "text/plain") {
		uis.renderText.Stream(w, http.StatusOK, logTemplateData{channel, usr}, "base", "task_log_raw.html")
		return
	}
	uis.render.Stream(w, http.StatusOK, logTemplateData{channel, usr}, "base", "task_log.html")
}

// avoids type-checking json params for the below function
func (uis *UIServer) taskModify(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	body := util.NewRequestReader(r)
	defer body.Close()

	reqBody, err := ioutil.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	putParams := struct {
		Action   string `json:"action"`
		Priority string `json:"priority"`

		// for the set_active option
		Active bool `json:"active"`
	}{}

	err = json.Unmarshal(reqBody, &putParams)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	authUser := gimlet.GetUser(ctx)
	authName := authUser.DisplayName()
	requiredPermission := gimlet.PermissionOpts{
		Resource:      projCtx.ProjectRef.Identifier,
		ResourceType:  "project",
		Permission:    evergreen.PermissionTasks,
		RequiredLevel: int(evergreen.TasksAdmin),
	}
	taskAdmin, err := authUser.HasPermission(requiredPermission)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error checking permissions: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// determine what action needs to be taken
	switch putParams.Action {
	case "restart":
		if err = model.TryResetTask(projCtx.Task.Id, authName, evergreen.UIPackage, nil); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case "abort":
		if err = model.AbortTask(projCtx.Task.Id, authName); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}
		if projCtx.Task.Requester == evergreen.MergeTestRequester {
			_, err := commitqueue.RemoveCommitQueueItem(projCtx.ProjectRef.Identifier,
				projCtx.ProjectRef.CommitQueue.PatchType, projCtx.Task.Version, true)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))

		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case "set_active":
		active := putParams.Active
		if err = model.SetActiveState(projCtx.Task.Id, authUser.Username(), active); err != nil {
			http.Error(w, fmt.Sprintf("Error activating task %v: %v", projCtx.Task.Id, err),
				http.StatusInternalServerError)
			return
		}

		if !active && projCtx.Task.Requester == evergreen.MergeTestRequester {
			_, err := commitqueue.RemoveCommitQueueItem(projCtx.ProjectRef.Identifier,
				projCtx.ProjectRef.CommitQueue.PatchType, projCtx.Task.Version, true)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case "set_priority":
		var priority int64
		priority, err = strconv.ParseInt(putParams.Priority, 10, 64)
		if err != nil {
			http.Error(w, "Bad priority value, must be int", http.StatusBadRequest)
			return
		}
		if priority > evergreen.MaxTaskPriority {
			if !uis.isSuperUser(authUser) && !taskAdmin { // TODO PM-1355 remove superuser check
				http.Error(w, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority),
					http.StatusUnauthorized)
				return
			}
		}
		if err = projCtx.Task.SetPriority(priority, authUser.Username()); err != nil {
			http.Error(w, fmt.Sprintf("Error setting task priority %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case "override_dependencies":
		if !projCtx.ProjectRef.IsAdmin(authUser.Username(), uis.Settings) && !taskAdmin { // TODO PM-1355 remove admin check
			http.Error(w, "not authorized to override dependencies", http.StatusUnauthorized)
			return
		}
		err = projCtx.Task.SetOverrideDependencies(authUser.Username())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	default:
		gimlet.WriteJSONError(w, "Unrecognized action: "+putParams.Action)
	}
}

func (uis *UIServer) testLog(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	logId := vars["log_id"]
	var (
		testLog  *model.TestLog
		err      error
		taskExec int
	)

	if logId != "" { // direct link to a log document by its ID
		testLog, err = model.FindOneTestLogById(logId)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	} else {
		taskID := vars["task_id"]
		testName := vars["test_name"]
		taskExecutionsAsString := vars["task_execution"]
		taskExec, err = strconv.Atoi(taskExecutionsAsString)
		if err != nil {
			http.Error(w, "task execution num must be an int", http.StatusBadRequest)
			return
		}

		testLog, err = model.FindOneTestLog(testName, taskID, taskExec)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	if testLog == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	displayLogs := make(chan apimodels.LogMessage)
	go func() {
		defer close(displayLogs)
		for _, line := range testLog.Lines {
			if ctx.Err() != nil {
				return
			}
			displayLogs <- apimodels.LogMessage{
				Type:     apimodels.TaskLogPrefix,
				Severity: apimodels.LogInfoPrefix,
				Version:  evergreen.LogmessageCurrentVersion,
				Message:  line,
			}
		}
	}()
	usr := gimlet.GetUser(ctx)
	template := "task_log.html"
	data := struct {
		Data chan apimodels.LogMessage
		User gimlet.User
	}{displayLogs, usr}

	if (r.FormValue("raw") == "1") || (r.Header.Get("Content-type") == "text/plain") {
		template = "task_log_raw.html"
		uis.renderText.Stream(w, http.StatusOK, data, "base", template)
	} else {
		uis.render.WriteResponse(w, http.StatusOK, data, "base", template)
	}
}
