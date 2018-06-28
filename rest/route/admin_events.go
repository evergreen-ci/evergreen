package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

func getAdminEventRouteManager(route string, version int) *RouteManager {
	aeg := &adminEventsGet{}
	getHandler := MethodHandler{
		Authenticator:  &SuperUserAuthenticator{},
		RequestHandler: aeg.Handler(),
		MethodType:     http.MethodGet,
	}
	return &RouteManager{
		Route:   route,
		Methods: []MethodHandler{getHandler},
		Version: version,
	}
}

type adminEventsGet struct {
	*PaginationExecutor
}

func (h *adminEventsGet) Handler() RequestHandler {
	paginator := &PaginationExecutor{
		KeyQueryParam:   "ts",
		LimitQueryParam: "limit",
		Paginator:       adminEventPaginator,
	}
	return &adminEventsGet{paginator}
}

func adminEventPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	var ts time.Time
	var err error
	if key == "" {
		ts = time.Now()
	} else {
		ts, err = time.Parse(time.RFC3339, key)
		if err != nil {
			return []model.Model{}, nil, errors.Wrapf(err, "unable to parse '%s' in RFC-3339 format")
		}
	}
	if limit == 0 {
		limit = 10
	}

	events, err := sc.GetAdminEventLog(ts, limit)
	if err != nil {
		return []model.Model{}, nil, err
	}
	nextPage := makeNextEventsPage(events, limit)
	pageResults := &PageResult{
		Next: nextPage,
	}

	lastIndex := len(events)
	if nextPage != nil {
		lastIndex = limit
	}
	events = events[:lastIndex]
	results := make([]model.Model, len(events))
	for i := range events {
		results[i] = model.Model(&events[i])
	}

	return results, pageResults, nil
}

func makeNextEventsPage(events []model.APIAdminEvent, limit int) *Page {
	var nextPage *Page
	if len(events) == limit {
		nextPage = &Page{
			Relation: "next",
			Key:      events[limit-1].Timestamp.Format(time.RFC3339),
			Limit:    limit,
		}
	}
	return nextPage
}

func makeRevertRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &revertHandler{
		sc: sc,
	}
}

type revertHandler struct {
	GUID string `json:"guid"`

	sc data.Connector
}

func (h *revertHandler) Factory() gimlet.RouteHandler { return &revertHandler{sc: h.sc} }

func (h *revertHandler) Parse(ctx context.Context, r *http.Request) (context.Context, error) {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return ctx, errors.WithStack(err)
	}

	if h.GUID == "" {
		return ctx, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "GUID to revert to must be specified",
		}
	}
	return ctx, nil
}

func (h *revertHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	err := h.sc.RevertConfigTo(h.GUID, u.Username())
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(struct{}{})
}
