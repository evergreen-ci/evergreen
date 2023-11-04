package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/task"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
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
	key        int
	limit      int
	latest     bool

	task *task.Task
	env  evergreen.Environment
	sc   data.Connector
}

func makeFetchTestsForTask(env evergreen.Environment, sc data.Connector) gimlet.RouteHandler {
	return &testGetHandler{
		env: env,
		sc:  sc,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get tests from a task
//	@Description	Fetches a paginated list of tests that ran as part of the given task. To filter the tasks, add the following parameters into the query string.
//	@Tags			tests
//	@Router			/tasks/{task_id}/tests [get]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path	string	true	"task ID"
//	@Param			start_at	query	string	false	"The identifier of the test to start at in the pagination"
//	@Param			limit		query	int		false	"The number of tests to be returned per page of pagination. Defaults to 100"
//	@Param			status		query	string	false	"A status of test to limit the results to."
//	@Param			execution	query	int		false	"The 0-based number corresponding to the execution of the task. Defaults to 0, meaning the first time the task was run."
//	@Param			test_name	query	string	false	"Only return the test matching the name."
//	@Param			latest		query	bool	false	"Return tests from the latest execution. Cannot be used with execution."
//	@Success		200			{array}	model.APITest
func (hgh *testGetHandler) Factory() gimlet.RouteHandler {
	return &testGetHandler{
		env: hgh.env,
		sc:  hgh.sc,
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

		execution := 0 // Default to first execution unless latest is specified to maintain backwards compatibility.
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
	if startAt := vals.Get("start_at"); startAt != "" {
		tgh.key, err = strconv.Atoi(startAt)
		if err != nil {
			return errors.New("invalid 'start at' value")
		}
	}
	tgh.testName = vals.Get("test_name")
	tgh.limit, err = getLimit(vals)
	if err != nil {
		return errors.Wrap(err, "getting limit")
	}

	return nil
}

func (tgh *testGetHandler) Run(ctx context.Context) gimlet.Responder {
	results, err := tgh.task.GetTestResults(
		ctx,
		tgh.env,
		&testresult.FilterOptions{
			TestName: tgh.testName,
			Statuses: tgh.testStatus,
			Limit:    tgh.limit,
			Page:     tgh.key,
		},
	)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting test results"))
	}

	var nextKey string
	if tgh.key*tgh.limit < utility.FromIntPtr(results.Stats.FilteredCount) {
		nextKey = fmt.Sprintf("%d", tgh.key+1)
	}

	return tgh.buildResponse(results.Results, nextKey)
}

func (tgh *testGetHandler) buildResponse(results []testresult.TestResult, key string) gimlet.Responder {
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

	for i, result := range results {
		if err := tgh.addDataToResponse(resp, &result); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding test result at index %d", i))
		}
	}

	return resp
}

func (tgh *testGetHandler) addDataToResponse(resp gimlet.Responder, result interface{}) error {
	at := &model.APITest{}
	if err := at.BuildFromService(tgh.taskID); err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "adding task ID to task API model").Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}
	if err := at.BuildFromService(result); err != nil {
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
	execution int
}

func makeFetchTestCountForTask() gimlet.RouteHandler {
	return &testCountGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get the test count from a task
//	@Description	Returns an integer representing the number of tests that ran as part of the given task.
//	@Tags			tests
//	@Router			/tasks/{task_id}/tests/count [get]
//	@Security		Api-User || Api-Key
//	@Param			task_id		path		string	true	"task ID"
//	@Param			execution	query		int		false	"The 0-based number corresponding to the execution of the task. Defaults to 0, meaning the first time the task was run."
//	@Success		200			{string}	string
func (h *testCountGetHandler) Factory() gimlet.RouteHandler {
	return &testCountGetHandler{}
}

func (h *testCountGetHandler) Parse(ctx context.Context, r *http.Request) error {
	executionString := r.URL.Query().Get("execution")
	if executionString != "" {
		var err error
		h.execution, err = strconv.Atoi(executionString)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("invalid execution '%s'", executionString),
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	return nil
}

func (h *testCountGetHandler) Run(ctx context.Context) gimlet.Responder {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "task not found",
			StatusCode: http.StatusNotFound,
		})
	}
	t := projCtx.Task
	if t.Execution != h.execution {
		t, err := task.FindOneIdOldOrNew(t.Id, h.execution)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrapf(err, "finding task '%s' with execution %d", t.Id, h.execution).Error(),
			})
		}
		if t == nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task '%s' with execution %d not found", t.Id, h.execution),
			})
		}
	}

	stats, err := t.GetTestResultsStats(ctx, evergreen.GetEnvironment())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "getting stats for task '%s'", t.Id).Error(),
		})
	}

	return gimlet.NewTextResponse(stats.TotalCount)
}
