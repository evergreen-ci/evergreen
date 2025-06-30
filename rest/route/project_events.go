package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func makeFetchProjectEvents() gimlet.RouteHandler {
	return &projectEventsGet{}
}

type projectEventsGet struct {
	Timestamp time.Time
	Limit     int
	Id        string
	Url       string
}

func (h *projectEventsGet) Factory() gimlet.RouteHandler {
	return &projectEventsGet{
		Timestamp: time.Now(),
		Limit:     10,
		Id:        "",
	}
}

func (h *projectEventsGet) Parse(ctx context.Context, r *http.Request) error {
	var err error

	h.Id = gimlet.GetVars(r)["project_id"]

	vals := r.URL.Query()
	h.Url = util.HttpsUrl(r.Host)

	k, ok := vals["ts"]
	if ok && len(k) > 0 {
		h.Timestamp, err = time.Parse(time.RFC3339, k[0])
		if err != nil {
			return errors.Wrap(err, "parsing time in RFC3339 format")
		}
	}

	h.Limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(err)
}

func (h *projectEventsGet) Run(ctx context.Context) gimlet.Responder {
	events, err := data.GetProjectEventLog(ctx, h.Id, h.Timestamp, h.Limit+1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting event log for project '%s'", h.Id))
	}

	resp := gimlet.NewResponseBuilder()

	lastIndex := len(events)
	if len(events) > h.Limit {
		lastIndex = h.Limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				BaseURL:         h.Url,
				KeyQueryParam:   "ts",
				LimitQueryParam: "limit",
				Relation:        "next",
				Key:             events[h.Limit-1].Timestamp.Format(time.RFC3339),
				Limit:           h.Limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"paginating response"))
		}
	}

	events = events[:lastIndex]
	catcher := grip.NewBasicCatcher()
	for i := range events {
		catcher.Wrapf(resp.AddData(&events[i]), "adding response data for event at index %d", i)
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(catcher.Resolve())
	}
	return resp
}
