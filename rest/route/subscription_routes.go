package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
)

type subscriptionPostHandler struct {
	Subscriptions   *[]model.APISubscription `json:"subscriptions"`
	dbSubscriptions []event.Subscription
}

func getSubscriptionRouteManager(route string, version int) *RouteManager {
	h := &subscriptionPostHandler{}

	handler := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    h.Handler(),
		MethodType:        http.MethodPost,
	}

	routeManager := RouteManager{
		Route:   route,
		Methods: []MethodHandler{handler},
		Version: version,
	}
	return &routeManager
}

func (s *subscriptionPostHandler) Handler() RequestHandler {
	return &subscriptionPostHandler{}
}

func (s *subscriptionPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	u := MustHaveUser(ctx)
	s.Subscriptions = &[]model.APISubscription{}
	s.dbSubscriptions = []event.Subscription{}
	if err := util.ReadJSONInto(r.Body, s.Subscriptions); err != nil {
		return err
	}
	for _, subscription := range *s.Subscriptions {
		subscriptionInterface, err := subscription.ToService()
		if err != nil {
			return rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Error parsing request body: " + err.Error(),
			}
		}

		dbSubscription, ok := subscriptionInterface.(event.Subscription)
		if !ok {
			return rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Error parsing subscription interface",
			}
		}

		if dbSubscription.Owner != u.Username() {
			return rest.APIError{
				StatusCode: http.StatusUnauthorized,
				Message:    "Cannot change subscriptions for anyone other than yourself",
			}
		}

		err = dbSubscription.Validate()
		if err != nil {
			return rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Error validating subscription: " + err.Error(),
			}
		}

		s.dbSubscriptions = append(s.dbSubscriptions, dbSubscription)
	}

	return nil
}

func (s *subscriptionPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.SaveSubscriptions(s.dbSubscriptions)
	if err != nil {
		return ResponseData{}, err
	}

	return ResponseData{}, nil
}
