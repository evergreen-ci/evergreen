package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type logData struct {
	Data chan apimodels.LogMessage
	User gimlet.User
}

// the task's most recent log messages
const DefaultLogMessages = 100 // passed as a limit, so 0 means don't limit

func (uis *UIServer) taskLog(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	tsk := projCtx.Task
	if execStr := gimlet.GetVars(r)["execution"]; execStr != "" {
		execution, err := strconv.Atoi(execStr)
		if err != nil {
			http.Error(w, "invalid execution", http.StatusBadRequest)
			return
		}

		if tsk.Execution != execution {
			tsk, err = task.FindOneIdAndExecution(r.Context(), tsk.Id, execution)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
			if tsk == nil {
				http.Error(w, fmt.Sprintf("task '%s' with execution '%d' not found", projCtx.Task.Id, execution), http.StatusNotFound)
				return
			}
		}
	}

	logType := r.FormValue("type")
	if logType == "EV" {
		loggedEvents, err := event.Find(r.Context(), event.MostRecentTaskEvents(projCtx.Task.Id, DefaultLogMessages))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		gimlet.WriteJSON(w, loggedEvents)
		return
	}

	it, err := tsk.GetTaskLogs(r.Context(), task.TaskLogGetOptions{
		LogType: getTaskLogTypeMapping(logType),
		TailN:   DefaultLogMessages,
	})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	lines, err := apimodels.ReadLogToSlice(it)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, struct {
		LogMessages []*apimodels.LogMessage
	}{LogMessages: lines})
}

func (uis *UIServer) taskLogRaw(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	tsk := projCtx.Task
	if execStr := gimlet.GetVars(r)["execution"]; execStr != "" {
		execution, err := strconv.Atoi(execStr)
		if err != nil {
			http.Error(w, "invalid execution", http.StatusBadRequest)
			return
		}

		if tsk.Execution != execution {
			tsk, err = task.FindOneIdAndExecution(r.Context(), tsk.Id, execution)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
			if tsk == nil {
				http.Error(w, fmt.Sprintf("task '%s' with execution '%d' not found", projCtx.Task.Id, execution), http.StatusNotFound)
				return
			}
		}
	}

	it, err := tsk.GetTaskLogs(r.Context(), task.TaskLogGetOptions{LogType: getTaskLogTypeMapping(r.FormValue("type"))})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if r.FormValue("text") == "true" || r.Header.Get("Content-Type") == "text/plain" {
		gimlet.WriteText(w, log.NewLogIteratorReader(it, log.LogIteratorReaderOptions{
			PrintTime:     true,
			TimeZone:      getUserTimeZone(MustHaveUser(r)),
			PrintPriority: r.FormValue("priority") == "true",
		}))
	} else {
		data := logData{
			Data: apimodels.StreamFromLogIterator(it),
			User: gimlet.GetUser(r.Context()),
		}
		uis.render.Stream(w, http.StatusOK, data, "base", "task_log.html")
	}
}

// getUserTimeZone returns the time zone specified by the user settings.
// Defaults to `America/New_York`.
func getUserTimeZone(u *user.DBUser) *time.Location {
	tz := u.Settings.Timezone
	if tz == "" {
		tz = "America/New_York"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}

	return loc
}

func getTaskLogTypeMapping(prefix string) task.TaskLogType {
	switch prefix {
	case apimodels.AgentLogPrefix:
		return task.TaskLogTypeAgent
	case apimodels.SystemLogPrefix:
		return task.TaskLogTypeSystem
	case apimodels.TaskLogPrefix:
		return task.TaskLogTypeTask
	default:
		return task.TaskLogTypeAll
	}
}

func (uis *UIServer) taskFileRaw(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	fileName := gimlet.GetVars(r)["file_name"]
	if fileName == "" {
		http.Error(w, "file name not specified", http.StatusBadRequest)
		return
	}
	executionNum := projCtx.Task.Execution
	var err error
	if execStr := gimlet.GetVars(r)["execution"]; execStr != "" {
		executionNum, err = strconv.Atoi(execStr)
		if err != nil {
			http.Error(w, "invalid execution", http.StatusBadRequest)
			return
		}
	}

	taskFiles, err := artifact.GetAllArtifacts(r.Context(), []artifact.TaskIDAndExecution{{TaskID: projCtx.Task.Id, Execution: executionNum}})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "unable to find artifacts for task '%s'", projCtx.Task.Id))
		return
	}
	taskFiles, err = artifact.StripHiddenFiles(r.Context(), taskFiles, true)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "unable to strip hidden files for task '%s'", projCtx.Task.Id))
		return
	}
	var tFile *artifact.File
	for _, taskFile := range taskFiles {
		if taskFile.Name == fileName {
			tFile = &taskFile
			break
		}
	}
	if tFile == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.New(fmt.Sprintf("file '%s' not found", fileName)))
		return
	}

	hasContentType := false
	for _, contentType := range uis.Settings.Ui.FileStreamingContentTypes {
		if strings.HasPrefix(tFile.ContentType, contentType) {
			hasContentType = true
			break
		}
	}
	if !hasContentType {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New(fmt.Sprintf("unsupported file content type '%s'", tFile.ContentType)))
		return
	}

	response, err := http.Get(tFile.Link)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "downloading file"))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		uis.LoggedError(w, r, response.StatusCode, errors.Errorf("failed to download file with status code: %d", response.StatusCode))
		return
	}

	// Create a buffer to stream the file in chunks.
	const bufferSize = 1024 * 1024 // 1MB
	buffer := make([]byte, bufferSize)

	w.Header().Set("Content-Type", tFile.ContentType)
	_, err = io.CopyBuffer(w, response.Body, buffer)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "writing to response"))
		return
	}

}

// avoids type-checking json params for the below function
func (uis *UIServer) taskModify(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Task == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	body := utility.NewRequestReader(r)
	defer body.Close()

	reqBody, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	putParams := struct {
		Action   evergreen.ModificationAction `json:"action"`
		Priority string                       `json:"priority"`

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
		Resource:      projCtx.ProjectRef.Id,
		ResourceType:  "project",
		Permission:    evergreen.PermissionTasks,
		RequiredLevel: evergreen.TasksAdmin.Value,
	}
	taskAdmin := authUser.HasPermission(requiredPermission)

	// determine what action needs to be taken
	switch putParams.Action {
	case evergreen.RestartAction:
		if err = model.TryResetTask(ctx, uis.env.Settings(), projCtx.Task.Id, authName, evergreen.UIPackage, nil); err != nil {
			http.Error(w, fmt.Sprintf("Error restarting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOneId(r.Context(), projCtx.Task.Id)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case evergreen.AbortAction:
		if err = model.AbortTask(ctx, projCtx.Task.Id, authName); err != nil {
			http.Error(w, fmt.Sprintf("Error aborting task %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOneId(r.Context(), projCtx.Task.Id)

		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case evergreen.SetActiveAction:
		active := putParams.Active
		if active && projCtx.Task.Requester == evergreen.GithubMergeRequester {
			http.Error(w, "commit queue tasks cannot be manually scheduled", http.StatusBadRequest)
			return
		}
		if err = model.SetActiveState(r.Context(), authUser.Username(), active, *projCtx.Task); err != nil {
			http.Error(w, fmt.Sprintf("Error activating task %v: %v", projCtx.Task.Id, err),
				http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOneId(r.Context(), projCtx.Task.Id)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case evergreen.SetPriorityAction:
		var priority int64
		priority, err = strconv.ParseInt(putParams.Priority, 10, 64)
		if err != nil {
			http.Error(w, "Bad priority value, must be int", http.StatusBadRequest)
			return
		}
		if priority > evergreen.MaxTaskPriority {
			if !taskAdmin {
				http.Error(w, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority),
					http.StatusUnauthorized)
				return
			}
		}
		if err = model.SetTaskPriority(r.Context(), *projCtx.Task, priority, authUser.Username()); err != nil {
			http.Error(w, fmt.Sprintf("Error setting task priority %v: %v", projCtx.Task.Id, err), http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOneId(r.Context(), projCtx.Task.Id)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case "override_dependencies":
		overrideRequesters := []string{
			evergreen.PatchVersionRequester,
			evergreen.GithubPRRequester,
		}
		if !utility.StringSliceContains(overrideRequesters, projCtx.Task.Requester) && !taskAdmin {
			http.Error(w, "not authorized to override dependencies", http.StatusUnauthorized)
			return
		}
		err = projCtx.Task.SetOverrideDependencies(ctx, authUser.Username())
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
	vals := r.URL.Query()

	taskID := vars["task_id"]
	execution, err := strconv.Atoi(vars["task_execution"])
	if err != nil {
		http.Error(w, "invalid execution", http.StatusBadRequest)
		return
	}
	tsk, err := task.FindOneIdAndExecution(r.Context(), taskID, execution)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if tsk == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	testName := vars["test_name"]
	if testName == "" {
		testName = vals.Get("test_name")
	}
	it, err := tsk.GetTestLogs(r.Context(), task.TestLogGetOptions{LogPaths: []string{testName}})
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if vals.Get("text") == "true" || r.Header.Get("Content-Type") == "text/plain" {
		gimlet.WriteText(w, log.NewLogIteratorReader(it, log.LogIteratorReaderOptions{
			PrintTime:     true,
			PrintPriority: r.FormValue("priority") == "true",
			TimeZone:      getUserTimeZone(MustHaveUser(r)),
		}))
	} else {
		data := logData{
			Data: apimodels.StreamFromLogIterator(it),
			User: gimlet.GetUser(r.Context()),
		}
		uis.render.Stream(w, http.StatusOK, data, "base", "task_log.html")
	}
}
