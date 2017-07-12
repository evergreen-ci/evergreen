package route

import (
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

////////////////////////////////////////////////////////////////////////
//
// Handler for fetching patches by id
//
//    /patches/{patch_id}

func getPatchByIdManager(route string, version int) *RouteManager {
	p := &patchByIdHandler{}
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

type patchesByProjectArgs struct {
	projectId string
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the patches for a project
//
//    /projects/{project_id}/patches

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
		ts, err = time.ParseInLocation(model.APITimeFormat, key, time.FixedZone("", 0))
		if err != nil {
			return []model.Model{}, nil, rest.APIError{
				Message:    fmt.Sprintf("problem parsing time from '%s' (%s)", key, err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	// sortDir is set to -1 in order to display patches in reverse chronological order
	patches, err := sc.FindPatchesByProject(proj, ts, limit*2, -1)
	if err != nil {
		if _, ok := err.(rest.APIError); !ok {
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
	prevPatches, err := sc.FindPatchesByProject(proj, ts, limit, 1)
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
			Key:      patches[limit].CreateTime.In(time.UTC).Format(model.APITimeFormat),
			//Key:   model.NewTime(patches[limit].CreateTime).String(),
			Limit: len(patches) - limit,
		}
	}
	if len(prevPatches) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      prevPatches[0].CreateTime.In(time.UTC).Format(model.APITimeFormat),
			//Key:   model.NewTime(prevPatches[0].CreateTime).String(),
			Limit: len(prevPatches),
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
			return []model.Model{}, nil, rest.APIError{
				Message:    "problem converting patch document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		models = append(models, patchModel)
	}

	return models, pages, nil
}
