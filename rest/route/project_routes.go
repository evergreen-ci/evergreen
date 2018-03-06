package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type projectGetHandler struct {
	PaginationExecutor
}

type projectGetArgs struct {
	User *user.DBUser
}

func getProjectRouteManager(route string, version int) *RouteManager {
	p := &projectGetHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &NoAuthAuthenticator{},
				RequestHandler:    p.Handler(),
				MethodType:        http.MethodGet,
			},
		},
	}
}

func (p *projectGetHandler) Handler() RequestHandler {
	return &projectGetHandler{PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       projectPaginator,
	}}
}

func (p *projectGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	p.Args = projectGetArgs{User: GetUser(ctx)}

	return p.PaginationExecutor.ParseAndValidate(ctx, r)
}

func projectPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model, *PageResult, error) {
	grip.Debugln("fetching all projects")
	isAuthenticated := false
	if args.(projectGetArgs).User != nil {
		isAuthenticated = true
	}
	projects, err := sc.FindProjects(key, limit*2, 1, isAuthenticated)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	if len(projects) <= 0 {
		err = &rest.APIError{
			Message:    "no projects found",
			StatusCode: http.StatusNotFound,
		}
		return []model.Model{}, nil, err
	}

	// Make the previous page
	prevProjects, err := sc.FindProjects(key, limit, -1, isAuthenticated)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	// Populate page info
	pages := &PageResult{}
	if len(projects) > limit {
		pages.Next = &Page{
			Relation: "next",
			Key:      projects[limit].Identifier,
			Limit:    len(projects) - limit,
		}
	}
	if len(prevProjects) >= 1 {
		pages.Prev = &Page{
			Relation: "prev",
			Key:      prevProjects[len(prevProjects)-1].Identifier,
			Limit:    len(prevProjects),
		}
	}

	// Truncate results data if there's a next page
	if pages.Next != nil {
		projects = projects[:limit]
	}
	models := []model.Model{}
	for _, p := range projects {
		projectModel := &model.APIProject{}
		if err = projectModel.BuildFromService(p); err != nil {
			return []model.Model{}, nil, &rest.APIError{
				Message:    "problem converting project document",
				StatusCode: http.StatusInternalServerError,
			}
		}

		// now set the vars field
		vars, err := sc.FindProjectVars(string(projectModel.Identifier))
		if err != nil {
			return []model.Model{}, nil, &rest.APIError{
				Message:    "problem fetching project vars",
				StatusCode: http.StatusInternalServerError,
			}
		}
		if vars != nil {
			vars.RedactPrivateVars()
			projectModel.Vars = vars.Vars
		}
		models = append(models, projectModel)
	}

	return models, pages, nil
}

type waterfallVersionsHandler struct {
	projectId string
}
