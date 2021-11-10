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

type legacyVersionsGetHandler struct {
	project string
	limit   int
	offset  int
	sc      data.Connector
}

func makeFetchProjectVersionsLegacy(sc data.Connector) gimlet.RouteHandler {
	return &legacyVersionsGetHandler{
		sc: sc,
	}
}

func (h *legacyVersionsGetHandler) Factory() gimlet.RouteHandler {
	return &legacyVersionsGetHandler{
		sc: h.sc,
	}
}

func (h *legacyVersionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
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

func (h *legacyVersionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	projRefId, err := dbModel.GetIdForProject(h.project)
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
	project          string
	user             *user.DBUser
	newProjectRef    *dbModel.ProjectRef
	originalProject  *dbModel.ProjectRef
	apiNewProjectRef *model.APIProjectRef

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

	oldProject, err := h.sc.FindProjectById(h.project, false, false)
	if err != nil {
		return errors.Wrap(err, "error finding original project")
	}
	requestProjectRef := &model.APIProjectRef{}
	if err = requestProjectRef.BuildFromService(*oldProject); err != nil {
		return errors.Wrap(err, "API error converting from model.ProjectRef to model.APIProjectRef")
	}

	// erase contents so apiNewProjectRef will only be populated with new elements for these fields
	requestProjectRef.Admins = nil
	requestProjectRef.GitTagAuthorizedUsers = nil
	requestProjectRef.GitTagAuthorizedTeams = nil

	if err = json.Unmarshal(b, requestProjectRef); err != nil {
		return errors.Wrap(err, "API error while unmarshalling JSON")
	}

	projectId := utility.FromStringPtr(requestProjectRef.Id)
	if projectId != oldProject.Id {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    "A project's id is immutable",
		}
	}

	i, err := requestProjectRef.ToService()
	if err != nil {
		return errors.Wrap(err, "API error converting from model.APIProjectRef to model.ProjectRef")
	}
	newProjectRef, ok := i.(*dbModel.ProjectRef)
	if !ok {
		return errors.Errorf("Unexpected type %T for model.ProjectRef", i)
	}
	newProjectRef.RepoRefId = oldProject.RepoRefId // this can't be modified by users

	h.newProjectRef = newProjectRef
	h.originalProject = oldProject
	h.apiNewProjectRef = requestProjectRef // needed for the delete fields
	return nil
}

// Run updates a project by name.
func (h *projectIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	if err := h.newProjectRef.ValidateOwnerAndRepo(h.settings.GithubOrgs); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}
	if h.newProjectRef.Identifier != h.originalProject.Identifier {
		if err := h.newProjectRef.ValidateIdentifier(); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			})
		}
	}

	before, err := h.sc.GetProjectSettings(h.newProjectRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting ProjectSettings before update for project'%s'", h.project))
	}

	adminsToDelete := utility.FromStringPtrSlice(h.apiNewProjectRef.DeleteAdmins)
	adminsToAdd := h.newProjectRef.Admins
	allAdmins := utility.UniqueStrings(append(h.originalProject.Admins, h.newProjectRef.Admins...)) // get original and new admin
	h.newProjectRef.Admins, _ = utility.StringSliceSymmetricDifference(allAdmins, adminsToDelete)   // add users that are in allAdmins and not in adminsToDelete

	usersToDelete := utility.FromStringPtrSlice(h.apiNewProjectRef.DeleteGitTagAuthorizedUsers)
	allAuthorizedUsers := utility.UniqueStrings(append(h.originalProject.GitTagAuthorizedUsers, h.newProjectRef.GitTagAuthorizedUsers...))
	h.newProjectRef.GitTagAuthorizedUsers, _ = utility.StringSliceSymmetricDifference(allAuthorizedUsers, usersToDelete)

	teamsToDelete := utility.FromStringPtrSlice(h.apiNewProjectRef.DeleteGitTagAuthorizedTeams)
	allAuthorizedTeams := utility.UniqueStrings(append(h.originalProject.GitTagAuthorizedTeams, h.newProjectRef.GitTagAuthorizedTeams...))
	h.newProjectRef.GitTagAuthorizedTeams, _ = utility.StringSliceSymmetricDifference(allAuthorizedTeams, teamsToDelete)

	// If the project ref doesn't use the repo, then this will just be the same as newProjectRef.
	// Used to verify that if something is set to nil in the request, we properly validate using the merged project ref.
	mergedProjectRef, err := dbModel.GetProjectRefMergedWithRepo(*h.newProjectRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error merging project ref"))
	}

	if h.newProjectRef.IsEnabled() {
		var hasHook bool
		hasHook, err = h.sc.EnableWebhooks(ctx, h.newProjectRef)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling webhooks for project '%s'", h.project))
		}

		var allAliases []model.APIProjectAlias
		if mergedProjectRef.AliasesNeeded() {
			allAliases, err = h.sc.FindProjectAliases(utility.FromStringPtr(h.apiNewProjectRef.Id), mergedProjectRef.RepoRefId, h.apiNewProjectRef.Aliases)
			if err != nil {
				return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "error checking existing patch definitions"))
			}
		}

		// verify enabling PR testing valid
		if mergedProjectRef.IsPRTestingEnabled() && !h.originalProject.IsPRTestingEnabled() {
			if !hasHook {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "Cannot enable PR Testing in this repo, must enable GitHub webhooks first",
				})
			}

			if !hasAliasDefined(allAliases, evergreen.GithubPRAlias) {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable PR testing without a PR patch definition",
				})
			}

			if err = h.sc.EnablePRTesting(h.newProjectRef); err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling PR testing for project '%s'", h.project))
			}
		}

		// verify enabling github checks is valid
		if mergedProjectRef.IsGithubChecksEnabled() && !h.originalProject.IsGithubChecksEnabled() {
			if !hasAliasDefined(allAliases, evergreen.GithubChecksAlias) {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable github checks without a version definition",
				})
			}
		}

		// verify enabling git tag versions is valid
		if mergedProjectRef.IsGitTagVersionsEnabled() && !h.originalProject.IsGitTagVersionsEnabled() {
			if !hasAliasDefined(allAliases, evergreen.GitTagAlias) {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable git tag versions without a version definition",
				})
			}
		}

		// verify enabling commit queue valid
		if mergedProjectRef.CommitQueue.IsEnabled() && !h.originalProject.CommitQueue.IsEnabled() {
			if !hasHook {
				gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "Cannot enable commit queue in this repo, must enable GitHub webhooks first",
				})
			}

			if !hasAliasDefined(allAliases, evergreen.CommitQueueAlias) {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "cannot enable commit queue without a commit queue patch definition",
				})
			}
			if err = h.sc.EnableCommitQueue(h.newProjectRef); err != nil {
				return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error enabling commit queue for project '%s'", h.project))
			}
		}
	}

	// validate triggers before updating project
	catcher := grip.NewSimpleCatcher()
	for i := range h.newProjectRef.Triggers {
		catcher.Add(h.newProjectRef.Triggers[i].Validate(h.newProjectRef.Id))
	}
	for i := range h.newProjectRef.PatchTriggerAliases {
		h.newProjectRef.PatchTriggerAliases[i], err = dbModel.ValidateTriggerDefinition(h.newProjectRef.PatchTriggerAliases[i], h.newProjectRef.Id)
		catcher.Add(err)
	}
	for _, buildDef := range h.newProjectRef.PeriodicBuilds {
		catcher.Wrapf(buildDef.Validate(), "invalid periodic build definition")
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "error validating triggers"))
	}

	newRevision := utility.FromStringPtr(h.apiNewProjectRef.Revision)
	if newRevision != "" {
		if err = h.sc.UpdateProjectRevision(h.project, newRevision); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		h.newProjectRef.RepotrackerError = &dbModel.RepositoryErrorDetails{
			Exists:            false,
			InvalidRevision:   "",
			MergeBaseRevision: "",
		}
	}

	if h.originalProject.Restricted != mergedProjectRef.Restricted {
		if mergedProjectRef.IsRestricted() {
			err = mergedProjectRef.MakeRestricted()
		} else {
			err = mergedProjectRef.MakeUnrestricted()
		}
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// if owner/repo has changed or we're toggling repo settings off, update scope
	if h.newProjectRef.Owner != h.originalProject.Owner || h.newProjectRef.Repo != h.originalProject.Repo ||
		(!h.newProjectRef.UseRepoSettings && h.originalProject.UseRepoSettings) {
		if err = h.newProjectRef.RemoveFromRepoScope(); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error removing project from old repo scope"))
		}
		h.newProjectRef.RepoRefId = "" // if using repo settings, will reassign this in the next block
	}
	if h.newProjectRef.UseRepoSettings && h.newProjectRef.RepoRefId == "" {
		if err = h.newProjectRef.AddToRepoScope(h.user); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// complete all updates
	if err = h.sc.UpdateProject(h.newProjectRef); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for update() by project id '%s'", h.project))
	}
	if err = h.sc.UpdateProjectVars(h.newProjectRef.Id, &h.apiNewProjectRef.Variables, false); err != nil { // destructively modifies h.apiNewProjectRef.Variables
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating variables for project '%s'", h.project))
	}
	if err = h.sc.UpdateProjectAliases(h.newProjectRef.Id, h.apiNewProjectRef.Aliases); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error updating aliases for project '%s'", h.project))
	}

	if err = h.sc.UpdateAdminRoles(h.newProjectRef, adminsToAdd, adminsToDelete); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error updating admins for project '%s'", h.project))
	}

	// Don't use Save to delete subscriptions, since we aren't checking the
	// delete subscriptions list against the inputted list of subscriptions.
	if err = h.sc.SaveSubscriptions(h.newProjectRef.Id, h.apiNewProjectRef.Subscriptions, true); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error saving subscriptions for project '%s'", h.project))
	}

	toDelete := []string{}
	for _, deleteSub := range h.apiNewProjectRef.DeleteSubscriptions {
		toDelete = append(toDelete, utility.FromStringPtr(deleteSub))
	}
	if err = h.sc.DeleteSubscriptions(h.newProjectRef.Id, toDelete); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error deleting subscriptions for project '%s'", h.project))
	}

	after, err := h.sc.GetProjectSettings(h.newProjectRef)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting ProjectSettings after update for project '%s'", h.project))
	}
	if err = dbModel.LogProjectModified(h.newProjectRef.Id, h.user.Username(), before, after); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error logging project modification for project '%s'", h.project))
	}

	// run the repotracker for the project
	if newRevision != "" {
		ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), h.newProjectRef.Id)

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
func hasAliasDefined(aliases []model.APIProjectAlias, alias string) bool {
	for _, a := range aliases {
		if utility.FromStringPtr(a.Alias) == alias {
			return true
		}
	}
	return false
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
	p, err := h.sc.FindProjectById(h.projectName, false, false)
	if err != nil && err.(gimlet.ErrorResponse).StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error for find() by project '%s'", h.projectName))
	}
	if p != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot create project with identifier '%s'", h.projectName),
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
	dbProjectRef.Identifier = h.projectName

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Cannot set HTTP status code to %d", http.StatusCreated))
	}
	u := gimlet.GetUser(ctx).(*user.DBUser)
	if err = h.sc.CreateProject(dbProjectRef, u); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "Database error for insert() project with project name '%s'", h.projectName))
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
	projectId, err := dbModel.GetIdForProject(h.projectName)
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
// DELETE /rest/v2/projects/{project_id}

type projectDeleteHandler struct {
	projectName string
	sc          data.Connector
}

func makeDeleteProject(sc data.Connector) gimlet.RouteHandler {
	return &projectDeleteHandler{
		sc: sc,
	}
}

func (h *projectDeleteHandler) Factory() gimlet.RouteHandler {
	return &projectDeleteHandler{
		sc: h.sc,
	}
}

func (h *projectDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	project, err := dbModel.FindBranchProjectRef(h.projectName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "project '%s' could not be found successfully", h.projectName))
	}
	if project == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' does not exist", h.projectName))
	}

	if project.IsHidden() {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' is already hidden", h.projectName))
	}

	if !project.UseRepoSettings {
		return gimlet.MakeJSONErrorResponder(
			errors.Errorf("project '%s' must have UseRepoSettings enabled to be eligible for deletion", h.projectName))
	}

	skeletonProj := dbModel.ProjectRef{
		Id:              project.Id,
		Owner:           project.Owner,
		Repo:            project.Repo,
		Branch:          project.Branch,
		RepoRefId:       project.RepoRefId,
		Enabled:         utility.FalsePtr(),
		UseRepoSettings: true,
		Hidden:          utility.TruePtr(),
	}
	if err = skeletonProj.Update(); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "project '%s' could not be updated", project.Id))
	}
	if err = h.sc.UpdateAdminRoles(project, nil, project.Admins); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error removing project auth for admins"))
	}

	projectAliases, err := dbModel.FindAliasesForProjectFromDb(project.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "project aliases for '%s' could not be found", project.Id))
	}

	for _, alias := range projectAliases {
		if err := dbModel.RemoveProjectAlias(alias.ID.Hex()); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(
				errors.Wrapf(err, "project alias '%s' for project '%s' could not be removed", alias.ID.Hex(), project.Id))
		}
	}

	skeletonProjVars := dbModel.ProjectVars{
		Id: project.Id,
	}
	if _, err := skeletonProjVars.Upsert(); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "project vars could not be updated for project '%s'", project.Id))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}

type projectIDGetHandler struct {
	projectName          string
	includeRepo          bool
	includeParserProject bool
	sc                   data.Connector
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
	h.includeRepo = r.URL.Query().Get("includeRepo") == "true"
	h.includeParserProject = r.URL.Query().Get("includeParserProject") == "true"
	return nil
}

func (h *projectIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	project, err := h.sc.FindProjectById(h.projectName, h.includeRepo, h.includeParserProject)
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

	// we pass the repoId through so we don't have to re-look up the project
	repoId := ""
	if h.includeRepo {
		repoId = project.RepoRefId
	}
	variables, err := h.sc.FindProjectVarsById(project.Id, repoId, true)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	projectModel.Variables = *variables
	if projectModel.Aliases, err = h.sc.FindProjectAliases(project.Id, repoId, nil); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if projectModel.Subscriptions, err = h.sc.GetSubscriptions(project.Id, event.OwnerTypeProject); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	return gimlet.NewJSONResponse(projectModel)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}/versions

const defaultVersionLimit = 20

type getProjectVersionsHandler struct {
	projectName string
	opts        dbModel.GetVersionsOptions

	sc data.Connector
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

	// body is optional
	b, _ := ioutil.ReadAll(r.Body)
	if len(b) > 0 {
		if err := json.Unmarshal(b, &h.opts); err != nil {
			return errors.Wrap(err, "error parsing request body")
		}
	}

	if h.opts.IncludeTasks && !h.opts.IncludeBuilds {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "can't include tasks without builds",
		}
	}

	// get some options from the query parameters for legacy usage
	limitStr := params.Get("limit")
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return errors.Wrap(err, "'limit' query parameter must be a valid integer")
		}
		h.opts.Limit = limit
	}
	if h.opts.Limit == 0 {
		h.opts.Limit = defaultVersionLimit
	}
	if h.opts.Limit < 1 {
		return errors.New("'limit' must be a positive integer")
	}

	startStr := params.Get("start")
	if startStr != "" {
		startOrder, err := strconv.Atoi(params.Get("start"))
		if err != nil {
			return errors.Wrap(err, "'start' query parameter must be a valid integer")
		}
		h.opts.StartAfter = startOrder
	}
	if h.opts.StartAfter < 0 {
		return errors.New("'start' must be a non-negative integer")
	}

	requester := params.Get("requester")
	if requester != "" {
		h.opts.Requester = requester
	}
	if h.opts.Requester == "" {
		h.opts.Requester = evergreen.RepotrackerVersionRequester
	}
	return nil
}

func (h *getProjectVersionsHandler) Run(ctx context.Context) gimlet.Responder {
	versions, err := h.sc.GetProjectVersionsWithOptions(h.projectName, h.opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error getting versions"))
	}

	resp, err := gimlet.NewBasicResponder(http.StatusOK, gimlet.JSON, versions)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "error constructing response"))
	}

	if len(versions) >= h.opts.Limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start",
				BaseURL:         h.sc.GetURL(),
				Key:             strconv.Itoa(versions[len(versions)-1].Order),
				Limit:           h.opts.Limit,
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
//    /projects/{project_id}/patch_trigger_aliases
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
	proj, err := dbModel.FindMergedProjectRef(p.projectID, "", true)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting project",
			"project": p.projectID,
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "unable to get project '%s'", p.projectID))
	}
	if proj == nil {
		return gimlet.NewJSONErrorResponse(errors.Errorf("project '%s' doesn't exist", p.projectID))
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
	id, err := dbModel.GetIdForProject(h.projectName)
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

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/projects/variables/rotate

type projectVarsPutInput struct {
	ToReplace   string `json:"to_replace"`
	Replacement string `json:"replacement"`
	DryRun      bool   `json:"dry_run"`
}

type projectVarsPutHandler struct {
	replaceVars *projectVarsPutInput
	user        *user.DBUser

	sc data.Connector
}

func makeProjectVarsPut(sc data.Connector) gimlet.RouteHandler {
	return &projectVarsPutHandler{
		sc: sc,
	}
}

func (h *projectVarsPutHandler) Factory() gimlet.RouteHandler {
	return &projectVarsPutHandler{
		sc: h.sc,
	}
}

// Parse fetches the project's identifier from the http request.
func (h *projectVarsPutHandler) Parse(ctx context.Context, r *http.Request) error {
	h.user = MustHaveUser(ctx)
	body := util.NewRequestReader(r)
	defer body.Close()
	b, err := ioutil.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	replacements := &projectVarsPutInput{}
	if err = json.Unmarshal(b, replacements); err != nil {
		return errors.Wrap(err, "API error while unmarshalling JSON")
	}
	if replacements.ToReplace == "" {
		return errors.New("'to_replace' parameter must be specified")
	}
	if replacements.Replacement == "" {
		return errors.New("'replacement' parameter must be specified")
	}
	h.replaceVars = replacements
	return nil
}

func (h *projectVarsPutHandler) Run(ctx context.Context) gimlet.Responder {
	res, err := h.sc.UpdateProjectVarsByValue(h.replaceVars.ToReplace, h.replaceVars.Replacement, h.user.Username(), h.replaceVars.DryRun)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err,
			"error updating projects with matching keys"))
	}
	return gimlet.NewJSONResponse(res)
}
