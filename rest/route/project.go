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
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
				Key:             projects[p.limit].Id,
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
	projRefId, err := dbModel.FindIdForProject(h.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Project not found",
		})
	}

	_, proj, err := dbModel.FindLatestVersionWithValidProject(projRefId)
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
	project string
	body    []byte
	user    *user.DBUser

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
	h.project = gimlet.GetVars(r)["project_id"]
	h.user = MustHaveUser(ctx)
	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	h.body = b

	return nil
}

// Run updates a project by name.
func (h *projectIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	oldProject, err := h.sc.FindProjectById(h.project)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	requestProjectRef := &model.APIProjectRef{}
	if err = requestProjectRef.BuildFromService(*oldProject); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.ProjectRef to model.APIProjectRef"))
	}

	// erase contents so requestProjectRef will only be populated with new elements
	requestProjectRef.Triggers = nil
	// these fields in the request represent the admins/users to be added
	requestProjectRef.Admins = nil
	requestProjectRef.GitTagAuthorizedUsers = nil
	requestProjectRef.GitTagAuthorizedTeams = nil
	if err = json.Unmarshal(h.body, requestProjectRef); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error while unmarshalling JSON"))
	}

	projectId := model.FromStringPtr(requestProjectRef.Id)
	if projectId != oldProject.Id {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("A project's id is immutable; cannot rename project '%s'", h.project),
		})
	}

	i, err := requestProjectRef.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APIProjectRef to model.ProjectRef"))
	}
	newProjectRef, ok := i.(*dbModel.ProjectRef)
	if !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for model.ProjectRef", i),
		})
	}

	if err = newProjectRef.ValidateOwnerAndRepo(h.settings.GithubOrgs); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}
	if newProjectRef.Identifier != oldProject.Identifier {
		if err = newProjectRef.ValidateIdentifier(); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			})
		}
	}
	if newProjectRef.RepoRefId != oldProject.RepoRefId {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "can't change repo ref ID manually",
		})
	}

	before, err := h.sc.GetProjectSettingsEvent(newProjectRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting ProjectSettingsEvent before update for project'%s'", h.project))
	}

	adminsToDelete := model.FromStringPtrSlice(requestProjectRef.DeleteAdmins)
	allAdmins := utility.UniqueStrings(append(oldProject.Admins, newProjectRef.Admins...))      // get original and new admin
	newProjectRef.Admins, _ = utility.StringSliceSymmetricDifference(allAdmins, adminsToDelete) // add users that are in allAdmins and not in adminsToDelete

	usersToDelete := model.FromStringPtrSlice(requestProjectRef.DeleteGitTagAuthorizedUsers)
	allAuthorizedUsers := utility.UniqueStrings(append(oldProject.GitTagAuthorizedUsers, newProjectRef.GitTagAuthorizedUsers...))
	newProjectRef.GitTagAuthorizedUsers, _ = utility.StringSliceSymmetricDifference(allAuthorizedUsers, usersToDelete)

	teamsToDelete := model.FromStringPtrSlice(requestProjectRef.DeleteGitTagAuthorizedTeams)
	allAuthorizedTeams := utility.UniqueStrings(append(oldProject.GitTagAuthorizedTeams, newProjectRef.GitTagAuthorizedTeams...))
	newProjectRef.GitTagAuthorizedTeams, _ = utility.StringSliceSymmetricDifference(allAuthorizedTeams, teamsToDelete)

	if newProjectRef.Enabled {
		var hasHook bool
		hasHook, err = h.sc.EnableWebhooks(ctx, newProjectRef)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling webhooks for project '%s'", h.project))
		}
		// verify enabling PR testing valid
		if newProjectRef.PRTestingEnabled {
			if !hasHook {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "Cannot enable PR Testing in this repo, must enable GitHub webhooks first",
				})
			}

			var ghAliasesDefined bool
			ghAliasesDefined, err = h.hasAliasDefined(requestProjectRef, evergreen.GithubAlias)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't check for alias definitions"))
			}
			if !ghAliasesDefined {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable PR testing without a PR patch definition",
				})
			}

			if err = h.sc.EnablePRTesting(newProjectRef); err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling PR testing for project '%s'", h.project))
			}
		}

		// verify enabling git tag versions is valid
		if newProjectRef.GitTagVersionsEnabled {
			var gitTagAliasesDefined bool
			gitTagAliasesDefined, err = h.hasAliasDefined(requestProjectRef, evergreen.GitTagAlias)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't check for alias definitions"))
			}
			if !gitTagAliasesDefined {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable git tag versions without a version definition",
				})
			}
			if len(newProjectRef.GitTagAuthorizedUsers) == 0 && len(newProjectRef.GitTagAuthorizedTeams) == 0 {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "must authorize users or teams to create git tag versions",
				})
			}
		}

		// verify enabling commit queue valid
		var temp interface{}
		temp, err = requestProjectRef.CommitQueue.ToService()
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

			var cqAliasesDefined bool
			cqAliasesDefined, err = h.hasAliasDefined(requestProjectRef, evergreen.CommitQueueAlias)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "can't check for alias definitions"))
			}
			if !cqAliasesDefined {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable commit queue without a commit queue patch definition",
				})
			}
			if err = h.sc.EnableCommitQueue(newProjectRef, commitQueueParams); err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling commit queue for project '%s'", h.project))
			}
		}
	}

	// validate triggers before updating project
	catcher := grip.NewSimpleCatcher()
	for i, trigger := range newProjectRef.Triggers {
		catcher.Add(trigger.Validate(newProjectRef.Id))
		if trigger.DefinitionID == "" {
			newProjectRef.Triggers[i].DefinitionID = utility.RandomString()
		}
	}
	newProjectRef.Triggers = append(oldProject.Triggers, newProjectRef.Triggers...)
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "error validating triggers"))
	}

	newRevision := model.FromStringPtr(requestProjectRef.Revision)
	if newRevision != "" {
		if err = h.sc.UpdateProjectRevision(h.project, newRevision); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		newProjectRef.RepotrackerError = &dbModel.RepositoryErrorDetails{
			Exists:            false,
			InvalidRevision:   "",
			MergeBaseRevision: "",
		}
	}
	if oldProject.Restricted != newProjectRef.Restricted {
		if newProjectRef.Restricted {
			err = newProjectRef.MakeRestricted(ctx)
		} else {
			err = oldProject.MakeUnrestricted(ctx)
		}
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// if owner/repo has changed or we're toggling repo settings off, update scope
	if newProjectRef.Owner != oldProject.Owner || newProjectRef.Repo != oldProject.Repo ||
		(!newProjectRef.UseRepoSettings && oldProject.UseRepoSettings) {
		if err = newProjectRef.RemoveFromRepoScope(); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error removing project from old repo scope"))
		}
		newProjectRef.RepoRefId = "" // if using repo settings, will reassign this in the next block
	}
	if newProjectRef.UseRepoSettings && newProjectRef.RepoRefId == "" {
		if err = newProjectRef.AddToRepoScope(h.user); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// complete all updates
	if err = h.sc.UpdateProject(newProjectRef); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() by project id '%s'", h.project))
	}
	if err = h.sc.UpdateProjectVars(projectId, &requestProjectRef.Variables, false); err != nil { // destructively modifies requestProjectRef.Variables
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating variables for project '%s'", h.project))
	}
	if err = h.sc.UpdateProjectAliases(projectId, requestProjectRef.Aliases); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating aliases for project '%s'", h.project))
	}

	if err = h.sc.UpdateAdminRoles(newProjectRef, newProjectRef.Admins, adminsToDelete); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error updating admins for project '%s'", h.project))
	}
	for i := range requestProjectRef.Subscriptions {
		requestProjectRef.Subscriptions[i].OwnerType = model.ToStringPtr(string(event.OwnerTypeProject))
		requestProjectRef.Subscriptions[i].Owner = model.ToStringPtr(h.project)
	}
	if err = h.sc.SaveSubscriptions(projectId, requestProjectRef.Subscriptions); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error saving subscriptions for project '%s'", h.project))
	}

	toDelete := []string{}
	for _, deleteSub := range requestProjectRef.DeleteSubscriptions {
		toDelete = append(toDelete, model.FromStringPtr(deleteSub))
	}
	if err = h.sc.DeleteSubscriptions(projectId, toDelete); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error deleting subscriptions for project '%s'", h.project))
	}

	after, err := h.sc.GetProjectSettingsEvent(newProjectRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting ProjectSettingsEvent after update for project '%s'", h.project))
	}
	if err = dbModel.LogProjectModified(projectId, h.user.Username(), before, after); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error logging project modification for project '%s'", h.project))
	}

	// run the repotracker for the project
	if newRevision != "" {
		ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectId)

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

// verify for a given alias that either the user has added a new definition or there is a pre-existing definition
func (h *projectIDPatchHandler) hasAliasDefined(pRef *model.APIProjectRef, alias string) (bool, error) {
	aliasesToDelete := map[string]bool{}
	for _, a := range pRef.Aliases {
		// return immediately if a new definition has been added
		if model.FromStringPtr(a.Alias) == alias && !a.Delete {
			return true, nil
		}
		aliasesToDelete[model.FromStringPtr(a.ID)] = a.Delete
	}

	// check if a definition exists and hasn't been deleted
	aliases, err := h.sc.FindProjectAliases(model.FromStringPtr(pRef.Id))
	if err != nil {
		return false, errors.Wrapf(err, "Error checking existing patch definitions")
	}
	for _, a := range aliases {
		if model.FromStringPtr(a.Alias) == alias && !aliasesToDelete[model.FromStringPtr(a.ID)] {
			return true, nil
		}
	}
	return false, nil
}

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/projects/{project_id}

type projectIDPutHandler struct {
	projectName string
	body        []byte
	sc          data.Connector
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
	h.projectName = gimlet.GetVars(r)["project_id"]

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
	p, err := h.sc.FindProjectById(h.projectName)
	if err != nil && err.(gimlet.ErrorResponse).StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error for find() by project id '%s'", h.projectName))
	}
	if p != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot create project with id '%s'", h.projectName),
		})
	}

	apiProjectRef := &model.APIProjectRef{}
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
	dbProjectRef.Id = h.projectName
	dbProjectRef.Identifier = h.projectName

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = h.sc.CreateProject(dbProjectRef, u); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error for insert() distro with distro id '%s'", h.projectName))
	}

	return responder
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/repotracker

type projectRepotrackerHandler struct {
	projectName string
	sc          data.Connector
}

func makeRunRepotrackerForProject(sc data.Connector) gimlet.RouteHandler {
	return &projectRepotrackerHandler{
		sc: sc,
	}
}

func (h *projectRepotrackerHandler) Factory() gimlet.RouteHandler {
	return &projectRepotrackerHandler{
		sc: h.sc,
	}
}

func (h *projectRepotrackerHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectRepotrackerHandler) Run(ctx context.Context) gimlet.Responder {
	projectId, err := dbModel.FindIdForProject(h.projectName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "can't find project '%s'", h.projectName))
	}

	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("rest-%s", ts), projectId)

	queue := evergreen.GetEnvironment().RemoteQueue()
	if err := queue.Put(ctx, j); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem creating catchup job from rest route"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}

type projectIDGetHandler struct {
	projectName string
	sc          data.Connector
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
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	project, err := h.sc.FindProjectById(h.projectName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if project == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' doesn't exist", h.projectName))
	}

	projectModel := &model.APIProjectRef{}

	if err = projectModel.BuildFromService(project); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "problem converting project document",
			StatusCode: http.StatusInternalServerError,
		})
	}

	variables, err := h.sc.FindProjectVarsById(project.Id, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	projectModel.Variables = *variables
	if projectModel.Aliases, err = h.sc.FindProjectAliases(project.Id); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if projectModel.Subscriptions, err = h.sc.GetSubscriptions(project.Id, event.OwnerTypeProject); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(projectModel)
}

type getProjectVersionsHandler struct {
	projectName string
	sc          data.Connector
	startOrder  int
	limit       int
	requester   string
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
	h.projectName = gimlet.GetVars(r)["project_id"]
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
	versions, err := h.sc.GetVersionsInProject(h.projectName, h.requester, h.limit, h.startOrder)
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

type GetProjectAliasResultsHandler struct {
	version             string
	alias               string
	includeDependencies bool

	sc data.Connector
}

func makeGetProjectAliasResultsHandler(sc data.Connector) gimlet.RouteHandler {
	return &GetProjectAliasResultsHandler{
		sc: sc,
	}
}

func (p *GetProjectAliasResultsHandler) Factory() gimlet.RouteHandler {
	return &GetProjectAliasResultsHandler{
		sc: p.sc,
	}
}

func (p *GetProjectAliasResultsHandler) Parse(ctx context.Context, r *http.Request) error {
	params := r.URL.Query()

	p.version = params.Get("version")
	if p.version == "" {
		return errors.New("'version' parameter must be specified")
	}
	p.alias = params.Get("alias")
	if p.alias == "" {
		return errors.New("'alias' parameter must be specified")
	}
	p.includeDependencies = (params.Get("include_deps") == "true")

	return nil
}

func (p *GetProjectAliasResultsHandler) Run(ctx context.Context) gimlet.Responder {
	proj, err := dbModel.FindProjectFromVersionID(p.version)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting project for version",
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.New("unable to get project from version"))
	}

	variantTasks, err := p.sc.GetProjectAliasResults(proj, p.alias, p.includeDependencies)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(variantTasks)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the patch trigger aliases defined for project
//
//    /projects/{project_id}/parameters
type GetPatchTriggerAliasHandler struct {
	projectID string
	sc        data.Connector
}

func makeFetchPatchTriggerAliases(sc data.Connector) gimlet.RouteHandler {
	return &GetPatchTriggerAliasHandler{
		sc: sc,
	}
}

func (p *GetPatchTriggerAliasHandler) Factory() gimlet.RouteHandler {
	return &GetPatchTriggerAliasHandler{
		sc: p.sc,
	}
}

func (p *GetPatchTriggerAliasHandler) Parse(ctx context.Context, r *http.Request) error {
	p.projectID = gimlet.GetVars(r)["project_id"]
	return nil
}

func (p *GetPatchTriggerAliasHandler) Run(ctx context.Context) gimlet.Responder {
	proj, err := dbModel.FindOneProjectRef(p.projectID)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting project",
			"project": p.projectID,
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("unable to get project '%s'", p.projectID))
	}

	triggerAliases := make([]string, 0, len(proj.PatchTriggerAliases))
	for _, a := range proj.PatchTriggerAliases {
		triggerAliases = append(triggerAliases, a.Alias)
	}
	return gimlet.NewJSONResponse(triggerAliases)
}

////////////////////////////////////////////////////////////////////////
//
// Handler for the most recent parameters of a project
//
//    /projects/{project_id}/parameters

type projectParametersGetHandler struct {
	projectName string
	sc          data.Connector
}

func makeFetchParameters(sc data.Connector) gimlet.RouteHandler {
	return &projectParametersGetHandler{
		sc: sc,
	}
}

func (h *projectParametersGetHandler) Factory() gimlet.RouteHandler {
	return &projectParametersGetHandler{
		sc: h.sc,
	}
}

func (h *projectParametersGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectParametersGetHandler) Run(ctx context.Context) gimlet.Responder {
	id, err := dbModel.FindIdForProject(h.projectName)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "error finding project '%s'", id))
	}
	_, p, err := dbModel.FindLatestVersionWithValidProject(id)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err,
			"error finding project config for project '%s'", id))
	}
	if p == nil {
		return gimlet.NewJSONErrorResponse(errors.Errorf("project '%s' not found", id))
	}

	// convert to API structure
	res := make([]model.APIParameterInfo, len(p.Parameters))
	for i, param := range p.Parameters {
		var apiParam model.APIParameterInfo
		if err = apiParam.BuildFromService(param); err != nil {
			return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err,
				"error converting to API structure"))
		}
		res[i] = apiParam
	}

	return gimlet.NewJSONResponse(res)
}
