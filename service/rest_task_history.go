package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
)

const (
	// Number of revisions to return in task history
	MaxRestNumRevisions = 10
)

type RestTestHistoryResult struct {
	TestFile     string        `json:"test_file" csv:"test_file"`
	TaskName     string        `json:"task_name" csv:"task_name"`
	TestStatus   string        `json:"test_status" csv:"test_status"`
	TaskStatus   string        `json:"task_status" csv:"task_status"`
	Revision     string        `json:"revision" csv:"revision"`
	Project      string        `json:"project" csv:"project"`
	TaskId       string        `json:"task_id" csv:"task_id"`
	BuildVariant string        `json:"variant" csv:"variant"`
	StartTime    time.Time     `json:"start_time" csv:"start_time"`
	EndTime      time.Time     `json:"end_time" csv:"end_time"`
	DurationMS   time.Duration `json:"duration" csv:"duration"`
	Execution    int           `json:"execution" csv:"execution"`
	Url          string        `json:"url" csv:"url"`
	UrlRaw       string        `json:"url_raw" csv:"url_raw"`
}

func (restapi restAPI) getTaskHistory(w http.ResponseWriter, r *http.Request) {
	taskName := mux.Vars(r)["task_name"]
	projCtx := MustHaveRESTContext(r)
	project := projCtx.Project
	if project == nil {
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: "error loading project"})
		return
	}

	buildVariants := project.GetVariantsWithTask(taskName)
	iter := model.NewTaskHistoryIterator(taskName, buildVariants, project.Identifier)

	chunk, err := iter.GetChunk(nil, MaxRestNumRevisions, NoRevisions, false)
	if err != nil {
		msg := fmt.Sprintf("Error finding history for task '%v'", taskName)
		grip.Errorf("%v: %+v", msg, err)
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: msg})
		return
	}

	restapi.WriteJSON(w, http.StatusOK, chunk)
	return

}

// logURL returns the full URL for linking to a test's logs.
// Returns the empty string if no internal or external log is referenced.
func logURL(url, logId, root string) string {
	if logId != "" {
		return root + "/test_log/" + logId
	}
	return url
}

// getTestHistory retrieves the test history query parameters from the request
// and passes them to the function that gets the test results.
func (restapi restAPI) GetTestHistory(w http.ResponseWriter, r *http.Request) {
	projectId := mux.Vars(r)["project_id"]
	if projectId == "" {
		restapi.WriteJSON(w, http.StatusInternalServerError, responseError{Message: "invalid project id"})
		return
	}
	params := model.TestHistoryParameters{}
	params.Project = projectId
	params.TaskNames = util.GetStringArrayValue(r, "tasks", []string{})
	params.TestNames = util.GetStringArrayValue(r, "tests", []string{})
	params.BuildVariants = util.GetStringArrayValue(r, "variants", []string{})
	params.TestStatuses = util.GetStringArrayValue(r, "testStatuses", []string{})
	params.TaskStatuses = util.GetStringArrayValue(r, "taskStatuses", []string{})

	var err error
	params.Limit, err = util.GetIntValue(r, "limit", 0)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, "invalid value for field 'limit'")
		return
	}

	if len(params.TaskStatuses) == 0 {
		params.TaskStatuses = []string{evergreen.TaskFailed}
	}
	if len(params.TestStatuses) == 0 {
		params.TestStatuses = []string{evergreen.TestFailedStatus}
	}

	params.BeforeRevision = r.FormValue("beforeRevision")
	params.AfterRevision = r.FormValue("afterRevision")

	beforeDate := r.FormValue("beforeDate")
	if beforeDate != "" {
		params.BeforeDate, err = time.Parse(time.RFC3339, beforeDate)
		if err != nil {
			restapi.WriteJSON(w, http.StatusBadRequest, "invalid format for field 'before date'")
			return
		}
	}

	afterDate := r.FormValue("afterDate")
	if afterDate != "" {
		params.AfterDate, err = time.Parse(time.RFC3339, afterDate)
		if err != nil {
			restapi.WriteJSON(w, http.StatusBadRequest, "invalid format for field 'after date'")
			return
		}
	}

	sort := r.FormValue("sort")
	switch sort {
	case "earliest":
		params.Sort = 1
	case "", "latest":
		params.Sort = -1
	default:
		restapi.WriteJSON(w, http.StatusBadRequest, "invalid sort, must be earliest or latest")
		return
	}

	// export format
	isCSV, err := util.GetBoolValue(r, "csv", false)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	err = params.SetDefaultsAndValidate()
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	results, err := model.GetTestHistory(&params)
	if err != nil {
		restapi.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	restHistoryResults := []RestTestHistoryResult{}
	for _, result := range results {
		startTime := time.Unix(int64(result.StartTime), 0)
		endTime := time.Unix(int64(result.EndTime), 0)
		taskStatus := result.TaskStatus
		if result.TaskStatus == evergreen.TaskFailed {
			if result.TaskTimedOut {
				taskStatus = model.TaskTimeout
			}
			if result.TaskDetailsType == "system" {
				taskStatus = model.TaskSystemFailure
			}
		}
		url := logURL(result.Url, result.LogId, restapi.GetSettings().Ui.Url)
		restHistoryResults = append(restHistoryResults, RestTestHistoryResult{
			TestFile:     result.TestFile,
			TaskName:     result.TaskName,
			TestStatus:   result.TestStatus,
			TaskStatus:   taskStatus,
			Revision:     result.Revision,
			Project:      result.Project,
			TaskId:       result.TaskId,
			BuildVariant: result.BuildVariant,
			StartTime:    startTime,
			EndTime:      endTime,
			DurationMS:   endTime.Sub(startTime),
			Url:          url,
			UrlRaw:       result.UrlRaw,
			Execution:    result.Execution,
		})
	}
	if isCSV {
		util.WriteCSVResponse(w, http.StatusOK, restHistoryResults)
		return
	}
	restapi.WriteJSON(w, http.StatusOK, restHistoryResults)

}
