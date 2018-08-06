package route

import (
	"context"
	"net/http"
	"strconv"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type projectGetHandler struct {
	key   string
	limit int
	user  *user.DBUser
	sc    data.Connector
}

func makeFetchProjectsRoute(sc data.Connector) gimlet.RouteHandler {
	return &projectGetHandler{
		sc: sc,
	}
}

func (p *projectGetHandler) Factory() gimlet.RouteHandler {
	return &projectGetHandler{
		sc: p.sc,
	}
}

func (p *projectGetHandler) Parse(ctx context.Context, r *http.Request) error {
	p.user, _ = gimlet.GetUser(ctx).(*user.DBUser)

	vals := r.URL.Query()

	p.key = vals.Get("start_at")
	var err error
	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *projectGetHandler) Run(ctx context.Context) gimlet.Responder {
	isAuthenticated := false
	if p.user != nil {
		isAuthenticated = true
	}

	projects, err := p.sc.FindProjects(p.key, p.limit+1, 1, isAuthenticated)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	if len(projects) == 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "no projects found",
			StatusCode: http.StatusNotFound,
		})
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(projects)
	if len(projects) > p.limit {
		lastIndex = p.limit

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.sc.GetURL(),
				Key:             projects[p.limit].Identifier,
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}
	projects = projects[:lastIndex]

	for _, proj := range projects {
		projectModel := &model.APIProject{}
		if err = projectModel.BuildFromService(proj); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    "problem converting project document",
				StatusCode: http.StatusInternalServerError,
			})
		}
		if err = resp.AddData(projectModel); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

type versionsGetHandler struct {
	project string
	limit   int
	offset  int
}

func getRecentVersionsManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				Authenticator:  &NoAuthAuthenticator{},
				RequestHandler: &versionsGetHandler{},
				MethodType:     http.MethodGet,
			},
		},
		Version: version,
	}
}

func (h *versionsGetHandler) Handler() RequestHandler {
	return &versionsGetHandler{}
}

func (h *versionsGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	var err error
	h.project = gimlet.GetVars(r)["project_id"]
	var query = r.URL.Query()

	limit := query.Get("limit")
	if limit != "" {
		h.limit, err = strconv.Atoi(limit)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid limit",
			}
		}
	} else {
		h.limit = 10
	}

	offset := query.Get("offset")
	if offset != "" {
		h.offset, err = strconv.Atoi(offset)
		if err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid offset",
			}
		}
	} else {
		h.offset = 0
	}

	return nil
}

func (h *versionsGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	projRef, err := dbModel.FindOneProjectRef(h.project)
	if err != nil {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		}
	}
	proj, err := dbModel.FindProject("", projRef)
	if err != nil {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		}
	}
	versions, err := sc.GetVersionsAndVariants(h.offset, h.limit, proj)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Error retrieving versions")
	}
	return ResponseData{
		Result: []model.Model{versions},
	}, nil
}
