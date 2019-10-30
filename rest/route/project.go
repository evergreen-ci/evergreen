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
	"github.com/evergreen-ci/evergreen/model/event"
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
	projects, err := p.sc.FindProjects(p.key, p.limit+1, 1)
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

	proj, err := dbModel.FindLastKnownGoodProject(projRef.Identifier)
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
	body      []byte

	sc       data.Connector
	settings *evergreen.Settings
}

func makePatchProjectByID(sc data.Connector, settings *evergreen.Settings) gimlet.RouteHandler {
	return &projectIDPatchHandler{
		sc:       sc,
		settings: settings,
	}
}

func (h *projectIDPatchHandler) Factory() gimlet.RouteHandler {
	return &projectIDPatchHandler{
		sc:       h.sc,
		settings: h.settings,
	}
}

// Parse fetches the project's identifier from the http request.
func (h *projectIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
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
	// erase contents so apiProjectRef will only be populated with new elements
	apiProjectRef.Admins = nil
	apiProjectRef.Triggers = nil
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

	if err := dbProjectRef.ValidateOwnerAndRepo(h.settings.GithubOrgs); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
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

	adminsToDelete := []string{}
	for _, admin := range apiProjectRef.DeleteAdmins {
		adminsToDelete = append(adminsToDelete, model.FromAPIString(admin))
	}
	allAdmins := util.UniqueStrings(append(p.Admins, dbProjectRef.Admins...)) // get original and new admin
	dbProjectRef.Admins = []string{}
	for _, admin := range allAdmins {
		if !util.StringSliceContains(adminsToDelete, admin) {
			dbProjectRef.Admins = append(dbProjectRef.Admins, admin)
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
	dbProjectRef.Triggers = append(p.Triggers, dbProjectRef.Triggers...)
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "error validating triggers"))
	}

	newRevision := model.FromAPIString(apiProjectRef.Revision)
	if newRevision != "" {
		if err = h.sc.UpdateProjectRevision(h.projectID, newRevision); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		dbProjectRef.RepotrackerError = &dbModel.RepositoryErrorDetails{
			Exists:            false,
			InvalidRevision:   "",
			MergeBaseRevision: "",
		}
	}

	// complete all updates
	if err = h.sc.UpdateProject(dbProjectRef); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() by project id '%s'", h.projectID))
	}
	if err = h.sc.UpdateProjectVars(h.projectID, &apiProjectRef.Variables); err != nil { // destructively modifies apiProjectRef.Variables
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating variables for project '%s'", h.projectID))
	}
	if err = h.sc.UpdateProjectAliases(h.projectID, apiProjectRef.Aliases); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating aliases for project '%s'", h.projectID))
	}

	for i := range apiProjectRef.Subscriptions {
		apiProjectRef.Subscriptions[i].OwnerType = model.ToAPIString(string(event.OwnerTypeProject))
		apiProjectRef.Subscriptions[i].Owner = model.ToAPIString(h.projectID)
	}
	if err = h.sc.SaveSubscriptions(h.projectID, apiProjectRef.Subscriptions); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error saving subscriptions for project '%s'", h.projectID))
	}

	toDelete := []string{}
	for _, deleteSub := range apiProjectRef.DeleteSubscriptions {
		toDelete = append(toDelete, model.FromAPIString(deleteSub))
	}
	if err = h.sc.DeleteSubscriptions(h.projectID, toDelete); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error deleting subscriptions for project '%s'", h.projectID))
	}

	// run the repotracker for the project
	if newRevision != "" {
		ts := util.RoundPartOfHour(1).Format(tsFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), h.projectID)

		queue := evergreen.GetEnvironment().RemoteQueue()
		if err = queue.Put(ctx, j); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem creating catchup job"))
		}
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusOK); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusOK))
	}
	return responder
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

// creates a new resource based on the Request-URI and JSON payload and returns a http.StatusCreated (201)
func (h *projectIDPutHandler) Run(ctx context.Context) gimlet.Responder {
	p, err := h.sc.FindProjectById(h.projectID)
	if err != nil && err.(gimlet.ErrorResponse).StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error for find() by project id '%s'", h.projectID))
	}
	if p != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot create project with id '%s'", h.projectID),
		})
	}
	apiProjectRef := &model.APIProjectRef{Identifier: model.ToAPIString(h.projectID)}
	if err = json.Unmarshal(h.body, apiProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
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

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}
	if err = h.sc.CreateProject(dbProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error for insert() distro with distro id '%s'", h.projectID))
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

	variables, err := h.sc.FindProjectVarsById(h.projectID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	projectModel.Variables = *variables
	if projectModel.Aliases, err = h.sc.FindProjectAliases(h.projectID); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if projectModel.Subscriptions, err = h.sc.GetSubscriptions(h.projectID, event.OwnerTypeProject); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(projectModel)
}

type getProjectVersionsHandler struct {
	projectID  string
	sc         data.Connector
	startOrder int
	limit      int
	requester  string
}

func makeGetProjectVersionsHandler(sc data.Connector) gimlet.RouteHandler {
	return &getProjectVersionsHandler{
		sc: sc,
	}
}

func (h *getProjectVersionsHandler) Factory() gimlet.RouteHandler {
	return &getProjectVersionsHandler{
		sc: h.sc,
	}
}

func (h *getProjectVersionsHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]
	params := r.URL.Query()

	limitStr := params.Get("limit")
	if limitStr == "" {
		h.limit = 20
	} else {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return errors.Wrap(err, "'limit' query parameter must be a valid integer")
		}
		if limit < 1 {
			return errors.New("'limit' must be a positive integer")
		}
		h.limit = limit
	}

	startStr := params.Get("start")
	if startStr == "" {
		h.startOrder = 0
	} else {
		startOrder, err := strconv.Atoi(params.Get("start"))
		if err != nil {
			return errors.Wrap(err, "'start' query parameter must be a valid integer")
		}
		if startOrder < 0 {
			return errors.New("'start' must be a non-negative integer")
		}
		h.startOrder = startOrder
	}

	h.requester = params.Get("requester")
	if h.requester == "" {
		return errors.New("'requester' must be one of patch_request, gitter_request, github_pull_request, merge_test, ad_hoc")
	}
	return nil
}

func (h *getProjectVersionsHandler) Run(ctx context.Context) gimlet.Responder {
	versions, err := h.sc.GetVersionsInProject(h.projectID, h.requester, h.limit, h.startOrder)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	resp, err := gimlet.NewBasicResponder(http.StatusOK, gimlet.JSON, versions)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error constructing response"))
	}

	if len(versions) >= h.limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start",
				BaseURL:         h.sc.GetURL(),
				Key:             strconv.Itoa(versions[len(versions)-1].Order),
				Limit:           h.limit,
			},
		})

		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error paginating response"))
		}
	}

	return resp
}
