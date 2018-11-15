package route

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

func makeFetchProjectEvents(sc data.Connector) gimlet.RouteHandler {
	return &projectEventsGet{sc: sc}
}

type projectEventsGet struct {
	Timestamp time.Time
	Limit     int
	Id        string
	sc        data.Connector
}

func (h *projectEventsGet) Factory() gimlet.RouteHandler {
	return &projectEventsGet{
		Timestamp: time.Now(),
		Limit:     10,
		Id:        "",
		sc:        h.sc,
	}
}

func (h *projectEventsGet) Parse(ctx context.Context, r *http.Request) error {
	var err error

	h.Id = gimlet.GetVars(r)["project_id"]

	vals := r.URL.Query()
	k, ok := vals["ts"]
	if ok && len(k) > 0 {
		h.Timestamp, err = time.Parse(time.RFC3339, k[0])
		if err != nil {
			return errors.Wrap(err, "problem parsing time as RFC-3339")
		}
	}

	h.Limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(err)
}

func (h *projectEventsGet) Run(ctx context.Context) gimlet.Responder {
	user := gimlet.GetUser(ctx)
	projectRef, err := model.FindOneProjectRef(h.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
	}
	if projectRef == nil {
		return gimlet.NewTextErrorResponse("project does not exist")
	}

	authorized := auth.IsSuperUser(h.sc.GetSuperUsers(), user) || util.StringSliceContains(projectRef.Admins, user.Username())
	if !authorized {
		return gimlet.NewTextErrorResponse("User not authorized")
	}
	events, err := h.sc.GetProjectEventLog(h.Id, h.Timestamp, h.Limit+1)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
	}

	resp := gimlet.NewResponseBuilder()

	lastIndex := len(events)
	if len(events) > h.Limit {
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
		catcher.Add(resp.AddData(restModel.Model(&events[i])))
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(catcher.Resolve())
	}

	if err = resp.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	return resp
}
