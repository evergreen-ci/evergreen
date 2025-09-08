package route

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type selectTestsHandler struct {
	selectTests model.SelectTestsRequest
	env         evergreen.Environment
}

func makeSelectTestsHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &selectTestsHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Select tests
//	@Description	Return a subset of tests to run for a given task. This endpoint is not yet ready. Please do not use it.
//	@Tags			select
//	@Router			/select/tests [post]
//	@Param			{object}	body	model.SelectTestsRequest	true	"Select tests request"
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	SelectTestsRequest
func (t *selectTestsHandler) Factory() gimlet.RouteHandler {
	return &selectTestsHandler{env: t.env}
}

func (t *selectTestsHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	if err = json.Unmarshal(b, &t.selectTests); err != nil {
		return errors.Wrap(err, "parsing request body")
	}
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(t.selectTests.Project == "", "project is required")
	catcher.NewWhen(t.selectTests.Requester == "", "requester is required")
	catcher.NewWhen(t.selectTests.BuildVariant == "", "build variant is required")
	catcher.NewWhen(t.selectTests.TaskID == "", "task ID is required")
	catcher.NewWhen(t.selectTests.TaskName == "", "task name is required")
	catcher.NewWhen(len(t.selectTests.Tests) == 0, "tests array must not be empty")
	return catcher.Resolve()
}

func (t *selectTestsHandler) Run(ctx context.Context) gimlet.Responder {
	selectedTests, err := data.SelectTests(ctx, t.selectTests)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}
	rhResp := t.selectTests
	rhResp.Tests = selectedTests
	return gimlet.NewJSONResponse(rhResp)
}
