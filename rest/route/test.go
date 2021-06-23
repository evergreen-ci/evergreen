package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/graphql"
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
	displayTask   bool
	testStatus    string
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
	if len(projCtx.Task.ExecutionTasks) > 0 {
		tgh.displayTask = true
	}

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
	tgh.testID = vals.Get("test_id")
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
	opts := apimodels.GetCedarTestResultsOptions{
		BaseURL:   evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		Execution: tgh.testExecution,
		// When testID is populated, that means the user wants to fetch
		// a single test by the unique test name stored in cedar.
		TestName: tgh.testID,
	}
	if tgh.displayTask {
		opts.DisplayTaskID = tgh.taskID
	} else {
		opts.TaskID = tgh.taskID
	}
	cedarTestResults, err := apimodels.GetCedarTestResults(ctx, opts)
	testStatuses := []string{}
	if tgh.testStatus != "" {
		testStatuses = append(testStatuses, tgh.testStatus)
	}
	if err == nil && tgh.testID == "" {
		startAt := 0
		if tgh.key != "" {
			startAt, err = strconv.Atoi(tgh.key)
			if err != nil {
				return gimlet.MakeJSONErrorResponder(errors.New("invalid start_at"))
			}
		}
		var filteredCount int
		cedarTestResults, filteredCount = graphql.FilterSortAndPaginateCedarTestResults(graphql.FilterSortAndPaginateCedarTestResultsOpts{
			Limit:       tgh.limit,
			Page:        startAt,
			SortDir:     1,
			Statuses:    testStatuses,
			TestName:    tgh.testName,
			TestResults: cedarTestResults,
		})

		if startAt*tgh.limit < filteredCount {
			key = fmt.Sprintf("%d", startAt+1)
		}
	} else if err != nil {
		// No cedar test results, fall back to the database.
		grip.Warning(message.WrapError(err, message.Fields{
			"task_id": tgh.taskID,
			"message": "problem getting cedar test results",
		}))

		if tgh.testID != "" {
			// When testID is populated, search the test results
			// collection for the given id.
			tests, err = tgh.sc.FindTestById(tgh.testID)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
			}
		} else {
			tests, err = tgh.sc.FindTestsByTaskIdFilterSortPaginate(data.FindTestsByTaskIdFilterSortPaginateOpts{
				Execution: tgh.testExecution,
				Limit:     tgh.limit + 1,
				Statuses:  testStatuses,
				TaskID:    tgh.taskID,
				TestID:    tgh.key,
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
