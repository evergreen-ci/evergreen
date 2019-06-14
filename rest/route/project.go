package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const tsFormat = "2006-01-02.15-04-05"

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
		projectModel := &model.APIProjectRef{}
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

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/projects/{project_id}

type projectIDPatchHandler struct {
	projectID string
	revision  string
	body      []byte
	sc        data.Connector
}

func makePatchProjectByID(sc data.Connector) gimlet.RouteHandler {
	return &projectIDPatchHandler{
		sc: sc,
	}
}

func (h *projectIDPatchHandler) Factory() gimlet.RouteHandler {
	return &projectIDPatchHandler{
		sc: h.sc,
	}
}

// Parse fetches the project's identifier from the http request.
func (h *projectIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]
	h.revision = r.URL.Query().Get("revision")
	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b

	return nil
}

// Run updates a project by identifier.
func (h *projectIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	p, err := h.sc.FindProjectById(h.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by project id '%s'", h.projectID))
	}

	apiProjectRef := &model.APIProjectRef{}
	if err = apiProjectRef.BuildFromService(*p); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.ProjectRef to model.APIProjectRef"))
	}

	if err = json.Unmarshal(h.body, apiProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	identifier := model.FromAPIString(apiProjectRef.Identifier)
	if h.projectID != identifier {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("A project's id is immutable; cannot rename project '%s'", h.projectID),
		})
	}

	i, err := apiProjectRef.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APIProjectRef to model.ProjectRef"))
	}
	dbProjectRef, ok := i.(*dbModel.ProjectRef)
	if !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for model.ProjectRef", i),
		})
	}

	// verify input and webhooks
	if dbProjectRef.Owner == "" || dbProjectRef.Repo == "" {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no owner/repo specified",
		})
	}

	if dbProjectRef.Enabled {
		var hasHook bool
		hasHook, err = h.sc.EnableWebhooks(ctx, dbProjectRef)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling webhooks for project '%s'", h.projectID))
		}
		// verify enabling PR testing valid
		if dbProjectRef.PRTestingEnabled {
			if !hasHook {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "Cannot enable PR Testing in this repo, must enable GitHub webhooks first",
				})
			}
			if err = h.sc.EnablePRTesting(dbProjectRef); err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling PR testing for project '%s'", h.projectID))
			}
		}
		// verify enabling commit queue valid
		var temp interface{}
		temp, err = apiProjectRef.CommitQueue.ToService()
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from APICommitQueueParams to CommitQueueParams"))
		}
		commitQueueParams, ok := temp.(dbModel.CommitQueueParams)
		if !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("Unexpected type %T for APICommitQueueParams", i),
			})
		}
		if commitQueueParams.Enabled {
			if !hasHook {
				gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "Cannot enable commit queue in this repo, must enable GitHub webhooks first",
				})
			}
			if err = h.sc.EnableCommitQueue(dbProjectRef, commitQueueParams); err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling commit queue for project '%s'", h.projectID))
			}
		}
	}

	// validate triggers before updating project
	catcher := grip.NewSimpleCatcher()
	for i, trigger := range dbProjectRef.Triggers {
		catcher.Add(trigger.Validate(dbProjectRef.Identifier))
		if trigger.DefinitionID == "" {
			dbProjectRef.Triggers[i].DefinitionID = util.RandomString()
		}
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "error validating triggers"))
	}

	if h.revision != "" {
		if err = h.sc.UpdateProjectRevision(h.projectID, h.revision); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		dbProjectRef.RepotrackerError = &dbModel.RepositoryErrorDetails{
			Exists:            false,
			InvalidRevision:   "",
			MergeBaseRevision: "",
		}
	}

	if err = h.sc.UpdateProject(dbProjectRef); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() by project id '%s'", h.projectID))
	}

	// run the repotracker for the project
	if h.revision != "" {
		ts := util.RoundPartOfHour(1).Format(tsFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), h.projectID)

		queue := evergreen.GetEnvironment().RemoteQueue()
		if err := queue.Put(ctx, j); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem creating catchup job"))
		}
	}
	return gimlet.NewJSONResponse(apiProjectRef)
}

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/projects/{project_id}

type projectIDPutHandler struct {
	projectID string
	body      []byte
	sc        data.Connector
}

func makePutProjectByID(sc data.Connector) gimlet.RouteHandler {
	return &projectIDPutHandler{
		sc: sc,
	}
}

func (h *projectIDPutHandler) Factory() gimlet.RouteHandler {
	return &projectIDPutHandler{
		sc: h.sc,
	}
}

// Parse fetches the distroId and JSON payload from the http request.
func (h *projectIDPutHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]

	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b

	return nil
}

// Run either:
// (a) replaces an existing resource with the entity defined in the JSON payload and returns a http.StatusOk (200), or
// (b) creates a new resource based on the Request-URI and JSON payload and returns a http.StatusCreated (201)
func (h *projectIDPutHandler) Run(ctx context.Context) gimlet.Responder {
	original, err := h.sc.FindProjectById(h.projectID)
	if err != nil && err.(gimlet.ErrorResponse).StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by project id '%s'", h.projectID))
	}

	apiProjectRef := &model.APIProjectRef{Identifier: model.ToAPIString(h.projectID)}
	if err = json.Unmarshal(h.body, apiProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	identifier := model.FromAPIString(apiProjectRef.Identifier)
	if h.projectID != identifier {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("A project's id is immutable; cannot rename project '%s'", h.projectID),
		})
	}

	i, err := apiProjectRef.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APIProjectRef to model.ProjectRef"))
	}
	dbProjectRef, ok := i.(*dbModel.ProjectRef)
	if !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for model.ProjectRef", i),
		})
	}
	// Existing resource
	if original != nil {
		if err = h.sc.UpdateProject(dbProjectRef); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() with project id '%s'", h.projectID))
		}
		return gimlet.NewJSONResponse(struct{}{})
	}
	// New resource
	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}
	if err = h.sc.CreateProject(dbProjectRef); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for insert() distro with distro id '%s'", h.projectID))
	}

	return responder
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}

type projectIDGetHandler struct {
	projectID string
	sc        data.Connector
}

func makeGetProjectByID(sc data.Connector) gimlet.RouteHandler {
	return &projectIDGetHandler{
		sc: sc,
	}
}

func (h *projectIDGetHandler) Factory() gimlet.RouteHandler {
	return &projectIDGetHandler{
		sc: h.sc,
	}
}

func (h *projectIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	project, err := h.sc.FindProjectById(h.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	projectModel := &model.APIProjectRef{}

	if err = projectModel.BuildFromService(project); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "problem converting project document",
			StatusCode: http.StatusInternalServerError,
		})
	}

	return gimlet.NewJSONResponse(projectModel)
}
