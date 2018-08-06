package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// getTestRouteManager gets the route manager for the GET /tasks/{task_id}/tests.
func getTestRouteManager(route string, version int) *RouteManager {
	tgh := &testGetHandler{}
	testGetMethodHandler := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchProjectContext},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    tgh.Handler(),
		MethodType:        http.MethodGet,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{testGetMethodHandler},
		Version: version,
	}
	return &taskRoute
}

// testGetHandlerArgs are the additional arguments that are needed when fetching
// paginated results for the tests.
type testGetHandlerArgs struct {
	taskId        string
	testStatus    string
	testExecution int
}

// testGetHandler is the MethodHandler for the GET /tasks/{task_id}/tests route.
type testGetHandler struct {
	*yPaginationExecutor
}

func (hgh *testGetHandler) Handler() RequestHandler {
	testPaginationExecutor := &PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       testPaginator,
		Args:            testGetHandlerArgs{},
	}

	return &testGetHandler{testPaginationExecutor}
}

// ParseAndValidate fetches the task Id and 'status' from the url and
// sets them as part of the args.
func (tgh *testGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	projCtx := MustHaveProjectContext(ctx)
	if projCtx.Task == nil {
		return gimlet.ErrorResponse{
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	var executionInt int
	var err error
	execution := r.URL.Query().Get("execution")
	if execution != "" {
		executionInt, err = strconv.Atoi(execution)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "Invalid execution",
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	tgh.Args = testGetHandlerArgs{
		taskId:        projCtx.Task.Id,
		testStatus:    r.URL.Query().Get("status"),
		testExecution: executionInt,
	}
	return tgh.PaginationExecutor.ParseAndValidate(ctx, r)
}

// testPaginator is the PaginatorFunc that implements the functionality of paginating
// over the tests results of a task. It executes the database lookup and creates
// the pages for pagination.
func testPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	tghArgs, ok := args.(testGetHandlerArgs)
	if !ok {
		grip.EmergencyPanic("Test pagination args had wrong type")
	}
	tests, err := sc.FindTestsByTaskId(tghArgs.taskId, key, tghArgs.testStatus, limit*2, 1, tghArgs.testExecution)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}

	// Make the previous page
	prevTests, err := sc.FindTestsByTaskId(tghArgs.taskId, key, tghArgs.testStatus, limit, -1, tghArgs.testExecution)
	if err != nil && tests == nil { // don't error if we already found valid results
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}

	nextPage := makeNextTestsPage(tests, limit)
	prevPage := makePrevTestsPage(prevTests)

	pageResults := &PageResult{
		Next: nextPage,
		Prev: prevPage,
	}

	lastIndex := len(tests)
	if nextPage != nil {
		lastIndex = limit
	}

	// Truncate the hosts to just those that will be returned.
	tests = tests[:lastIndex]

	models := make([]model.Model, len(tests))
	for ix, testResult := range tests {
		at := &model.APITest{}
		err = at.BuildFromService(tghArgs.taskId)
		if err != nil {
			return []model.Model{}, nil, errors.Wrap(err, "Model error")
		}
		err = at.BuildFromService(&testResult)
		if err != nil {
			return []model.Model{}, nil, errors.Wrap(err, "Model error")
		}
		models[ix] = at
	}
	return models, pageResults, nil
}

func makeNextTestsPage(tests []testresult.TestResult, limit int) *Page {
	var nextPage *Page
	if len(tests) > limit {
		nextLimit := len(tests) - limit
		nextPage = &Page{
			Relation: "next",
			Key:      string(tests[limit].ID),
			Limit:    nextLimit,
		}
	}
	return nextPage
}

func makePrevTestsPage(tests []testresult.TestResult) *Page {
	var prevPage *Page
	if len(tests) > 1 {
		prevPage = &Page{
			Relation: "prev",
			Key:      string(tests[0].ID),
			Limit:    len(tests),
		}
	}
	return prevPage
}
