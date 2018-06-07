package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
)

func getSubscriptionRouteManager(route string, version int) *RouteManager {
	h := &subscriptionPostHandler{}

	postHandler := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    h.Handler(),
		MethodType:        http.MethodPost,
	}
	getHandler := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    &subscriptionGetHandler{},
		MethodType:        http.MethodGet,
	}

	routeManager := RouteManager{
		Route:   route,
		Methods: []MethodHandler{postHandler, getHandler},
		Version: version,
	}
	return &routeManager
}

type subscriptionPostHandler struct {
	Subscriptions   *[]model.APISubscription `json:"subscriptions"`
	dbSubscriptions []event.Subscription
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
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Error parsing request body: " + err.Error(),
			}
		}

		dbSubscription, ok := subscriptionInterface.(event.Subscription)
		if !ok {
			return &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    "Error parsing subscription interface",
			}
		}

		if dbSubscription.OwnerType == event.OwnerTypePerson && dbSubscription.Owner == "" {
			dbSubscription.Owner = u.Username() // default the current user
		}

		if dbSubscription.OwnerType == event.OwnerTypePerson && dbSubscription.Owner != u.Username() {
			return &rest.APIError{
				StatusCode: http.StatusUnauthorized,
				Message:    "Cannot change subscriptions for anyone other than yourself",
			}
		}

		if ok, msg := isSubscriptionAllowed(dbSubscription); !ok {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    msg,
			}
		}

		err = dbSubscription.Validate()
		if err != nil {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "Error validating subscription: " + err.Error(),
			}
		}

		s.dbSubscriptions = append(s.dbSubscriptions, dbSubscription)
	}

	return nil
}

func isSubscriptionAllowed(sub event.Subscription) (bool, string) {
	for _, selector := range sub.Selectors {

		if selector.Type == "object" {
			if selector.Data == "build" || selector.Data == "version" || selector.Data == "task" {
				if sub.Subscriber.Type == "jira-issue" || sub.Subscriber.Type == "evergreen-webhook" {
					return false, fmt.Sprintf("Cannot notify by %s for %s", sub.Subscriber.Type, selector.Data)
				}
			}
		}

	}

	return true, ""
}

func (s *subscriptionPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.SaveSubscriptions(s.dbSubscriptions)
	if err != nil {
		return ResponseData{}, err
	}

	return ResponseData{}, nil
}

type subscriptionGetHandler struct {
	owner     string
	ownerType string
}

func (s *subscriptionGetHandler) Handler() RequestHandler {
	return &subscriptionGetHandler{}
}

func (s *subscriptionGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	u := MustHaveUser(ctx)
	vars := mux.Vars(r)
	s.owner = vars["owner"]
	s.ownerType = vars["type"]
	if !event.IsValidOwnerType(s.ownerType) {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid owner type",
		}
	}
	if s.owner == "" {
		return rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "Owner cannot be blank",
		}
	}
	if s.ownerType == string(event.OwnerTypePerson) && s.owner != u.Username() {
		return rest.APIError{
			StatusCode: http.StatusUnauthorized,
			Message:    "Cannot get subscriptions for someone other than yourself",
		}
	}

	return nil
}

func (s *subscriptionGetHandler) Execute(_ context.Context, sc data.Connector) (ResponseData, error) {
	subs, err := sc.GetSubscriptions(s.owner, event.OwnerType(s.ownerType))
	if err != nil {
		return ResponseData{}, err
	}

	model := make([]model.Model, len(subs))
	for i := range subs {
		model[i] = &subs[i]
	}

	return ResponseData{
		Result: model,
	}, nil
}
