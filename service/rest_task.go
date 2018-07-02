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
	Restarts            int                   `json:"restarts"`
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
	Logs      interface{}   `json:"logs"`
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
	destTask.Restarts = srcTask.Restarts
	destTask.Execution = srcTask.Execution
	destTask.Archived = srcTask.Archived
	destTask.RevisionOrderNumber = srcTask.RevisionOrderNumber
	destTask.Requester = srcTask.Requester
	destTask.Status = srcTask.Status
	destTask.Aborted = srcTask.Aborted
	destTask.TimeTaken = srcTask.TimeTaken
	destTask.ExpectedDuration = srcTask.ExpectedDuration

	var err error
	destTask.MinQueuePos, err = model.FindMinimumQueuePositionForTask(destTask.Id)
	if err != nil {
		msg := fmt.Sprintf("Error calculating task queue position for '%v'", srcTask.Id)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return
	}

	if destTask.MinQueuePos < 0 {
		destTask.MinQueuePos = 0
	}

	// Copy over the status details
	destTask.StatusDetails.TimedOut = srcTask.Details.TimedOut
	destTask.StatusDetails.TimeoutStage = srcTask.Details.Description

	// Copy over the test results
	testResults := srcTask.LocalTestResults
	if srcTask.DisplayOnly {
		testResults, err = srcTask.GetTestResultsForDisplayTask()
		if err != nil {
			err = errors.Wrapf(err, "Error finding test results for display task", srcTask.Id)
			grip.Error(err)
			gimlet.WriteJSONInternalError(w, responseError{Message: err.Error()})
			return
		}
	}
	destTask.LocalTestResults = make(taskTestResultsByName, len(testResults))
	for _, _testResult := range testResults {
		numSecs := _testResult.EndTime - _testResult.StartTime
		testResult := taskTestResult{
			Status:    _testResult.Status,
			TimeTaken: time.Duration(numSecs * float64(time.Second)),
			Logs:      taskTestLogURL{_testResult.URL},
		}
		destTask.LocalTestResults[_testResult.TestFile] = testResult
	}

	// Copy over artifacts and binaries
	entries, err := artifact.FindAll(artifact.ByTaskId(srcTask.Id))
	if err != nil {
		msg := fmt.Sprintf("Error finding task '%v'", srcTask.Id)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return

	}
	for _, entry := range entries {
		for _, _file := range entry.Files {
			file := taskFile{
				Name: _file.Name,
				URL:  _file.Link,
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

	result := taskStatusContent{
		Id:     task.Id,
		Name:   task.DisplayName,
		Status: task.Status,
	}

	// Copy over the status details
	result.StatusDetails.TimedOut = task.Details.TimedOut
	result.StatusDetails.TimeoutStage = task.Details.Description

	// Copy over the test results
	result.Tests = make(taskStatusByTest, len(task.LocalTestResults))
	for _, _testResult := range task.LocalTestResults {
		numSecs := _testResult.EndTime - _testResult.StartTime
		testResult := taskTestResult{
			Status:    _testResult.Status,
			TimeTaken: time.Duration(numSecs * float64(time.Second)),
			Logs:      taskTestLogURL{_testResult.URL},
		}
		result.Tests[_testResult.TestFile] = testResult
	}

	gimlet.WriteJSON(w, result)
}
