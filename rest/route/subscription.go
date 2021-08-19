package route

import (
	"context"
	"fmt"
	"net/http"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/subscriptions

type subscriptionPostHandler struct {
	Subscriptions *[]model.APISubscription `json:"subscriptions"`
	sc            data.Connector
}

func makeSetSubscription(sc data.Connector) gimlet.RouteHandler {
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
	s.Subscriptions = &[]model.APISubscription{}
	if err := utility.ReadJSON(r.Body, s.Subscriptions); err != nil {
		return err
	}

	return nil
}

func (s *subscriptionPostHandler) Run(ctx context.Context) gimlet.Responder {
	err := s.sc.SaveSubscriptions(MustHaveUser(ctx).Username(), *s.Subscriptions, false)
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
	if s.ownerType == string(event.OwnerTypeProject) {
		id, err := dbModel.GetIdForProject(s.owner)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "owner not found",
			}
		}
		s.owner = id
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
	idString := r.FormValue("id")
	if idString == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Must specify an ID to delete",
		}
	}
	s.id = idString

	return nil
}

func (s *subscriptionDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	if err := s.sc.DeleteSubscriptions(MustHaveUser(ctx).Username(), []string{s.id}); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}
