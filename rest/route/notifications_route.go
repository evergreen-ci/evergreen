package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
)

func getNotificationsStatusRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &notificationsStatusHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

type notificationsStatusHandler struct{}

func (gh *notificationsStatusHandler) Handler() RequestHandler {
	return &notificationsStatusHandler{}
}

func (gh *notificationsStatusHandler) ParseAndValidate(_ context.Context, _ *http.Request) error {
	return nil
}

func (gh *notificationsStatusHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	stats, err := sc.GetNotificationsStats(ctx, nil)
	if err != nil {
		return ResponseData{}, err
	}

	return ResponseData{Result: []model.Model{stats}}, nil
}
