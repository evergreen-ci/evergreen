package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/task"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////////////
//
// GET /tasks/{task_id}/tests

type testGetHandler struct {
	taskID     string
	testStatus []string
	testName   string
	key        string
	limit      int
	sc         data.Connector
	latest     bool

	task *task.Task
}

func makeFetchTestsForTask(sc data.Connector) gimlet.RouteHandler {
	return &testGetHandler{
		sc: sc,
	}
}

func (hgh *testGetHandler) Factory() gimlet.RouteHandler {
	return &testGetHandler{
		sc: hgh.sc,
	}
}

func (tgh *testGetHandler) Parse(ctx context.Context, r *http.Request) error {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
		}
	}
	tgh.taskID = gimlet.GetVars(r)["task_id"]

	var err error
	vals := r.URL.Query()
	executionStr := vals.Get("execution")
	tgh.latest = vals.Get("latest") == "true"

	if tgh.latest && executionStr != "" {
		return gimlet.ErrorResponse{
			Message:    "cannot specify both latest and execution",
			StatusCode: http.StatusBadRequest,
		}
	} else if tgh.latest {
		tgh.task = projCtx.Task
	} else {
		// Default to first execution unless latest is specified to
		// maintain backwards compatibility.
		execution := 0
		if executionStr != "" {
			execution, err = strconv.Atoi(executionStr)
			if err != nil {
				return gimlet.ErrorResponse{
					Message:    "invalid execution",
					StatusCode: http.StatusBadRequest,
				}
			}
		}
		taskByExecution, err := task.FindOneIdAndExecution(tgh.taskID, execution)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    errors.Wrapf(err, "finding execution '%d' for task '%s'", execution, tgh.taskID).Error(),
			}
		}
		if taskByExecution == nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task '%s' not found", tgh.taskID),
			}
		}
		tgh.task = taskByExecution
	}

	if status := vals.Get("status"); status != "" {
		tgh.testStatus = []string{status}
	}
	tgh.key = vals.Get("start_at")
	tgh.testName = vals.Get("test_name")
	tgh.limit, err = getLimit(vals)
	if err != nil {
		return errors.Wrap(err, "getting limit")
	}

	return nil
}

func (tgh *testGetHandler) Run(ctx context.Context) gimlet.Responder {
	var (
		page int
		err  error
	)
	if tgh.key != "" {
		page, err = strconv.Atoi(tgh.key)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.New("invalid 'start at' time"))
		}
	}

	cedarTestResults, status, err := apimodels.GetCedarTestResults(ctx, apimodels.GetCedarTestResultsOptions{
		BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		TaskID:      tgh.taskID,
		Execution:   utility.ToIntPtr(tgh.task.Execution),
		DisplayTask: tgh.task.DisplayOnly,
		TestName:    tgh.testName,
		Statuses:    tgh.testStatus,
		Limit:       tgh.limit,
		Page:        page,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting test results"))
	}
	if status != http.StatusOK && status != http.StatusNotFound {
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("getting test results from Cedar returned status %d", status))
	}

	var key string
	if page*tgh.limit < utility.FromIntPtr(cedarTestResults.Stats.FilteredCount) {
		key = fmt.Sprintf("%d", page+1)
	}

	return tgh.buildResponse(cedarTestResults.Results, key)
}

func (tgh *testGetHandler) buildResponse(cedarTestResults []apimodels.CedarTestResult, key string) gimlet.Responder {
	resp := gimlet.NewResponseBuilder()
	if err := resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting response format"))
	}

	if key != "" {
		err := resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tgh.sc.GetURL(),
				Key:             key,
				Limit:           tgh.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	for i, testResult := range cedarTestResults {
		if err := tgh.addDataToResponse(resp, &testResult); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding Cedar test result at index %d", i))
		}
	}

	return resp
}

func (tgh *testGetHandler) addDataToResponse(resp gimlet.Responder, testResult interface{}) error {
	at := &model.APITest{}
	if err := at.BuildFromService(tgh.taskID); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "adding task ID to task API model").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	if err := at.BuildFromService(testResult); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "adding test result to task API model").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	if err := resp.AddData(at); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "adding response data for task '%s'", tgh.taskID).Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	return nil
}

///////////////////////////////////////////////////////////////////////////////
//
// GET /tasks/{task_id}/tests/count

type testCountGetHandler struct {
	taskID    string
	execution int
}

func makeFetchTestCountForTask() gimlet.RouteHandler {
	return &testCountGetHandler{}
}

func (h *testCountGetHandler) Factory() gimlet.RouteHandler {
	return &testCountGetHandler{}
}

func (h *testCountGetHandler) Parse(ctx context.Context, r *http.Request) error {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
			Message:    "task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	h.taskID = projCtx.Task.Id

	var err error
	vals := r.URL.Query()
	execution := vals.Get("execution")
	if execution != "" {
		h.execution, err = strconv.Atoi(execution)
		if err != nil {
			return errors.Wrap(err, "invalid execution")
		}
	}

	return nil
}

func (h *testCountGetHandler) Run(ctx context.Context) gimlet.Responder {
	count, err := data.CountTestsByTaskID(ctx, h.taskID, h.execution)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewTextResponse(count)
}
