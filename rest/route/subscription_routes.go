package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

func getSubscriptionRouteManager(route string, version int) *RouteManager {
	h := &subscriptionPostHandler{}

	postHandler := MethodHandler{
		Authenticator:  &RequireUserAuthenticator{},
		RequestHandler: h.Handler(),
		MethodType:     http.MethodPost,
	}
	getHandler := MethodHandler{
		Authenticator:  &RequireUserAuthenticator{},
		RequestHandler: &subscriptionGetHandler{},
		MethodType:     http.MethodGet,
	}
	deleteHandler := MethodHandler{
		Authenticator:  &RequireUserAuthenticator{},
		RequestHandler: &subscriptionDeleteHandler{},
		MethodType:     http.MethodDelete,
	}

	routeManager := RouteManager{
		Route:   route,
		Methods: []MethodHandler{postHandler, getHandler, deleteHandler},
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
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Error parsing request body: " + err.Error(),
			}
		}

		dbSubscription, ok := subscriptionInterface.(event.Subscription)
		if !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Error parsing subscription interface",
			}
		}

		if dbSubscription.OwnerType == event.OwnerTypePerson && dbSubscription.Owner == "" {
			dbSubscription.Owner = u.Username() // default the current user
		}

		if dbSubscription.OwnerType == event.OwnerTypePerson && dbSubscription.Owner != u.Username() {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				Message:    "Cannot change subscriptions for anyone other than yourself",
			}
		}

		if ok, msg := isSubscriptionAllowed(dbSubscription); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    msg,
			}
		}

		if ok, msg := validateSelectors(dbSubscription.Subscriber, dbSubscription.Selectors); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid selectors: %s", msg),
			}
		}
		if ok, msg := validateSelectors(dbSubscription.Subscriber, dbSubscription.RegexSelectors); !ok {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid regex selectors: %s", msg),
			}
		}

		err = dbSubscription.Validate()
		if err != nil {
			return gimlet.ErrorResponse{
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

func validateSelectors(subscriber event.Subscriber, selectors []event.Selector) (bool, string) {
	for i := range selectors {
		if len(selectors[i].Type) == 0 || len(selectors[i].Data) == 0 {
			return false, "Selector had empty type or data"
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
	s.owner = r.FormValue("owner")
	s.ownerType = r.FormValue("type")
	if !event.IsValidOwnerType(s.ownerType) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid owner type",
		}
	}
	if s.owner == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Owner cannot be blank",
		}
	}
	if s.ownerType == string(event.OwnerTypePerson) && s.owner != u.Username() {
		return gimlet.ErrorResponse{
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

type subscriptionDeleteHandler struct {
	id string
}

func (s *subscriptionDeleteHandler) Handler() RequestHandler {
	return &subscriptionDeleteHandler{}
}

func (s *subscriptionDeleteHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	u := MustHaveUser(ctx)
	idString := r.FormValue("id")
	if idString == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Must specify an ID to delete",
		}
	}
	s.id = idString
	subscription, err := event.FindSubscriptionByIDString(s.id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}
	if subscription == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "Subscription not found",
		}
	}
	if subscription.Owner != u.Username() {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "Cannot delete subscriptions for someone other than yourself",
		}
	}

	return nil
}

func (s *subscriptionDeleteHandler) Execute(_ context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.DeleteSubscription(s.id)
	if err != nil {
		return ResponseData{}, err
	}

	return ResponseData{}, nil
}
