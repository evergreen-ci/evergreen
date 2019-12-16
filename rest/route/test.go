package route

import (
	"context"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// testGetHandler is the MethodHandler for the GET /tasks/{task_id}/tests route.
type testGetHandler struct {
	taskId        string
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
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	tgh.taskId = projCtx.Task.Id

	var err error
	vals := r.URL.Query()
	execution := vals.Get("execution")

	if execution != "" {
		tgh.testExecution, err = strconv.Atoi(execution)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "Invalid execution",
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
	tests, err := tgh.sc.FindTestsByTaskId(tgh.taskId, tgh.key, tgh.testName, tgh.testStatus, tgh.limit+1, tgh.testExecution)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(tests)
	if len(tests) > tgh.limit {
		lastIndex = tgh.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tgh.sc.GetURL(),
				Key:             string(tests[tgh.limit].ID),
				Limit:           tgh.limit,
			},
		})

		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	// Truncate the hosts to just those that will be returned.
	tests = tests[:lastIndex]

	for _, testResult := range tests {
		at := &model.APITest{}
		err = at.BuildFromService(tgh.taskId)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Model error"))
		}

		err = at.BuildFromService(&testResult)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Model error"))
		}

		if err = resp.AddData(at); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
