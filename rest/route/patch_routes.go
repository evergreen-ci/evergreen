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

	patchModel := &model.APIPatch{}
	if err = patchModel.BuildFromService(*foundPatch); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}
	return gimlet.NewJSONResponse(patchModel)
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

	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(patchModel)
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
	p.key, err = time.ParseInLocation(model.APITimeFormat, vals.Get("start_at"), time.FixedZone("", 0))
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", p.key, err.Error()),
			StatusCode: http.StatusBadRequest,
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

	lastIndex := len(patches)
	if len(patches) > p.limit {
		lastIndex = p.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             patches[p.limit].Id.String(),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	patches = patches[:lastIndex]

	for _, info := range patches {
		patchModel := &model.APIPatch{}
		if err = patchModel.BuildFromService(info); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			})
		}

		if err = resp.AddData(info); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
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
	p.key, err = time.ParseInLocation(model.APITimeFormat, vals.Get("start_at"), time.FixedZone("", 0))
	if err != nil {
		return errors.WithStack(err)
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
		return gimlet.NewJSONResponse(errors.Wrap(err, "Database error"))
	}

	if len(patches) == 0 {
		return gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		})
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(patches)
	if len(patches) > p.limit {
		lastIndex = p.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             model.NewTime(patches[p.limit].CreateTime).String(),
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	patches = patches[:lastIndex]

	for _, info := range patches {
		patchModel := &model.APIPatch{}
		if err = patchModel.BuildFromService(info); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			})
		}

		if err = resp.AddData(patchModel); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// Handler for aborting patches by id
//
//    /patches/{patch_id}/abort

func getPatchAbortManager(route string, version int) *RouteManager {
	p := &patchAbortHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

type patchAbortHandler struct {
	patchId string
}

func (p *patchAbortHandler) Handler() RequestHandler {
	return &patchAbortHandler{}
}

func (p *patchAbortHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	usr := MustHaveUser(ctx)
	err := sc.AbortPatch(p.patchId, usr.Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Abort error")
	}

	// Patch may be deleted by abort (eg not finalized) and not found here
	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}
	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)

	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for restarting patches by id
//
//    /patches/{patch_id}/restart

func getPatchRestartManager(route string, version int) *RouteManager {
	p := &patchRestartHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

type patchRestartHandler struct {
	patchId string
}

func (p *patchRestartHandler) Handler() RequestHandler {
	return &patchRestartHandler{}
}

func (p *patchRestartHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.patchId = gimlet.GetVars(r)["patch_id"]
	return nil
}

func (p *patchRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {

	// If the version has not been finalized, returns NotFound
	usr := MustHaveUser(ctx)
	err := sc.RestartVersion(p.patchId, usr.Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Restart error")
	}

	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}
	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)

	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}
