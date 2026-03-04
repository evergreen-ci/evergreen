package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type taskStatusContent struct {
	Id            string            `json:"task_id"`
	Name          string            `json:"task_name"`
	Status        string            `json:"status"`
	StatusDetails taskStatusDetails `json:"status_details"`
	Tests         taskStatusByTest  `json:"tests"`
}

type RestTask struct {
	Id                  string                `json:"id"`
	CreateTime          time.Time             `json:"create_time"`
	ScheduledTime       time.Time             `json:"scheduled_time"`
	DispatchTime        time.Time             `json:"dispatch_time"`
	StartTime           time.Time             `json:"start_time"`
	FinishTime          time.Time             `json:"finish_time"`
	Version             string                `json:"version"`
	Project             string                `json:"project"`
	Revision            string                `json:"revision"`
	Priority            int64                 `json:"priority"`
	LastHeartbeat       time.Time             `json:"last_heartbeat"`
	Activated           bool                  `json:"activated"`
	BuildId             string                `json:"build_id"`
	DistroId            string                `json:"distro"`
	BuildVariant        string                `json:"build_variant"`
	DependsOn           []task.Dependency     `json:"depends_on"`
	DisplayName         string                `json:"display_name"`
	HostId              string                `json:"host_id"`
	Execution           int                   `json:"execution"`
	Archived            bool                  `json:"archived"`
	RevisionOrderNumber int                   `json:"order"`
	Requester           string                `json:"requester"`
	Status              string                `json:"status"`
	StatusDetails       taskStatusDetails     `json:"status_details"`
	Aborted             bool                  `json:"aborted"`
	TimeTaken           time.Duration         `json:"time_taken"`
	ExpectedDuration    time.Duration         `json:"expected_duration"`
	LocalTestResults    taskTestResultsByName `json:"test_results"`
	MinQueuePos         int                   `json:"min_queue_pos"`
	PatchNumber         int                   `json:"patch_number,omitempty"`
	PatchId             string                `json:"patch_id,omitempty"`
	ModulePaths         map[string]string     `json:"module_paths,omitempty"`

	// Artifacts and binaries
	Files []taskFile `json:"files"`
}

type taskStatusDetails struct {
	TimedOut     bool   `json:"timed_out"`
	TimeoutStage string `json:"timeout_stage"`
}

type taskTestResult struct {
	Status    string        `json:"status"`
	TimeTaken time.Duration `json:"time_taken"`
	Logs      any           `json:"logs"`
}

type taskTestLogURL struct {
	URL string `json:"url"`
}

type taskFile struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	IgnoreForFetch bool   `json:"ignore_for_fetch"`
}

type taskTestResultsByName map[string]taskTestResult

type taskStatusByTest map[string]taskTestResult

// Returns a JSON response with the marshaled output of the task
// specified in the request.
func (restapi restAPI) getTaskInfo(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	srcTask := projCtx.Task
	if srcTask == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error finding task"})
		return
	}

	destTask := &RestTask{}
	destTask.Id = srcTask.Id
	destTask.CreateTime = srcTask.CreateTime
	destTask.ScheduledTime = srcTask.ScheduledTime
	destTask.DispatchTime = srcTask.DispatchTime
	destTask.StartTime = srcTask.StartTime
	destTask.FinishTime = srcTask.FinishTime
	destTask.Version = srcTask.Version
	destTask.Project = srcTask.Project
	destTask.Revision = srcTask.Revision
	destTask.Priority = srcTask.Priority
	destTask.LastHeartbeat = srcTask.LastHeartbeat
	destTask.Activated = srcTask.Activated
	destTask.BuildId = srcTask.BuildId
	destTask.DistroId = srcTask.DistroId
	destTask.BuildVariant = srcTask.BuildVariant
	destTask.DependsOn = srcTask.DependsOn
	destTask.DisplayName = srcTask.DisplayName
	destTask.HostId = srcTask.HostId
	destTask.Execution = srcTask.Execution
	destTask.Archived = srcTask.Archived
	destTask.RevisionOrderNumber = srcTask.RevisionOrderNumber
	destTask.Requester = srcTask.Requester
	destTask.Status = srcTask.Status
	destTask.Aborted = srcTask.Aborted
	destTask.TimeTaken = srcTask.TimeTaken
	destTask.ExpectedDuration = srcTask.ExpectedDuration
	destTask.ModulePaths = srcTask.Details.Modules.Prefixes

	var err error
	destTask.MinQueuePos, err = model.FindMinimumQueuePositionForTask(r.Context(), destTask.Id)
	if err != nil {
		msg := fmt.Sprintf("Error calculating task queue position for '%v'", srcTask.Id)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return
	}

	if destTask.MinQueuePos < 0 {
		destTask.MinQueuePos = 0
	}

	// Copy over the status details.
	destTask.StatusDetails.TimedOut = srcTask.Details.TimedOut
	destTask.StatusDetails.TimeoutStage = srcTask.Details.Description

	// Copy over the test results.
	if err := srcTask.PopulateTestResults(r.Context()); err != nil {
		err = errors.Wrapf(err, "Error finding test results for task '%s'", srcTask.Id)
		grip.Error(err)
		gimlet.WriteJSONInternalError(w, responseError{Message: err.Error()})
		return
	}
	destTask.LocalTestResults = make(taskTestResultsByName, len(srcTask.LocalTestResults))
	for _, tr := range srcTask.LocalTestResults {
		testResult := taskTestResult{
			Status:    tr.Status,
			TimeTaken: tr.Duration(),
			Logs:      taskTestLogURL{tr.LogURL},
		}
		destTask.LocalTestResults[tr.TestName] = testResult
	}

	// Copy over artifacts and binaries.
	entries, err := artifact.FindAll(r.Context(), artifact.ByTaskId(srcTask.Id))
	if err != nil {
		msg := fmt.Sprintf("Error finding task '%s'", srcTask.Id)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return

	}
	for _, entry := range entries {
		strippedFiles, err := artifact.StripHiddenFiles(r.Context(), entry.Files, true)
		if err != nil {
			msg := fmt.Sprintf("Error getting artifact files for task '%s'", srcTask.Id)
			grip.Error(message.WrapError(err, message.Fields{
				"message": msg,
				"task":    srcTask.Id,
			}))
			gimlet.WriteJSONInternalError(w, responseError{Message: msg})
			return
		}

		for _, f := range strippedFiles {
			file := taskFile{
				Name:           f.Name,
				URL:            f.Link,
				IgnoreForFetch: f.IgnoreForFetch,
			}
			destTask.Files = append(destTask.Files, file)
		}
	}

	if projCtx.Patch != nil {
		destTask.PatchNumber = projCtx.Patch.PatchNumber
		destTask.PatchId = projCtx.Patch.Id.Hex()
	}
	gimlet.WriteJSON(w, destTask)
}

// getTaskStatus returns a JSON response with the status of the specified task.
// The keys of the object are the test names.
func (restapi restAPI) getTaskStatus(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveRESTContext(r)
	task := projCtx.Task
	if task == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error finding task"})
		return
	}
	if err := task.PopulateTestResults(r.Context()); err != nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound, responseError{Message: "error populating test results"})
		return
	}

	result := taskStatusContent{
		Id:     task.Id,
		Name:   task.DisplayName,
		Status: task.Status,
	}

	// Copy over the status details
	result.StatusDetails.TimedOut = task.Details.TimedOut
	// TODO DEVPROD-9694: Stop storing failing command in Description
	if task.Details.Description == "" {
		result.StatusDetails.TimeoutStage = task.Details.FailingCommand
	} else {
		result.StatusDetails.TimeoutStage = task.Details.Description
	}

	// Copy over the test results
	result.Tests = make(taskStatusByTest, len(task.LocalTestResults))
	for _, _testResult := range task.LocalTestResults {
		testResult := taskTestResult{
			Status:    _testResult.Status,
			TimeTaken: _testResult.Duration(),
			Logs:      taskTestLogURL{_testResult.LogURL},
		}
		result.Tests[_testResult.TestName] = testResult
	}

	gimlet.WriteJSON(w, result)
}
