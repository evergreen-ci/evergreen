package route

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
//	@Description	Return a subset of tests to run for a given task.
//	@Tags			select
//	@Router			/select/tests [post]
//	@Param			{object}	body	model.SelectTestsRequest	true	"Select tests request"
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.SelectTestsRequest
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
	catcher.NewWhen(t.selectTests.Project == "", "project ID is required")
	catcher.NewWhen(t.selectTests.Requester == "", "requester is required")
	catcher.NewWhen(t.selectTests.BuildVariant == "", "build variant is required")
	catcher.NewWhen(t.selectTests.TaskID == "", "task ID is required")
	catcher.NewWhen(t.selectTests.TaskName == "", "task name is required")
	return catcher.Resolve()
}

func (t *selectTestsHandler) Run(ctx context.Context) gimlet.Responder {
	selectedTests, err := data.SelectTests(ctx, t.selectTests)
	if err != nil {
		return makeSelectTestsErrorResponse(err)
	}

	// The quarantined-tests snapshot is best effort and shouldn't fail test selection
	startAt := time.Now()
	if err := data.RecordQuarantinedTestsSkipped(ctx, t.env, t.selectTests, selectedTests); err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":       "error recording quarantined tests skipped by test selection",
			"project_id":    t.selectTests.Project,
			"requester":     t.selectTests.Requester,
			"build_variant": t.selectTests.BuildVariant,
			"task_id":       t.selectTests.TaskID,
			"task_name":     t.selectTests.TaskName,
			"duration_ms":   time.Since(startAt).Milliseconds(),
		}))
	}

	rhResp := t.selectTests
	rhResp.Tests = selectedTests
	return gimlet.NewJSONResponse(rhResp)
}

func makeSelectTestsErrorResponse(err error) gimlet.Responder {
	if errors.Is(err, context.DeadlineExceeded) {
		// The agent retries other server and timeout statuses, which would
		// reissue the TSS request and amplify an outage.
		return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
			StatusCode: http.StatusFailedDependency,
			Message:    err.Error(),
		})
	}
	return gimlet.NewJSONInternalErrorResponse(err)
}
