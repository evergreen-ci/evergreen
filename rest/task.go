package rest

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/gorilla/mux"
	"github.com/shelman/angier"
	"net/http"
	"time"
)

type taskStatusContent struct {
	Id            string            `json:"task_id"`
	Name          string            `json:"task_name"`
	Status        string            `json:"status"`
	StatusDetails taskStatusDetails `json:"status_details"`
	Tests         taskStatusByTest  `json:"tests"`
}

type task struct {
	Id                  string                `json:"id"`
	CreateTime          time.Time             `json:"create_time"`
	ScheduledTime       time.Time             `json:"scheduled_time"`
	DispatchTime        time.Time             `json:"dispatch_time"`
	StartTime           time.Time             `json:"start_time"`
	FinishTime          time.Time             `json:"finish_time"`
	PushTime            time.Time             `json:"push_time"`
	Version             string                `json:"version"`
	Project             string                `json:"project"`
	Revision            string                `json:"revision"`
	Priority            int                   `json:"priority"`
	LastHeartbeat       time.Time             `json:"last_heartbeat"`
	Activated           bool                  `json:"activated"`
	BuildId             string                `json:"build_id"`
	DistroId            string                `json:"distro"`
	BuildVariant        string                `json:"build_variant"`
	DependsOn           []model.Dependency    `json:"depends_on"`
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
	TestResults         taskTestResultsByName `json:"test_results"`
	MinQueuePos         int                   `json:"min_queue_pos"`

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
	Name string `json:"name"`
	URL  string `json:"url"`
}

type taskTestResultsByName map[string]taskTestResult

type taskStatusByTest map[string]taskTestResult

// Returns a JSON response with the marshalled output of the task
// specified in the request.
func (restapi restAPI) getTaskInfo(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]

	srcTask, err := model.FindTask(taskId)
	if err != nil || srcTask == nil {
		msg := fmt.Sprintf("Error finding task '%v'", taskId)
		statusCode := http.StatusNotFound

		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	destTask := &task{}
	// Copy the contents from the database into our local task type
	err = angier.TransferByFieldNames(srcTask, destTask)
	if err != nil {
		msg := fmt.Sprintf("Error finding task '%v'", taskId)
		evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return

	}

	// Copy over the status details
	destTask.StatusDetails.TimedOut = srcTask.Details.TimedOut
	destTask.StatusDetails.TimeoutStage = srcTask.Details.Description

	// Copy over the test results
	destTask.TestResults = make(taskTestResultsByName, len(srcTask.TestResults))
	for _, _testResult := range srcTask.TestResults {
		numSecs := _testResult.EndTime - _testResult.StartTime
		testResult := taskTestResult{
			Status:    _testResult.Status,
			TimeTaken: time.Duration(numSecs * float64(time.Second)),
			Logs:      taskTestLogURL{_testResult.URL},
		}
		destTask.TestResults[_testResult.TestFile] = testResult
	}

	// Copy over artifacts and binaries
	entries, err := artifact.FindAll(artifact.ByTaskId(taskId))
	if err != nil {
		msg := fmt.Sprintf("Error finding task '%v'", taskId)
		evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
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

	restapi.WriteJSON(w, http.StatusOK, destTask)
	return

}

// Returns a JSON response with the status of the specified task.
// The keys of the object are the test names.
func (restapi restAPI) getTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]

	task, err := model.FindTask(taskId)
	if err != nil || task == nil {
		msg := fmt.Sprintf("Error finding task '%v'", taskId)
		statusCode := http.StatusNotFound

		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "%v: %v", msg, err)
			statusCode = http.StatusInternalServerError
		}

		restapi.WriteJSON(w, statusCode, responseError{Message: msg})
		return

	}

	result := taskStatusContent{
		Id:     taskId,
		Name:   task.DisplayName,
		Status: task.Status,
	}

	// Copy over the status details
	result.StatusDetails.TimedOut = task.Details.TimedOut
	result.StatusDetails.TimeoutStage = task.Details.Description

	// Copy over the test results
	result.Tests = make(taskStatusByTest, len(task.TestResults))
	for _, _testResult := range task.TestResults {
		numSecs := _testResult.EndTime - _testResult.StartTime
		testResult := taskTestResult{
			Status:    _testResult.Status,
			TimeTaken: time.Duration(numSecs * float64(time.Second)),
			Logs:      taskTestLogURL{_testResult.URL},
		}
		result.Tests[_testResult.TestFile] = testResult
	}

	restapi.WriteJSON(w, http.StatusOK, result)
	return

}
