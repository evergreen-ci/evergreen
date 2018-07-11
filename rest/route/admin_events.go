package route

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func makeFetchAdminEvents(sc data.Connector) gimlet.RouteHandler {
	return &adminEventsGet{sc: sc}
}

type adminEventsGet struct {
	Timestamp time.Time
	Limit     int

	sc data.Connector
}

func (h *adminEventsGet) Factory() gimlet.RouteHandler {
	return &adminEventsGet{
		Timestamp: time.Now(),
		Limit:     10,
		sc:        h.sc,
	}
}

func (h *adminEventsGet) Parse(ctx context.Context, r *http.Request) error {
	var err error
	vals := r.URL.Query()

	k, ok := vals["ts"]
	if ok && len(k) > 0 {
		h.Timestamp, err = time.Parse(time.RFC3339, k[0])
		if err != nil {
			return errors.Wrap(err, "problem parsing time as RFC-3339")
		}
	}

	if l, ok := vals["limit"]; ok && len(l) > 0 {
		h.Limit, err = strconv.Atoi(l[0])
		if err != nil {
			return errors.Wrap(err, "problem parsing limit")
		}
	}

	// if user asks for a limit of 0 we should override.
	if h.Limit == 0 {
		h.Limit = 10
	}

	return errors.WithStack(err)
}

func (h *adminEventsGet) Run(ctx context.Context) gimlet.Responder {
	resp := gimlet.NewResponseBuilder()

	events, err := h.sc.GetAdminEventLog(h.Timestamp, h.Limit)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "database error"))
	}

	lastIndex := len(events)

	if len(events) == h.Limit {
		lastIndex = h.Limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				BaseURL:         h.sc.GetURL(),
				KeyQueryParam:   "ts",
				LimitQueryParam: "limit",
				Relation:        "next",
				Key:             events[h.Limit-1].Timestamp.Format(time.RFC3339),
				Limit:           h.Limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	events = events[:lastIndex]
	catcher := grip.NewBasicCatcher()
	for i := range events {
		catcher.Add(resp.AddData(model.Model(&events[i])))

	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(catcher.Resolve())
	}

	if err = resp.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return resp
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

func (h *revertHandler) Parse(ctx context.Context, r *http.Request) error {
	if err := gimlet.GetJSON(r.Body, h); err != nil {
		return errors.WithStack(err)
	}

	if h.GUID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "GUID to revert to must be specified",
		}
	}
	return nil
}

func (h *revertHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	err := h.sc.RevertConfigTo(h.GUID, u.Username())
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(struct{}{})
}
