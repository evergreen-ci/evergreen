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

func makeFetchAdminEvents() gimlet.RouteHandler {
	return &adminEventsGet{}
}

type adminEventsGet struct {
	Timestamp time.Time
	Limit     int
	url       string
}

func (h *adminEventsGet) Factory() gimlet.RouteHandler {
	return &adminEventsGet{
		Timestamp: time.Now(),
		Limit:     10,
	}
}

func (h *adminEventsGet) Parse(ctx context.Context, r *http.Request) error {
	var err error
	vals := r.URL.Query()
	h.url = util.HttpsUrl(r.Host)

	k, ok := vals["ts"]
	if ok && len(k) > 0 {
		h.Timestamp, err = time.Parse(time.RFC3339, k[0])
		if err != nil {
			return errors.Wrap(err, "parsing time as RFC-3339")
		}
	}

	h.Limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(err)
}

func (h *adminEventsGet) Run(ctx context.Context) gimlet.Responder {
	resp := gimlet.NewResponseBuilder()

	events, err := data.GetAdminEventLog(ctx, h.Timestamp, h.Limit+1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting admin event log"))
	}

	lastIndex := len(events)
	if len(events) > h.Limit {
		lastIndex = h.Limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				BaseURL:         h.url,
				KeyQueryParam:   "ts",
				LimitQueryParam: "limit",
				Relation:        "next",
				Key:             events[h.Limit-1].Timestamp.Format(time.RFC3339),
				Limit:           h.Limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	events = events[:lastIndex]
	catcher := grip.NewBasicCatcher()
	for i := range events {
		catcher.Wrapf(resp.AddData(&events[i]), "adding data for event at index %d", i)
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(catcher.Resolve())
	}
	return resp
}
