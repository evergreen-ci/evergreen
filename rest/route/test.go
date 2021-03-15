package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// testGetHandler is the MethodHandler for the GET /tasks/{task_id}/tests route.
type testGetHandler struct {
	taskID        string
	testStatus    string
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

// ParseAndValidate fetches the task Id and 'status' from the url and
// sets them as part of the args.
func (tgh *testGetHandler) Parse(ctx context.Context, r *http.Request) error {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
			Message:    "task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	tgh.taskID = projCtx.Task.Id

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

	tgh.testStatus = vals.Get("status")
	tgh.key = vals.Get("start_at")
	tgh.testName = vals.Get("test_name")
	tgh.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (tgh *testGetHandler) Run(ctx context.Context) gimlet.Responder {
	var (
		tests []testresult.TestResult
		key   string
	)

	// Check cedar test results first.
	dbTask, err := task.FindByIdExecution(tgh.taskID, &tgh.testExecution)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
	} else if dbTask == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "not found",
			StatusCode: http.StatusNotFound,
		})
	}
	opts := apimodels.GetCedarTestResultsOptions{
		BaseURL:   evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		Execution: tgh.testExecution,
	}
	if len(dbTask.ExecutionTasks) > 0 {
		opts.DisplayTaskID = tgh.taskID
	} else {
		opts.TaskID = tgh.taskID
	}
	cedarTestResults, err := apimodels.GetCedarTestResults(ctx, opts)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"task_id": tgh.taskID,
			"message": "problem getting cedar test results",
		}))

		// No cedar test results, fall back to the database.
		tests, err = tgh.sc.FindTestsByTaskId(tgh.taskID, tgh.key, tgh.testName, tgh.testStatus, tgh.limit+1, tgh.testExecution)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
		} else if len(tests) == 0 {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    "not found",
				StatusCode: http.StatusNotFound,
			})
		}

		lastIndex := len(tests)
		if lastIndex > tgh.limit {
			key = string(tests[tgh.limit].ID)
			lastIndex = tgh.limit
		}
		// Truncate the hosts to just those that will be returned.
		tests = tests[:lastIndex]
	} else {
		startAt, err := strconv.Atoi(tgh.key)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.New("invalid start_at"))
		}

		var filteredCount int
		cedarTestResults, filteredCount = graphql.FilterSortAndPaginateCedarTestResults(
			cedarTestResults,
			tgh.testName,
			[]string{tgh.testStatus},
			"",
			1,
			startAt,
			tgh.limit,
		)

		if startAt*tgh.limit < filteredCount {
			key = string(startAt + 1)
		}
	}

	return tgh.buildResponse(cedarTestResults, tests, key)
}

func (tgh *testGetHandler) buildResponse(cedarTestResults []apimodels.CedarTestResult, testResults []testresult.TestResult, key string) gimlet.Responder {
	resp := gimlet.NewResponseBuilder()
	if err := resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error setting response format"))
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
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error paginating response"))
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
			Message:    errors.Wrap(err, "error building response").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}

	return nil
}
