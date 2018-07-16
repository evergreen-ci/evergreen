package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
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
	taskName := gimlet.GetVars(r)["task_name"]
	projectID := r.FormValue("project_id")
	if projectID == "" {
		gimlet.WriteJSONInternalError(w, responseError{Message: "project id must not be empty"})
		return
	}
	projectRef, err := model.FindOneProjectRef(projectID)
	if err != nil || projectRef == nil {
		gimlet.WriteJSONInternalError(w, responseError{Message: "error loading project"})
		return
	}
	project, err := model.FindProject("", projectRef)
	if err != nil || project == nil {
		gimlet.WriteJSONInternalError(w, responseError{Message: "error loading project"})
		return
	}

	buildVariants := project.GetVariantsWithTask(taskName)
	iter := model.NewTaskHistoryIterator(taskName, buildVariants, project.Identifier)

	chunk, err := iter.GetChunk(nil, MaxRestNumRevisions, NoRevisions, false)
	if err != nil {
		msg := fmt.Sprintf("Error finding history for task '%v'", taskName)
		grip.Errorf("%v: %+v", msg, err)
		gimlet.WriteJSONInternalError(w, responseError{Message: msg})
		return
	}

	gimlet.WriteJSON(w, chunk)
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
	projectId := gimlet.GetVars(r)["project_id"]
	if projectId == "" {
		gimlet.WriteJSONInternalError(w, responseError{Message: "invalid project id"})
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
		gimlet.WriteJSONError(w, "invalid value for field 'limit'")
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
			gimlet.WriteJSONError(w, "invalid format for field 'before date'")
			return
		}
	}

	afterDate := r.FormValue("afterDate")
	if afterDate != "" {
		params.AfterDate, err = time.Parse(time.RFC3339, afterDate)
		if err != nil {
			gimlet.WriteJSONError(w, "invalid format for field 'after date'")
			return
		}
	}

	switch r.FormValue("requestSource") {
	case "patch", "patch_request":
		params.TaskRequestType = evergreen.PatchVersionRequester
	case "", "commit", "gitter", "gitter_request", "repotracker":
		params.TaskRequestType = evergreen.RepotrackerVersionRequester
	case "all", "both", "any":
		params.TaskRequestType = ""
	default:
		gimlet.WriteJSONError(w,
			fmt.Sprintf("invalid request type '%s', should be 'patch' or 'commit'",
				r.FormValue("requestSource")))
		return
	}

	sort := r.FormValue("sort")
	switch sort {
	case "earliest":
		params.Sort = 1
	case "", "latest":
		params.Sort = -1
	default:
		gimlet.WriteJSONError(w, "invalid sort, must be earliest or latest")
		return
	}

	// export format
	isCSV, err := util.GetBoolValue(r, "csv", false)
	if err != nil {
		gimlet.WriteJSONError(w, err.Error())
		return
	}

	err = params.SetDefaultsAndValidate()
	if err != nil {
		gimlet.WriteJSONError(w, err.Error())
		return
	}
	var results []model.TestHistoryResult
	if params.BeforeRevision == "" && params.AfterRevision == "" &&
		params.BeforeDate.IsZero() && params.AfterDate.IsZero() {
		// if the task results could be unbounded, use the aggregation version of the query
		results, err = model.GetTestHistory(&params)
	} else {
		results, err = model.GetTestHistoryV2(&params)
	}
	if err != nil {
		gimlet.WriteJSONError(w, err.Error())
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
			if result.TaskDetailsType == evergreen.CommandTypeSystem {
				taskStatus = model.TaskSystemFailure
			} else if result.TaskDetailsType == evergreen.CommandTypeSetup {
				taskStatus = model.TaskSetupFailure
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
	gimlet.WriteJSON(w, restHistoryResults)

}
