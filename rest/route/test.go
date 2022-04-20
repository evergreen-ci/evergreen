package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////////////
//
// GET  /tasks/{task_id}/tests

type testGetHandler struct {
	taskID        string
	displayTask   bool
	cedarResults  bool
	testStatus    []string
	testID        string
	testName      string
	testExecution int
	key           string
	limit         int
	sc            data.Connector
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
			Message:    "task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	tgh.taskID = projCtx.Task.Id
	tgh.displayTask = projCtx.Task.DisplayOnly
	tgh.cedarResults = projCtx.Task.HasCedarResults

	var err error
	vals := r.URL.Query()
	execution := vals.Get("execution")

	if execution != "" {
		tgh.testExecution, err = strconv.Atoi(execution)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "invalid execution",
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	if status := vals.Get("status"); status != "" {
		tgh.testStatus = []string{status}
	}
	tgh.key = vals.Get("start_at")
	tgh.testName = vals.Get("test_name")
	tgh.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (tgh *testGetHandler) Run(ctx context.Context) gimlet.Responder {
	var err error
	var key string

	if tgh.cedarResults {
		var page int
		if tgh.key != "" {
			page, err = strconv.Atoi(tgh.key)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(errors.New("invalid start_at"))
			}
		}

		cedarTestResults, status, err := apimodels.GetCedarTestResults(ctx, apimodels.GetCedarTestResultsOptions{
			BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:      tgh.taskID,
			Execution:   utility.ToIntPtr(tgh.testExecution),
			DisplayTask: tgh.displayTask,
			TestName:    tgh.testName,
			Statuses:    tgh.testStatus,
			Limit:       tgh.limit,
			Page:        page,
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting test results"))
		}
		if status != http.StatusOK && status != http.StatusNotFound {
			return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("getting test results from Cedar returned status '%d'", status))
		}

		if page*tgh.limit < utility.FromIntPtr(cedarTestResults.Stats.FilteredCount) {
			key = fmt.Sprintf("%d", page+1)
		}

		return tgh.buildResponse(cedarTestResults.Results, nil, key)
	}

	var tests []testresult.TestResult
	if tgh.testID != "" {
		// When testID is populated, search the test results collection
		// for the given ID.
		tests, err = data.FindTestById(tgh.testID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
		}
	} else {
		// we're going in here, and we've provided nothing so the limit is 101
		tests, err = tgh.sc.FindTestsByTaskId(data.FindTestsByTaskIdOpts{
			Execution: tgh.testExecution,
			Limit:     tgh.limit + 1,
			Statuses:  tgh.testStatus, // we haven't provided a status or execution or test name
			TaskID:    tgh.taskID,
			TestID:    tgh.key, // TESTID IS KEY
			TestName:  tgh.testName,
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
		}

		lastIndex := len(tests)
		if lastIndex > tgh.limit {
			key = string(tests[tgh.limit].ID)
			lastIndex = tgh.limit
		}
		// Truncate the test results to just those that will be
		// returned.
		tests = tests[:lastIndex]
	}

	return tgh.buildResponse(nil, tests, key)
}

func (tgh *testGetHandler) buildResponse(cedarTestResults []apimodels.CedarTestResult, testResults []testresult.TestResult, key string) gimlet.Responder {
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

	for _, testResult := range cedarTestResults {
		if err := tgh.addDataToResponse(resp, &testResult); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}
	for _, testResult := range testResults {
		if err := tgh.addDataToResponse(resp, &testResult); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

func (tgh *testGetHandler) addDataToResponse(resp gimlet.Responder, testResult interface{}) error {
	at := &model.APITest{}
	if err := at.BuildFromService(tgh.taskID); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "model error").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	if err := at.BuildFromService(testResult); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "model error").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	if err := resp.AddData(at); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "building response").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	return nil
}

///////////////////////////////////////////////////////////////////////////////
//
// GET  /tasks/{task_id}/tests/count

type testCountGetHandler struct {
	taskID    string
	execution int
	sc        data.Connector
}

func makeFetchTestCountForTask(sc data.Connector) gimlet.RouteHandler {
	return &testCountGetHandler{
		sc: sc,
	}
}

func (h *testCountGetHandler) Factory() gimlet.RouteHandler {
	return &testCountGetHandler{
		sc: h.sc,
	}
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
			return gimlet.ErrorResponse{
				Message:    "invalid execution",
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	return nil
}

func (h *testCountGetHandler) Run(ctx context.Context) gimlet.Responder {
	count, err := h.sc.TestCountByTaskID(ctx, h.taskID, h.execution)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
	}

	return gimlet.NewTextResponse(count)
}
