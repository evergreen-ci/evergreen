package ui

import (
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const (
	// status overwrites
	TaskBlocked = "blocked"
	TaskPending = "pending"
)

var NumTestsToSearchForTestNames = 100

type uiTaskData struct {
	Id               string                  `json:"id"`
	DisplayName      string                  `json:"display_name"`
	Revision         string                  `json:"gitspec"`
	BuildVariant     string                  `json:"build_variant"`
	Distro           string                  `json:"distro"`
	BuildId          string                  `json:"build_id"`
	Status           string                  `json:"status"`
	TaskWaiting      string                  `json:"task_waiting"`
	Activated        bool                    `json:"activated"`
	Restarts         int                     `json:"restarts"`
	Execution        int                     `json:"execution"`
	StartTime        int64                   `json:"start_time"`
	DispatchTime     int64                   `json:"dispatch_time"`
	FinishTime       int64                   `json:"finish_time"`
	Requester        string                  `json:"r"`
	ExpectedDuration time.Duration           `json:"expected_duration"`
	Priority         int64                   `json:"priority"`
	PushTime         time.Time               `json:"push_time"`
	TimeTaken        time.Duration           `json:"time_taken"`
	TaskEndDetails   apimodels.TaskEndDetail `json:"task_end_details"`
	TestResults      []task.TestResult       `json:"test_results"`
	Aborted          bool                    `json:"abort"`
	MinQueuePos      int                     `json:"min_queue_pos"`
	DependsOn        []uiDep                 `json:"depends_on"`

	// from the host doc (the dns name)
	HostDNS string `json:"host_dns,omitempty"`
	// from the host doc (the host id)
	HostId string `json:"host_id,omitempty"`

	// for breadcrumb
	BuildVariantDisplay string `json:"build_variant_display"`

	// from version
	VersionId   string `json:"version_id"`
	Message     string `json:"message"`
	Project     string `json:"branch"`
	Author      string `json:"author"`
	AuthorEmail string `json:"author_email"`
	CreatedTime int64  `json:"created_time"`

	// from project
	RepoOwner string `json:"repo_owner"`
	Repo      string `json:"repo_name"`

	// to avoid time skew b/t browser and API server
	CurrentTime int64 `json:"current_time"`

	// flag to indicate whether this is the current execution of this task, or
	// a previous execution
	Archived bool `json:"archived"`

	PatchInfo *uiPatch `json:"patch_info"`
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

func (uis *UIServer) taskPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if projCtx.Build == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("build not found"))
		return
	}

	if projCtx.Version == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("version not found"))
		return
	}

	if projCtx.ProjectRef == nil {
		evergreen.Logger.Logf(slogger.ERROR, "Project ref is nil")
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("version not found"))
		return
	}

	executionStr := mux.Vars(r)["execution"]
	archived := false
	if executionStr != "" {
		// otherwise we can look in either tasks or old_tasks
		// where tasks are looked up in the old_tasks collection with key made up of
		// the original key and the execution number joined by an "_"
		// and the tasks are looked up in the tasks collection by key and execution
		// number, so that we avoid finding the wrong execution in the tasks
		// collection
		execution, err := strconv.Atoi(executionStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Bad execution number: %v", executionStr), http.StatusBadRequest)
			return
		}
		oldTaskId := fmt.Sprintf("%v_%v", projCtx.Task.Id, executionStr)
		taskFromDb, err := task.FindOneOld(task.ById(oldTaskId))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		archived = true

		if taskFromDb == nil {
			if execution != projCtx.Task.Execution {
				uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error finding old task: %v", err))
				return
			}

		} else {
			projCtx.Task = taskFromDb
		}
	}

	// Build a struct containing the subset of task data needed for display in the UI

	task := uiTaskData{
		Id:                  projCtx.Task.Id,
		DisplayName:         projCtx.Task.DisplayName,
		Revision:            projCtx.Task.Revision,
		Status:              projCtx.Task.Status,
		TaskEndDetails:      projCtx.Task.Details,
		Distro:              projCtx.Task.DistroId,
		BuildVariant:        projCtx.Task.BuildVariant,
		BuildId:             projCtx.Task.BuildId,
		Activated:           projCtx.Task.Activated,
		Restarts:            projCtx.Task.Restarts,
		Execution:           projCtx.Task.Execution,
		Requester:           projCtx.Task.Requester,
		StartTime:           projCtx.Task.StartTime.UnixNano(),
		DispatchTime:        projCtx.Task.DispatchTime.UnixNano(),
		FinishTime:          projCtx.Task.FinishTime.UnixNano(),
		ExpectedDuration:    projCtx.Task.ExpectedDuration,
		PushTime:            projCtx.Task.PushTime,
		TimeTaken:           projCtx.Task.TimeTaken,
		Priority:            projCtx.Task.Priority,
		TestResults:         projCtx.Task.TestResults,
		Aborted:             projCtx.Task.Aborted,
		CurrentTime:         time.Now().UnixNano(),
		BuildVariantDisplay: projCtx.Build.DisplayName,
		Message:             projCtx.Version.Message,
		Project:             projCtx.Version.Identifier,
		Author:              projCtx.Version.Author,
		AuthorEmail:         projCtx.Version.AuthorEmail,
		VersionId:           projCtx.Version.Id,
		RepoOwner:           projCtx.ProjectRef.Owner,
		Repo:                projCtx.ProjectRef.Repo,
		Archived:            archived,
	}

	deps, taskWaiting, err := getTaskDependencies(projCtx.Task)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	task.DependsOn = deps
	task.TaskWaiting = taskWaiting

	// Activating and deactivating tasks should clear out the
	// MinQueuePos but just in case, lets not show it if we shouldn't
	if projCtx.Task.Status == evergreen.TaskUndispatched && projCtx.Task.Activated {
		task.MinQueuePos = projCtx.Task.MinQueuePos
	}

	if projCtx.Task.HostId != "" {
		task.HostDNS = projCtx.Task.HostId
		task.HostId = projCtx.Task.HostId
		taskHost, err := host.FindOne(host.ById(projCtx.Task.HostId))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if taskHost != nil {
			task.HostDNS = taskHost.Host
		}
	}

	if projCtx.Patch != nil {
		taskOnBaseCommit, err := projCtx.Task.FindTaskOnBaseCommit()
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		taskPatch := &uiPatch{Patch: *projCtx.Patch}
		if taskOnBaseCommit != nil {
			taskPatch.BaseTaskId = taskOnBaseCommit.Id
		}
		taskPatch.StatusDiffs = model.StatusDiffTasks(taskOnBaseCommit, projCtx.Task).Tests
		task.PatchInfo = taskPatch
	}

	flashes := PopFlashes(uis.CookieStore, r, w)

	pluginContext := projCtx.ToPluginContext(uis.Settings, GetUser(r))
	pluginContent := getPluginDataAndHTML(uis, plugin.TaskPage, pluginContext)

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData   projectContext
		User          *user.DBUser
		Flashes       []interface{}
		Task          uiTaskData
		PluginContent pluginData
	}{projCtx, GetUser(r), flashes, task, pluginContent}, "base",
		"task.html", "base_angular.html", "menu.html")
}

type taskHistoryPageData struct {
	TaskName    string
	Tasks       []bson.M
	Variants    []string
	FailedTests map[string][]task.TestResult
	Versions    []version.Version

	// Flags that indicate whether the beginning/end of history has been reached
	ExhaustedBefore bool
	ExhaustedAfter  bool

	// The revision for which the surrounding history was requested
	SelectedRevision string
}

// the task's most recent log messages
const DefaultLogMessages = 20 // passed as a limit, so 0 means don't limit

const AllLogsType = "ALL"

func getTaskLogs(taskId string, execution int, limit int, logType string,
	loggedIn bool) ([]model.LogMessage, error) {

	logTypeFilter := []string{}
	if logType != AllLogsType {
		logTypeFilter = []string{logType}
	}

	// auth stuff
	if !loggedIn {
		if logType == AllLogsType {
			logTypeFilter = []string{model.TaskLogPrefix}
		}
		if logType == model.AgentLogPrefix || logType == model.SystemLogPrefix {
			return []model.LogMessage{}, nil
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

	idToUiDep := make(map[string]uiDep)
	// match each task with its dependency requirements
	for _, depTask := range dependencies {
		for _, dep := range t.DependsOn {
			if dep.TaskId == depTask.Id {
				idToUiDep[depTask.Id] = uiDep{
					Id:             depTask.Id,
					Name:           depTask.DisplayName,
					Status:         depTask.Status,
					RequiredStatus: dep.Status,
					Activated:      depTask.Activated,
					BuildVariant:   depTask.BuildVariant,
					Details:        depTask.Details,
					//TODO EVG-614: add "Recursive: dep.Recursive," once Task.DependsOn includes all recursive dependencies
				}
			}
		}
	}

	idToDep := make(map[string]task.Task)
	for _, dep := range dependencies {
		idToDep[dep.Id] = dep
	}

	// TODO EVG 614: delete this section once Task.DependsOn includes all recursive dependencies
	err = addRecDeps(idToDep, idToUiDep, make(map[string]bool))
	if err != nil {
		return nil, "", err
	}

	// set the status for each of the uiDeps as "blocked" or "pending" if appropriate
	// and get the status for task
	status := setBlockedOrPending(*t, idToDep, idToUiDep)

	uiDeps := make([]uiDep, 0, len(idToUiDep))
	for _, dep := range idToUiDep {
		uiDeps = append(uiDeps, dep)
	}
	return uiDeps, status, nil
}

// addRecDeps recursively finds all dependencies of tasks and adds them to tasks and uiDeps.
// done is a hashtable of task IDs whose dependencies we have found.
// TODO EVG-614: delete this function once Task.DependsOn includes all recursive dependencies.
func addRecDeps(tasks map[string]task.Task, uiDeps map[string]uiDep, done map[string]bool) error {
	curTask := make(map[string]bool)
	depIds := make([]string, 0)
	for _, t := range tasks {
		if _, ok := done[t.Id]; !ok {
			for _, dep := range t.DependsOn {
				depIds = append(depIds, dep.TaskId)
			}
			curTask[t.Id] = true
		}
	}

	if len(depIds) == 0 {
		return nil
	}

	deps, err := task.Find(task.ByIds(depIds).WithFields(task.DisplayNameKey, task.StatusKey, task.ActivatedKey,
		task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))

	if err != nil {
		return err
	}

	for _, dep := range deps {
		tasks[dep.Id] = dep
	}

	for _, t := range tasks {
		if _, ok := curTask[t.Id]; ok {
			for _, dep := range t.DependsOn {
				if uid, ok := uiDeps[dep.TaskId]; !ok ||
					// only replace if the current uiDep is not strict and not recursive
					(uid.RequiredStatus == model.AllStatuses && !uid.Recursive) {
					depTask := tasks[dep.TaskId]
					uiDeps[depTask.Id] = uiDep{
						Id:             depTask.Id,
						Name:           depTask.DisplayName,
						Status:         depTask.Status,
						RequiredStatus: dep.Status,
						Activated:      depTask.Activated,
						BuildVariant:   depTask.BuildVariant,
						Details:        depTask.Details,
						Recursive:      true,
					}
				}
			}
			done[t.Id] = true
		}
	}

	return addRecDeps(tasks, uiDeps, done)
}

// setBlockedOrPending sets the status of all uiDeps to "blocked" or "pending" if appropriate
// and returns "blocked", "pending", or the original status of task as appropriate.
// A task is blocked if some recursive dependency is in an undesirable state.
// A task is pending if some dependency has not finished.
func setBlockedOrPending(t task.Task, tasks map[string]task.Task, uiDeps map[string]uiDep) string {
	blocked := false
	pending := false
	for _, dep := range t.DependsOn {
		depTask := tasks[dep.TaskId]

		uid := uiDeps[depTask.Id]
		uid.TaskWaiting = setBlockedOrPending(depTask, tasks, uiDeps)
		uiDeps[depTask.Id] = uid
		if uid.TaskWaiting == TaskBlocked {
			blocked = true
		} else if depTask.Status == evergreen.TaskSucceeded || depTask.Status == evergreen.TaskFailed {
			if depTask.Status != dep.Status && dep.Status != model.AllStatuses {
				blocked = true
			}
		} else {
			pending = true
		}
	}
	if blocked {
		return TaskBlocked
	}
	if pending {
		return TaskPending
	}
	return ""
}

// async handler for polling the task log
type taskLogsWrapper struct {
	LogMessages []model.LogMessage
}

func (uis *UIServer) taskLog(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	execution, err := strconv.Atoi(mux.Vars(r)["execution"])
	if err != nil {
		http.Error(w, "Invalid execution number", http.StatusBadRequest)
		return
	}
	logType := r.FormValue("type")

	wrapper := &taskLogsWrapper{}
	if logType == "EV" {
		loggedEvents, err := event.Find(event.MostRecentTaskEvents(projCtx.Task.Id, DefaultLogMessages))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		uis.WriteJSON(w, http.StatusOK, loggedEvents)
		return
	} else {
		taskLogs, err := getTaskLogs(projCtx.Task.Id, execution, DefaultLogMessages, logType, GetUser(r) != nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		wrapper.LogMessages = taskLogs
		uis.WriteJSON(w, http.StatusOK, wrapper)
	}
}

func (uis *UIServer) taskLogRaw(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	execution, err := strconv.Atoi(mux.Vars(r)["execution"])
	logType := r.FormValue("type")

	if logType == "" {
		logType = AllLogsType
	}

	logTypeFilter := []string{}
	if logType != AllLogsType {
		logTypeFilter = []string{logType}
	}

	// restrict access if the user is not logged in
	if GetUser(r) == nil {
		if logType == AllLogsType {
			logTypeFilter = []string{model.TaskLogPrefix}
		}
		if logType == model.AgentLogPrefix || logType == model.SystemLogPrefix {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	channel, err := model.GetRawTaskLogChannel(projCtx.Task.Id, execution, []string{}, logTypeFilter)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error getting log data: %s", err))
		return
	}

	type logTemplateData struct {
		Data chan model.LogMessage
		User *user.DBUser
	}

	if (r.FormValue("text") == "true") || (r.Header.Get("Content-Type") == "text/plain") {
		err = uis.StreamText(w, http.StatusOK, logTemplateData{channel, GetUser(r)}, "base", "task_log_raw.html")
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, err.Error())
		}
		return
	}
	err = uis.StreamHTML(w, http.StatusOK, logTemplateData{channel, GetUser(r)}, "base", "task_log.html")
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, err.Error())
	}
}

// avoids type-checking json params for the below function
func (uis *UIServer) taskModify(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

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

	authUser := GetUser(r)
	authName := authUser.DisplayName()

	// determine what action needs to be taken
	switch putParams.Action {
	case "restart":
		if err := model.TryResetTask(projCtx.Task.Id, authName, evergreen.UIPackage, projCtx.Project, nil); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		uis.WriteJSON(w, http.StatusOK, projCtx.Task)
		return
	case "abort":
		if err := model.AbortTask(projCtx.Task.Id, authName, true); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}
		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))

		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		uis.WriteJSON(w, http.StatusOK, projCtx.Task)
		return
	case "set_active":
		active := putParams.Active
		if err := model.SetActiveState(projCtx.Task.Id, authName, active); err != nil {
			http.Error(w, fmt.Sprintf("Error activating task %v: %v", projCtx.Task.Id, err),
				http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		uis.WriteJSON(w, http.StatusOK, projCtx.Task)
		return
	case "set_priority":
		priority, err := strconv.ParseInt(putParams.Priority, 10, 64)
		if err != nil {
			http.Error(w, "Bad priority value, must be int", http.StatusBadRequest)
			return
		}
		if err = projCtx.Task.SetPriority(priority); err != nil {
			http.Error(w, fmt.Sprintf("Error setting task priority %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}
		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOne(task.ById(projCtx.Task.Id))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		uis.WriteJSON(w, http.StatusOK, projCtx.Task)
		return
	default:
		uis.WriteJSON(w, http.StatusBadRequest, "Unrecognized action: "+putParams.Action)
	}
}

func (uis *UIServer) testLog(w http.ResponseWriter, r *http.Request) {
	logId := mux.Vars(r)["log_id"]
	var testLog *model.TestLog
	var err error

	if logId != "" { // direct link to a log document by its ID
		testLog, err = model.FindOneTestLogById(logId)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	} else {
		taskID := mux.Vars(r)["task_id"]
		testName := mux.Vars(r)["test_name"]
		taskExecutionsAsString := mux.Vars(r)["task_execution"]
		taskExec, err := strconv.Atoi(taskExecutionsAsString)
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

	displayLogs := make(chan model.LogMessage)
	go func() {
		for _, line := range testLog.Lines {
			displayLogs <- model.LogMessage{
				Type:     model.TaskLogPrefix,
				Severity: model.LogInfoPrefix,
				Version:  evergreen.LogmessageCurrentVersion,
				Message:  line,
			}
		}
		close(displayLogs)
	}()

	uis.WriteHTML(w, http.StatusOK, struct {
		Data chan model.LogMessage
		User *user.DBUser
	}{displayLogs, GetUser(r)}, "base", "task_log.html")
}
