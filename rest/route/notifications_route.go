package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func makeFetchNotifcationStatusRoute() gimlet.RouteHandler {
	return &notificationsStatusHandler{}
}

type notificationsStatusHandler struct{}

// Factory creates an instance of the handler.
//
//	@Summary		Get notification status
//	@Description	Get statistics on unprocessed events and pending notifications.
//	@Tags			notifications
//	@Router			/status/notifications [get]
//	@Security		Api-User || Api-Key
//	@Success		200	{object}	model.APIEventStats
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
