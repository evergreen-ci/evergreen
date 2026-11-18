package route

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeFetchNotifcationStatusRoute() gimlet.RouteHandler {
	return &notificationsStatusHandler{}
}

type notificationsStatusHandler struct{}

func (gh *notificationsStatusHandler) Factory() gimlet.RouteHandler {
	return &notificationsStatusHandler{}
}

func (gh *notificationsStatusHandler) Parse(_ context.Context, _ *http.Request) error { return nil }

func (gh *notificationsStatusHandler) Run(ctx context.Context) gimlet.Responder {
	stats, err := data.GetNotificationsStats(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting notification stats"))
	}

	return gimlet.NewJSONResponse(stats)
}

// maxRetryFailedWebhooksBodySize caps the retry request body, which only holds
// the dry-run option.
const maxRetryFailedWebhooksBodySize = 1024

func makeRetryFailedWebhooksRoute(env evergreen.Environment) gimlet.RouteHandler {
	return &retryFailedWebhooksHandler{env: env}
}

type retryFailedWebhooksHandler struct {
	env  evergreen.Environment
	opts retryFailedWebhooksOptions
}

type retryFailedWebhooksOptions struct {
	// DryRun lists the failed deliveries that would be retried without retrying them.
	DryRun bool `json:"dry_run"`
}

// Factory creates an instance of the handler.
//
//	@Summary		Retry failed webhook notifications
//	@Description	Retry webhook notifications that failed to deliver for a project. Set dry_run to list the failed deliveries that would be retried without retrying them.
//	@Tags			notifications
//	@Router			/projects/{project_id}/retry_failed_webhooks [post]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path	string						true	"the project ID or identifier"
//	@Param			{object}	body	retryFailedWebhooksOptions	false	"parameters"
//	@Success		200			{array}	model.APIFailedWebhook
func (h *retryFailedWebhooksHandler) Factory() gimlet.RouteHandler {
	return &retryFailedWebhooksHandler{env: h.env}
}

// Parse reads the dry-run option from the request body. An empty body uses the
// defaults.
func (h *retryFailedWebhooksHandler) Parse(_ context.Context, r *http.Request) error {
	if r.Body == nil {
		return nil
	}
	data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxRetryFailedWebhooksBodySize))
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "reading request body").Error(),
		}
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &h.opts); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	return nil
}

// Run returns the failed webhooks for the project, retrying them unless the request is a dry run.
func (h *retryFailedWebhooksHandler) Run(ctx context.Context) gimlet.Responder {
	pRef := MustHaveProjectContext(ctx).ProjectRef
	if pRef == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "project not found",
		})
	}

	failedWebhooks, err := data.RetryFailedWebhooks(ctx, h.env, pRef, h.opts.DryRun)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return gimlet.NewJSONResponse(failedWebhooks)
}
