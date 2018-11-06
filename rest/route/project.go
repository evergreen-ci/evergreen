package route

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"strconv"

	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
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
	sc      data.Connector
}

func makeFetchProjectVersions(sc data.Connector) gimlet.RouteHandler {
	return &versionsGetHandler{
		sc: sc,
	}
}

func (h *versionsGetHandler) Factory() gimlet.RouteHandler {
	return &versionsGetHandler{
		sc: h.sc,
	}
}

func (h *versionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
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

func (h *versionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	projRef, err := dbModel.FindOneProjectRef(h.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		})
	}

	proj, err := dbModel.FindProject("", projRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		})
	}

	versions, err := h.sc.GetVersionsAndVariants(h.offset, h.limit, proj)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error retrieving versions"))
	}

	return gimlet.NewJSONResponse(versions)
}

/*
 * Creates project (project_ref)
 * PUT: /rest/v2/projects/
 * Perm: checkUser
 */
type projectCreateHandler struct {
	projectRef *model.APIProjectRef

	sc data.Connector
}

func makeProjectCreateRoute(sc data.Connector) gimlet.RouteHandler {
	return &projectCreateHandler{sc: sc}
}

func (h *projectCreateHandler) Factory() gimlet.RouteHandler {
	return &projectCreateHandler{sc: h.sc}
}

func (h *projectCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := util.ReadJSONInto(body, &h.projectRef); err != nil {
		return errors.Wrap(err, "Cannot parse the content as JSON!")
	}

	return nil
}

func (h *projectCreateHandler) Run(ctx context.Context) gimlet.Responder {
	createdProject, err := h.sc.CreateProject(h.projectRef)

	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Cannot create project!"))
	}

	apiProject := &model.APIProject{}
	// The method doesn't accept pointers, we have to dereference it first
	err = apiProject.BuildFromService(*createdProject)

	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "problem converting project document",
			StatusCode: http.StatusInternalServerError,
		})
	}

	resp := gimlet.NewJSONResponse(apiProject)
	err = resp.SetStatus(http.StatusCreated)

	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "Cannot set status code",
			StatusCode: http.StatusInternalServerError,
		})
	}

	return resp
}

/*
 * Updates project (project_ref) with given identifier
 * PATCH: /rest/v2/projects/{project_id}
 * Perm: checkUser
 */
type projectUpdateHandler struct {
	projectRef *model.APIProjectRef
	keys       *[]string

	sc data.Connector
}

func makeProjectUpdateRoute(sc data.Connector) gimlet.RouteHandler {
	return &projectUpdateHandler{sc: sc}
}

func (h *projectUpdateHandler) Factory() gimlet.RouteHandler {
	return &projectUpdateHandler{sc: h.sc}
}

func (h *projectUpdateHandler) Parse(ctx context.Context, r *http.Request) error {
	// Read raw bytes from request body
	body := util.NewRequestReader(r)
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Unable to read request body!")
	}
	if err := body.Close(); err != nil {
		return errors.Wrap(err, "Cannot close reader")
	}

	// Construct body copy and read JSON into projectRef model
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	body = util.NewRequestReader(r)
	if err := util.ReadJSONInto(body, &h.projectRef); err != nil {
		return errors.Wrap(err, "JSON format or content is invalid!")
	}
	if err := body.Close(); err != nil {
		return errors.Wrap(err, "Cannot close reader")
	}

	// Construct body copy and read JSON into payload map
	r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
	body = util.NewRequestReader(r)
	var payload map[string]interface{}
	if err := util.ReadJSONInto(body, &payload); err != nil {
		return errors.Wrap(err, "JSON format or content is invalid!")
	}
	if err := body.Close(); err != nil {
		return errors.Wrap(err, "Cannot close reader")
	}

	// Read keys names from payload
	keys := make([]string, len(payload))
	i := 0
	for k := range payload {
		keys[i] = k
		i++
	}
	h.keys = &keys

	// Set proper ID (from URL param)
	h.projectRef.Identifier = model.ToAPIString(gimlet.GetVars(r)["project_id"])
	return nil
}

func (h *projectUpdateHandler) Run(ctx context.Context) gimlet.Responder {
	updatedProject, err := h.sc.UpdateProject(h.projectRef, h.keys)

	if err != nil {
		// Don't expose error code in order to keep project names in secret
		return gimlet.MakeJSONErrorResponder(errors.New("Cannot update project!"))
	}

	apiProject := &model.APIProject{}
	// The method doesn't accept pointers, we have to dereference it first
	err = apiProject.BuildFromService(*updatedProject)

	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "problem converting project document",
			StatusCode: http.StatusInternalServerError,
		})
	}

	return gimlet.NewJSONResponse(apiProject)
}
