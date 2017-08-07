package route

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

////////////////////////////////////////////////////////////////////////
//
// Handler for fetching patches by id and changing patch status
//
//    /patches/{patch_id}

func getPatchByIdManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     evergreen.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &patchByIdHandler{},
			},
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        evergreen.MethodPatch,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &patchChangeStatusHandler{},
			},
		},
	}
}

type patchChangeStatusHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	patchId string
}

func (p *patchChangeStatusHandler) Handler() RequestHandler {
	return &patchChangeStatusHandler{}
}

func (p *patchChangeStatusHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.patchId = mux.Vars(r)["patch_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := util.ReadJSONInto(body, p); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	if p.Activated == nil && p.Priority == nil {
		return &rest.APIError{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (p *patchChangeStatusHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	user := GetUser(ctx)
	if p.Priority != nil {
		priority := *p.Priority
		if ok := validPriority(priority, user, sc); !ok {
			return ResponseData{}, &rest.APIError{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			}
		}
		if err := sc.SetPatchPriority(p.patchId, priority); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	if p.Activated != nil {
		if err := sc.SetPatchActivated(p.patchId, user.Username(), *p.Activated); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}

type patchByIdHandler struct {
	patchId string
}

func (p *patchByIdHandler) Handler() RequestHandler {
	return &patchByIdHandler{}
}

func (p *patchByIdHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	p.patchId = vars["patch_id"]
	return nil
}

func (p *patchByIdHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for fetching current users patches
//
//    /patches/mine

type patchesByUserHandler struct {
	PaginationExecutor
}

type patchesByUserArgs struct {
	user string
}

func getPatchesByUserManager(route string, version int) *RouteManager {
	p := &patchesByUserHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        evergreen.MethodGet,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    p.Handler(),
			},
		},
	}
}

func (p *patchesByUserHandler) Handler() RequestHandler {
	return &patchesByUserHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       patchesByUserPaginator,
		Args:            patchesByUserArgs{},
	}}
}

func (p *patchesByUserHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = patchesByUserArgs{mux.Vars(r)["user_id"]}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func patchesByUserPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	user := args.(patchesByUserArgs).user
	grip.Debugln("getting : ", limit, "patches for user: ", user, " starting from time: ", key)
	var ts time.Time
	var err error
	if key == "" {
		ts = time.Now()
	} else {
		ts, err = time.ParseInLocation(model.APITimeFormat, key, time.UTC)
		if err != nil {
			return []model.Model{}, nil, &rest.APIError{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	// sortAsc set to false in order to display patches in desc chronological order
	patches, err := sc.FindPatchesByUser(user, ts, limit*2, false)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	if len(patches) <= 0 {
		err = rest.APIError{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		}
		return []model.Model{}, nil, err
	}

	// Make the previous page
	prevPatches, err := sc.FindPatchesByUser(user, ts, limit, true)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	// populate the page info structure
	pages := &PageResult{}
	if len(patches) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(patches[limit].CreateTime).String(),
			Limit:    len(patches) - limit,
		}
	}
	if len(prevPatches) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(prevPatches[len(prevPatches)-1].CreateTime).String(),
			Limit:    len(prevPatches),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		patches = patches[:limit]
	}
	models := []model.Model{}
	for _, info := range patches {
		patchModel := &model.APIPatch{}
		if err = patchModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, &rest.APIError{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models = append(models, patchModel)
	}

	return models, pages, nil
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the patches for a project
//
//    /projects/{project_id}/patches

type patchesByProjectArgs struct {
	projectId string
}

func getPatchesByProjectManager(route string, version int) *RouteManager {
	p := &patchesByProjectHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     evergreen.MethodGet,
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: p.Handler(),
			},
		},
	}
}

type patchesByProjectHandler struct {
	PaginationExecutor
}

func (p *patchesByProjectHandler) Handler() RequestHandler {
	return &patchesByProjectHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       patchesByProjectPaginator,
		Args:            patchesByProjectArgs{},
	}}
}

func (p *patchesByProjectHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = patchesByProjectArgs{projectId: mux.Vars(r)["project_id"]}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func patchesByProjectPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	proj := args.(patchesByProjectArgs).projectId
	grip.Debugln("getting patches for project: ", proj, " starting from time: ", key)
	var ts time.Time
	var err error
	if key == "" {
		ts = time.Now()
	} else {
		ts, err = time.ParseInLocation(model.APITimeFormat, key, time.UTC)
		if err != nil {
			return []model.Model{}, nil, &rest.APIError{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	// sortDir is set to -1 in order to display patches in reverse chronological order
	patches, err := sc.FindPatchesByProject(proj, ts, limit*2, false)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	if len(patches) <= 0 {
		err = rest.APIError{
			Message:    "no patches found",
			StatusCode: http.StatusNotFound,
		}
		return []model.Model{}, nil, err
	}

	// Make the previous page
	prevPatches, err := sc.FindPatchesByProject(proj, ts, limit, true)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	// populate the page info structure
	pages := &PageResult{}
	if len(patches) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      model.NewTime(patches[limit].CreateTime).String(),
			Limit:    len(patches) - limit,
		}
	}
	if len(prevPatches) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      model.NewTime(prevPatches[len(prevPatches)-1].CreateTime).String(),
			Limit:    len(prevPatches),
		}
	}

	// truncate results data if there's a next page.
	if pages.Next != nil {
		patches = patches[:limit]
	}
	models := []model.Model{}
	for _, info := range patches {
		patchModel := &model.APIPatch{}
		if err = patchModel.BuildFromService(info); err != nil {
			return []model.Model{}, nil, &rest.APIError{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models = append(models, patchModel)
	}

	return models, pages, nil
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
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        evergreen.MethodPost,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    p.Handler(),
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
	vars := mux.Vars(r)
	p.patchId = vars["patch_id"]
	return nil
}

func (p *patchAbortHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	err := sc.AbortPatch(p.patchId, GetUser(ctx).Id)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Abort error")
		}
		return ResponseData{}, err
	}

	// Patch may be deleted by abort (eg not finalized) and not found here
	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)

	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
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
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        evergreen.MethodPost,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    p.Handler(),
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
	vars := mux.Vars(r)
	p.patchId = vars["patch_id"]
	return nil
}

func (p *patchRestartHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {

	// If the version has not been finalized, returns NotFound
	err := sc.RestartVersion(p.patchId, GetUser(ctx).Id)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Restart error")
		}
		return ResponseData{}, err
	}

	foundPatch, err := sc.FindPatchById(p.patchId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}
	patchModel := &model.APIPatch{}
	err = patchModel.BuildFromService(*foundPatch)

	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{patchModel},
	}, nil
}
