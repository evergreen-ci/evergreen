package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
)

func makeFetchNotifcationStatusRoute(sc data.Connector) gimlet.RouteHandler {
	return &notificationsStatusHandler{sc: sc}
}

type notificationsStatusHandler struct{ sc data.Connector }

func (gh *notificationsStatusHandler) Factory() gimlet.RouteHandler {
	return &notificationsStatusHandler{sc: gh.sc}
}

func (gh *notificationsStatusHandler) Parse(_ context.Context, _ *http.Request) error { return nil }

func (gh *notificationsStatusHandler) Run(ctx context.Context) gimlet.Responder {
	stats, err := gh.sc.GetNotificationsStats()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(stats)
}
