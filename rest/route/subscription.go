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

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/subscriptions

type subscriptionPostHandler struct {
	Subscriptions   *[]model.APISubscription `json:"subscriptions"`
	dbSubscriptions []event.Subscription
	sc              data.Connector
}

func makeSetSubscrition(sc data.Connector) gimlet.RouteHandler {
	return &subscriptionPostHandler{
		sc: sc,
	}
}

func (s *subscriptionPostHandler) Factory() gimlet.RouteHandler {
	return &subscriptionPostHandler{
		sc: s.sc,
	}
}

func (s *subscriptionPostHandler) Parse(ctx context.Context, r *http.Request) error {
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

func (s *subscriptionPostHandler) Run(ctx context.Context) gimlet.Responder {
	err := s.sc.SaveSubscriptions(s.dbSubscriptions)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/subscriptions

type subscriptionGetHandler struct {
	owner     string
	ownerType string
	sc        data.Connector
}

func makeFetchSubscription(sc data.Connector) gimlet.RouteHandler {
	return &subscriptionGetHandler{
		sc: sc,
	}
}

func (s *subscriptionGetHandler) Factory() gimlet.RouteHandler {
	return &subscriptionGetHandler{
		sc: s.sc,
	}
}

func (s *subscriptionGetHandler) Parse(ctx context.Context, r *http.Request) error {
	u := MustHaveUser(ctx)
	s.owner = r.FormValue("owner")
	s.ownerType = r.FormValue("type")
	if !event.IsValidOwnerType(s.ownerType) {
		fmt.Println(s.ownerType)
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

func (s *subscriptionGetHandler) Run(ctx context.Context) gimlet.Responder {
	subs, err := s.sc.GetSubscriptions(s.owner, event.OwnerType(s.ownerType))
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(subs)
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/subscriptions

type subscriptionDeleteHandler struct {
	id string
	sc data.Connector
}

func makeDeleteSubscription(sc data.Connector) gimlet.RouteHandler {
	return &subscriptionDeleteHandler{
		sc: sc,
	}
}

func (s *subscriptionDeleteHandler) Factory() gimlet.RouteHandler {
	return &subscriptionDeleteHandler{sc: s.sc}
}

func (s *subscriptionDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	u := MustHaveUser(ctx)
	idString := r.FormValue("id")
	if idString == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Must specify an ID to delete",
		}
	}
	s.id = idString
	subscription, err := event.FindSubscriptionByID(s.id)
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

func (s *subscriptionDeleteHandler) Run(_ context.Context) gimlet.Responder {
	err := s.sc.DeleteSubscription(s.id)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
