package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/patches/{patch_id}

type patchChangeStatusHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	patchId string
	sc      data.Connector
}

func makeChangePatchStatus(sc data.Connector) gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		sc: sc,
	}
}

func (p *patchChangeStatusHandler) Factory() gimlet.RouteHandler {
	return &patchChangeStatusHandler{
		sc: p.sc,
	}
}

func (p *patchChangeStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := util.ReadJSONInto(body, p); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	if p.Activated == nil && p.Priority == nil {
		return gimlet.ErrorResponse{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (p *patchChangeStatusHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	if p.Priority != nil {
		priority := *p.Priority
		if ok := validPriority(priority, user, p.sc); !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			})
		}
		if err := p.sc.SetPatchPriority(p.patchId, priority); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	if p.Activated != nil {
		if err := p.sc.SetPatchActivated(p.patchId, user.Username(), *p.Activated); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
		}
	}
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/patches/{patch_id}

type patchByIdHandler struct {
	patchId string
	sc      data.Connector
}

func makeFetchPatchByID(sc data.Connector) gimlet.RouteHandler {
	return &patchByIdHandler{
		sc: sc,
	}
}

func (p *patchByIdHandler) Factory() gimlet.RouteHandler {
	return &patchByIdHandler{sc: p.sc}
}

func (p *patchByIdHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchByIdHandler) Run(ctx context.Context) gimlet.Responder {
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/users/<id>/patches

type patchesByUserHandler struct {
	limit int
	key   time.Time
	user  string
	sc    data.Connector
}

func makeUserPatchHandler(sc data.Connector) gimlet.RouteHandler {
	return &patchesByUserHandler{
		sc: sc,
	}
}

func (p *patchesByUserHandler) Factory() gimlet.RouteHandler {
	return &patchesByUserHandler{
		sc: p.sc,
	}
}

func (p *patchesByUserHandler) Parse(ctx context.Context, r *http.Request) error {
	p.user = gimlet.GetVars(r)["user_id"]
	vals := r.URL.Query()

	var err error
	if vals.Get("start_at") == "" {
		p.key = time.Now()
	} else {
		p.key, err = model.ParseTime(vals.Get("start_at"))
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", p.key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *patchesByUserHandler) Run(ctx context.Context) gimlet.Responder {
	grip.Debug(message.Fields{
		"limit": p.limit,
		"user":  p.user,
		"key":   p.key,
		"op":    "patches for user",
	})

	// sortAsc set to false in order to display patches in desc chronological order
	patches, err := p.sc.FindPatchesByUser(p.user, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	if len(patches) == 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		})
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if len(patches) > p.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             patches[p.limit].CreateTime.Format(model.APITimeFormat),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
		patches = patches[:p.limit]
	}
	for _, model := range patches {
		err = resp.AddData(model)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem forming response data"))
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the patches for a project
//
//    /projects/{project_id}/patches

type patchesByProjectHandler struct {
	projectId string
	key       time.Time
	limit     int
	sc        data.Connector
}

func makePatchesByProjectRoute(sc data.Connector) gimlet.RouteHandler {
	return &patchesByProjectHandler{
		sc: sc,
	}
}

func (p *patchesByProjectHandler) Factory() gimlet.RouteHandler {
	return &patchesByProjectHandler{
		sc: p.sc,
	}
}

func (p *patchesByProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	p.projectId = gimlet.GetVars(r)["project_id"]

	vals := r.URL.Query()

	var err error
	if vals.Get("start_at") == "" {
		p.key = time.Now()
	} else {
		p.key, err = time.ParseInLocation(model.APITimeFormat, vals.Get("start_at"), time.FixedZone("", 0))
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", p.key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *patchesByProjectHandler) Run(ctx context.Context) gimlet.Responder {
	patches, err := p.sc.FindPatchesByProject(p.projectId, p.key, p.limit+1)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	if len(patches) == 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		})
	}

	resp := gimlet.NewResponseBuilder()
	err = resp.SetFormat(gimlet.JSON)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to set response format"))
	}
	if len(patches) > p.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             patches[p.limit].CreateTime.Format(model.APITimeFormat),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
		patches = patches[:p.limit]
	}
	for _, model := range patches {
		err = resp.AddData(model)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem forming response data"))
		}
	}
	err = resp.SetStatus(http.StatusOK)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "unable to set response status"))
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// Handler for aborting patches by id
//
//    /patches/{patch_id}/abort

type patchAbortHandler struct {
	patchId string
	sc      data.Connector
}

func makeAbortPatch(sc data.Connector) gimlet.RouteHandler {
	return &patchAbortHandler{
		sc: sc,
	}
}

func (p *patchAbortHandler) Factory() gimlet.RouteHandler {
	return &patchAbortHandler{sc: p.sc}
}

func (p *patchAbortHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchAbortHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)

	if err := p.sc.AbortPatch(p.patchId, usr.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Abort error"))
	}

	// Patch may be deleted by abort (eg not finalized) and not found here
	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for restarting patches by id
//
//    /patches/{patch_id}/restart

type patchRestartHandler struct {
	patchId string
	sc      data.Connector
}

func makeRestartPatch(sc data.Connector) gimlet.RouteHandler {
	return &patchRestartHandler{
		sc: sc,
	}
}

func (p *patchRestartHandler) Factory() gimlet.RouteHandler {
	return &patchRestartHandler{
		sc: p.sc,
	}
}

func (p *patchRestartHandler) Parse(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchRestartHandler) Run(ctx context.Context) gimlet.Responder {
	// If the version has not been finalized, returns NotFound
	usr := MustHaveUser(ctx)

	if err := p.sc.RestartVersion(p.patchId, usr.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Restart error"))
	}

	foundPatch, err := p.sc.FindPatchById(p.patchId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	return gimlet.NewJSONResponse(foundPatch)
}
