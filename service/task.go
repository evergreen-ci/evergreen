package service

import (
	"context"
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
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type uiTaskData struct {
	Id                   string                  `json:"id"`
	DisplayName          string                  `json:"display_name"`
	Revision             string                  `json:"gitspec"`
	BuildVariant         string                  `json:"build_variant"`
	Distro               string                  `json:"distro"`
	Container            string                  `json:"container,omitempty"`
	BuildId              string                  `json:"build_id"`
	Status               string                  `json:"status"`
	TaskWaiting          string                  `json:"task_waiting"`
	Activated            bool                    `json:"activated"`
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
	AbortInfo            task.AbortInfo          `json:"abort_info,omitempty"`
	MinQueuePos          int                     `json:"min_queue_pos"`
	DependsOn            []uiDep                 `json:"depends_on"`
	AbortedByDisplay     *abortedByDisplay       `json:"aborted_by_display,omitempty"`
	OverrideDependencies bool                    `json:"override_dependencies"`
	IngestTime           time.Time               `json:"ingest_time"`
	EstWaitTime          time.Duration           `json:"wait_time"`
	UpstreamData         *uiUpstreamData         `json:"upstream_data,omitempty"`

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

	// generated task info
	GeneratedById   string `json:"generated_by_id"`
	GeneratedByName string `json:"generated_by_name"`

	// CanSync indicates that the task can sync its working directory.
	CanSync bool `json:"can_sync"`
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
	TestResult testresult.TestResult `json:"test_result"`
	TaskId     string                `json:"task_id"`
	TaskName   string                `json:"task_name"`
	URL        string                `json:"url"`
	URLRaw     string                `json:"url_raw"`
	URLParsley string                `json:"url_parsley"`
}

type logData struct {
	Data chan apimodels.LogMessage
	User gimlet.User
}

type abortedByDisplay struct {
	TaskDisplayName     string `json:"task_display_name"`
	BuildVariantDisplay string `json:"build_variant_display"`
}

func (uis *UIServer) taskPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projCtx := MustHaveProjectContext(r)
	executionStr := gimlet.GetVars(r)["execution"]
	var execution int
	var err error

	if executionStr != "" {
		execution, err = strconv.Atoi(executionStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Bad execution number: %v", executionStr), http.StatusBadRequest)
			return
		}
	}

	if projCtx.Task == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if RedirectSpruceUsers(w, r, fmt.Sprintf("%s/task/%s?execution=%d", uis.Settings.Ui.UIv2Url, projCtx.Task.Id, execution)) {
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

	archived := false

	// if there is an execution number, the task might be in the old_tasks collection, so we
	// query that collection and set projCtx.Task to the old task if it exists.
	if executionStr != "" {
		// Construct the old task id.
		oldTaskId := task.MakeOldID(projCtx.Task.Id, execution)

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
	}

	// Build a struct containing the subset of task data needed for display in the UI
	tId := projCtx.Task.Id
	totalExecutions := projCtx.Task.Execution
	taskExecution := projCtx.Task.Execution
	if archived {
		tId = projCtx.Task.OldTaskId

		// Get total number of executions for executions drop down
		mostRecentExecution, err := task.FindOneId(tId)
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
		Container:            projCtx.Task.Container,
		BuildVariant:         projCtx.Task.BuildVariant,
		BuildId:              projCtx.Task.BuildId,
		Activated:            projCtx.Task.Activated,
		Execution:            taskExecution,
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
		AbortInfo:            projCtx.Task.AbortInfo,
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
		Archived:             archived,
		TotalExecutions:      totalExecutions,
		PartOfDisplay:        projCtx.Task.IsPartOfDisplay(),
		CanSync:              projCtx.Task.CanSync,
		GeneratedById:        projCtx.Task.GeneratedBy,
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
		uiTask.EstWaitTime, err = model.GetEstimatedStartTime(ctx, *projCtx.Task)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}
	if uiTask.GeneratedById != "" {
		var generator *task.Task
		generator, err = task.FindOneIdWithFields(uiTask.GeneratedById, task.DisplayNameKey)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		uiTask.GeneratedByName = generator.DisplayName
	}

	var taskHost *host.Host
	if projCtx.Task.HostId != "" {
		uiTask.HostDNS = projCtx.Task.HostId
		uiTask.HostId = projCtx.Task.HostId
		taskHost, err = host.FindOne(ctx, host.ById(projCtx.Task.HostId))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if taskHost != nil {
			uiTask.HostDNS = taskHost.Host
			// ensure that the ability to spawn is updated from the existing distro
			taskHost.Distro.SpawnAllowed = false
			var d *distro.Distro
			d, err = distro.FindOneId(ctx, taskHost.Distro.Id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if d != nil {
				taskHost.Distro.SpawnAllowed = d.SpawnAllowed
			}
		}
	}

	testResults := uis.getTestResults(r.Context(), projCtx, &uiTask)
	if projCtx.Patch != nil {
		var taskOnBaseCommit *task.Task
		var testResultsOnBaseCommit []testresult.TestResult
		taskOnBaseCommit, err = projCtx.Task.FindTaskOnBaseCommit()
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		taskPatch := &uiPatch{Patch: *projCtx.Patch}
		if taskOnBaseCommit != nil {
			if err = taskOnBaseCommit.PopulateTestResults(r.Context()); err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}

			taskPatch.BaseTaskId = taskOnBaseCommit.Id
			taskPatch.BaseTimeTaken = taskOnBaseCommit.TimeTaken
			testResultsOnBaseCommit = taskOnBaseCommit.LocalTestResults
		}
		taskPatch.StatusDiffs = model.StatusDiffTests(testResultsOnBaseCommit, testResults)
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

	usr := gimlet.GetUser(ctx)
	pluginContext := projCtx.ToPluginContext(uis.Settings, usr)
	pluginContext.Request = r
	pluginContent := getPluginDataAndHTML(uis, plugin.TaskPage, pluginContext)
	permissions := gimlet.Permissions{}
	if usr != nil {
		opts := gimlet.PermissionOpts{Resource: projCtx.ProjectRef.Id, ResourceType: evergreen.ProjectResourceType}
		permissions, err = rolemanager.HighestPermissionsForRoles(usr.Roles(), evergreen.GetEnvironment().RoleManager(), opts)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}
	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = fmt.Sprintf("%s/task/%s?execution=%d", uis.Settings.Ui.UIv2Url, tId, taskExecution)
	}

	if uiTask.AbortInfo.TaskID != "" {
		abortedBy, err := getAbortedBy(projCtx.Task.AbortInfo.TaskID)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		uiTask.AbortedByDisplay = abortedBy
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		EvgBaseUrl    string
		Task          uiTaskData
		Host          *host.Host
		PluginContent pluginData
		JiraHost      string
		Permissions   gimlet.Permissions
		NewUILink     string
		ViewData
	}{uis.Settings.Ui.Url, uiTask, taskHost, pluginContent, uis.Settings.Jira.Host, permissions, newUILink, uis.GetCommonViewData(w, r, false, true)}, "base", "task.html", "base_angular.html", "menu.html")
}

func getAbortedBy(abortedByTaskId string) (*abortedByDisplay, error) {
	abortedTask, err := task.FindOneId(abortedByTaskId)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting abortedBy task")
	}
	buildDisplay, err := build.FindOne(build.ById(abortedTask.BuildId))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting abortedBy build")
	}
	if buildDisplay == nil || abortedTask == nil {
		return nil, errors.New("problem getting abortBy display information")
	}
	abortedBy := &abortedByDisplay{
		TaskDisplayName:     abortedTask.DisplayName,
		BuildVariantDisplay: buildDisplay.DisplayName,
	}

	return abortedBy, nil
}

type taskHistoryPageData struct {
	TaskName    string
	Tasks       []bson.M
	Variants    []string
	FailedTests map[string][]string
	Versions    []model.Version

	// Flags that indicate whether the beginning/end of history has been reached
	ExhaustedBefore bool
	ExhaustedAfter  bool

	// The revision for which the surrounding history was requested
	SelectedRevision string
}

// the task's most recent log messages
const DefaultLogMessages = 100 // passed as a limit, so 0 means don't limit

// getTaskDependencies returns the uiDeps for the task and its status (either its original status,
// "blocked", or "pending")
func getTaskDependencies(t *task.Task) ([]uiDep, string, error) {
	depIds := []string{}
	for _, dep := range t.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}
	dependencies, err := task.FindWithFields(task.ByIds(depIds), task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey)
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
		})
	}

	if err = t.CircularDependencies(); err != nil {
		return nil, "", err
	}
	state, err := t.BlockedState(taskMap)
	if err != nil {
		return nil, "", errors.Wrap(err, "can't get blocked state")
	}

	return uiDependencies, state, nil
}

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
			tsk, err = task.FindOneIdAndExecution(tsk.Id, execution)
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
		loggedEvents, err := event.Find(event.MostRecentTaskEvents(projCtx.Task.Id, DefaultLogMessages))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		gimlet.WriteJSON(w, loggedEvents)
		return
	}

	it, err := tsk.GetTaskLogs(r.Context(), taskoutput.TaskLogGetOptions{
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
			tsk, err = task.FindOneIdAndExecution(tsk.Id, execution)
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

	it, err := tsk.GetTaskLogs(r.Context(), taskoutput.TaskLogGetOptions{LogType: getTaskLogTypeMapping(r.FormValue("type"))})
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

func getTaskLogTypeMapping(prefix string) taskoutput.TaskLogType {
	switch prefix {
	case apimodels.AgentLogPrefix:
		return taskoutput.TaskLogTypeAgent
	case apimodels.SystemLogPrefix:
		return taskoutput.TaskLogTypeSystem
	case apimodels.TaskLogPrefix:
		return taskoutput.TaskLogTypeTask
	default:
		return taskoutput.TaskLogTypeAll
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

	taskFiles, err := artifact.GetAllArtifacts([]artifact.TaskIDAndExecution{{TaskID: projCtx.Task.Id, Execution: executionNum}})
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
		projCtx.Task, err = task.FindOneId(projCtx.Task.Id)
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
		projCtx.Task, err = task.FindOneId(projCtx.Task.Id)

		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		gimlet.WriteJSON(w, projCtx.Task)
		return
	case evergreen.SetActiveAction:
		active := putParams.Active
		if active && projCtx.Task.Requester == evergreen.MergeTestRequester {
			http.Error(w, "commit queue tasks cannot be manually scheduled", http.StatusBadRequest)
			return
		}
		if err = model.SetActiveState(r.Context(), authUser.Username(), active, *projCtx.Task); err != nil {
			http.Error(w, fmt.Sprintf("Error activating task %v: %v", projCtx.Task.Id, err),
				http.StatusInternalServerError)
			return
		}

		// Reload the task from db, send it back
		projCtx.Task, err = task.FindOneId(projCtx.Task.Id)
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
		projCtx.Task, err = task.FindOneId(projCtx.Task.Id)
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
	vals := r.URL.Query()

	taskID := vars["task_id"]
	execution, err := strconv.Atoi(vars["task_execution"])
	if err != nil {
		http.Error(w, "invalid execution", http.StatusBadRequest)
		return
	}
	tsk, err := task.FindOneIdAndExecution(taskID, execution)
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
	it, err := tsk.GetTestLogs(r.Context(), taskoutput.TestLogGetOptions{LogPaths: []string{testName}})
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

func (uis *UIServer) getTestResults(ctx context.Context, projCtx projectContext, uiTask *uiTaskData) []testresult.TestResult {
	if err := projCtx.Task.PopulateTestResults(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"task_id": projCtx.Task.Id,
			"message": "fetching test results for task",
		}))
		return nil
	}

	uiTask.TestResults = []uiTestResult{}
	if uiTask.DisplayOnly {
		execTaskDisplayNameMap := map[string]string{}
		for _, t := range projCtx.Task.ExecutionTasks {
			var (
				et  *task.Task
				err error
			)
			if uiTask.Archived {
				et, err = task.FindOneOldByIdAndExecution(t, projCtx.Task.Execution)
			} else {
				et, err = task.FindOneId(t)
			}
			if err != nil {
				grip.Error(message.Fields{
					"message":        "fetching test results for execution task",
					"task_id":        t,
					"parent_task_id": projCtx.Task.Id,
				})
				return nil
			}
			if et == nil {
				grip.Error(message.Fields{
					"message":        "execution task not found",
					"task_id":        t,
					"parent_task_id": projCtx.Task.Id,
				})
				continue
			}

			execTaskDisplayNameMap[t] = et.DisplayName
			uiTask.ExecutionTasks = append(uiTask.ExecutionTasks, uiExecTask{
				Id:        t,
				Name:      et.DisplayName,
				TimeTaken: et.TimeTaken,
				Status:    et.ResultStatus(),
			})
		}

		for _, tr := range projCtx.Task.LocalTestResults {
			uiTask.TestResults = append(uiTask.TestResults, uiTestResult{
				TestResult: tr,
				TaskId:     tr.TaskID,
				TaskName:   execTaskDisplayNameMap[tr.TaskID],
				URL:        tr.GetLogURL(uis.env, evergreen.LogViewerHTML),
				URLRaw:     tr.GetLogURL(uis.env, evergreen.LogViewerRaw),
				URLParsley: tr.GetLogURL(uis.env, evergreen.LogViewerParsley),
			})
		}
	} else {
		for _, tr := range projCtx.Context.Task.LocalTestResults {
			uiTask.TestResults = append(uiTask.TestResults, uiTestResult{
				TestResult: tr,
				TaskId:     tr.TaskID,
				URL:        tr.GetLogURL(uis.env, evergreen.LogViewerHTML),
				URLRaw:     tr.GetLogURL(uis.env, evergreen.LogViewerRaw),
				URLParsley: tr.GetLogURL(uis.env, evergreen.LogViewerParsley),
			})
		}

		if uiTask.PartOfDisplay {
			// Display task ID would've been populated when setting PartOfDisplay.
			uiTask.DisplayTaskID = utility.FromStringPtr(projCtx.Task.DisplayTaskId)
		}
	}

	return projCtx.Task.LocalTestResults
}
