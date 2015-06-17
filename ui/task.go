package ui

import (
	"encoding/json"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
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
	// Initial number of revisions to return on first page load
	InitRevisionsBefore = 9
	InitRevisionsAfter  = 20

	// Number of revisions to return on subsequent requests
	NoRevisions     = 0
	MaxNumRevisions = 30
)

var NumTestsToSearchForTestNames = 100

type uiTaskData struct {
	Id               string                   `json:"id"`
	DisplayName      string                   `json:"display_name"`
	Revision         string                   `json:"gitspec"`
	BuildVariant     string                   `json:"build_variant"`
	Distro           string                   `json:"distro"`
	BuildId          string                   `json:"build_id"`
	Status           string                   `json:"status"`
	Activated        bool                     `json:"activated"`
	Restarts         int                      `json:"restarts"`
	Execution        int                      `json:"execution"`
	StartTime        int64                    `json:"start_time"`
	DispatchTime     int64                    `json:"dispatch_time"`
	FinishTime       int64                    `json:"finish_time"`
	Requester        string                   `json:"r"`
	ExpectedDuration time.Duration            `json:"expected_duration"`
	Priority         int                      `json:"priority"`
	PushTime         time.Time                `json:"push_time"`
	TimeTaken        time.Duration            `json:"time_taken"`
	TaskEndDetails   apimodels.TaskEndDetails `json:"task_end_details"`
	TestResults      []model.TestResult       `json:"test_results"`
	Aborted          bool                     `json:"abort"`
	MinQueuePos      int                      `json:"min_queue_pos"`

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
		taskFromDb, err := model.FindOneOldTask(bson.M{"_id": oldTaskId}, db.NoProjection, db.NoSort)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		archived = true
		if taskFromDb == nil {
			// for backwards compatibility with tasks without an execution
			if execution == 0 {
				taskFromDb, err = model.FindOneTask(bson.M{
					"$and": []bson.M{
						bson.M{"_id": projCtx.Task.Id},
						bson.M{"$or": []bson.M{bson.M{"execution": 0}, bson.M{"execution": nil}}}}},
					db.NoProjection,
					db.NoSort)
			} else {
				taskFromDb, err = model.FindOneTask(bson.M{"_id": projCtx.Task.Id, "execution": execution},
					db.NoProjection, db.NoSort)
			}
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error finding old task: %v", err))
				return
			}
		}
		projCtx.Task = taskFromDb
	}

	// Build a struct containing the subset of task data needed for display in the UI

	task := uiTaskData{
		Id:                  projCtx.Task.Id,
		DisplayName:         projCtx.Task.DisplayName,
		Revision:            projCtx.Task.Revision,
		Status:              projCtx.Task.Status,
		TaskEndDetails:      projCtx.Task.StatusDetails,
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
		Project:             projCtx.Version.Project,
		Author:              projCtx.Version.Author,
		AuthorEmail:         projCtx.Version.AuthorEmail,
		VersionId:           projCtx.Version.Id,
		RepoOwner:           projCtx.ProjectRef.Owner,
		Repo:                projCtx.ProjectRef.Repo,
		Archived:            archived,
	}

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
	FailedTests map[string][]model.TestResult
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

func (uis *UIServer) taskDependencies(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	dependencies, err := model.FindAllTasks(
		bson.M{
			"_id": bson.M{"$in": projCtx.Task.DependsOn},
		},
		bson.M{
			"display_name":   1,
			"status":         1,
			"activated":      1,
			"status_details": 1,
		}, []string{}, 0, 0)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	uis.WriteJSON(w, http.StatusOK, dependencies)
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

	templateToUse := "task_log.html"
	if (r.FormValue("text") == "true") || (r.Header.Get("Content-Type") == "text/plain") {
		templateToUse = "task_log_raw.html"
	}

	channel, err := model.GetRawTaskLogChannel(projCtx.Task.Id, execution, []string{}, logTypeFilter)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error getting log data: %s", err))
		return
	}
	err = uis.StreamHTML(w, http.StatusOK,
		struct {
			Data chan model.LogMessage
			User *user.DBUser
		}{channel, GetUser(r)}, "base", templateToUse)
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
		if err := projCtx.Task.TryReset(authName, evergreen.UIPackage, projCtx.Project, nil); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Task is now marked to restart."))
		uis.WriteJSON(w, http.StatusOK, "Task successfully restarted")
		return
	case "abort":
		if err := projCtx.Task.Abort(authName, true); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Task is now aborting."))
		uis.WriteJSON(w, http.StatusOK, "Task successfully restarted")
		return
	case "set_active":
		active := putParams.Active
		if err := model.SetTaskActivated(projCtx.Task.Id, authName, active); err != nil {
			http.Error(w, fmt.Sprintf("Error activating task %v: %v", projCtx.Task.Id, err),
				http.StatusInternalServerError)
			return
		}

		if active {
			PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Task is now scheduled."))
		} else {
			PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Task is now unscheduled."))
		}

		uis.WriteJSON(w, http.StatusOK, "Task successfully activated")
		return
	case "set_priority":
		priority, err := strconv.Atoi(putParams.Priority)
		if err != nil {
			http.Error(w, "Bad priority value, must be int", http.StatusBadRequest)
			return
		}
		if err = projCtx.Task.SetPriority(priority); err != nil {
			http.Error(w, fmt.Sprintf("Error setting task priority %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Priority for task set to %v.", priority)))
		uis.WriteJSON(w, http.StatusOK, "Successfully set priority")
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
