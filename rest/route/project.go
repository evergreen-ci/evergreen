package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type projectGetHandler struct {
	key        string
	limit      int
	user       *user.DBUser
	url        string
	ownerName  string
	repoName   string
	activeOnly bool
}

func makeFetchProjectsRoute() gimlet.RouteHandler {
	return &projectGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Fetch all projects
//	@Description	Returns a paginated list of all non-hidden projects. Any authenticated user can access this endpoint, so potentially sensitive information (variables, task annotation settings, workstation settings, and container secrets) is omitted. subscriptions.subscriber.target is undocumented by the OpenAPI spec, and can be either a string or an object.
//	@Tags			projects
//	@Router			/projects [get]
//	@Security		Api-User || Api-Key
//	@Param			start_at	query	string	false	"The project identifier to start at in the pagination"
//	@Param			limit		query	int		false	"The number of projects to be returned per page of pagination. Defaults to 100"
//	@Param			owner_name	query	string	false	"Filter projects by owner name (GitHub organization)"
//	@Param			repo_name	query	string	false	"Filter projects by repository name"
//	@Param			active		query	bool	false	"If true, only return projects that are currently enabled"
//	@Success		200			{array}	model.APIProjectRef
func (p *projectGetHandler) Factory() gimlet.RouteHandler {
	return &projectGetHandler{}
}

func (p *projectGetHandler) Parse(ctx context.Context, r *http.Request) error {
	p.user, _ = gimlet.GetUser(ctx).(*user.DBUser)

	vals := r.URL.Query()
	p.url = util.HttpsUrl(r.Host)

	p.key = vals.Get("start_at")
	p.ownerName = vals.Get("owner_name")
	p.repoName = vals.Get("repo_name")
	p.activeOnly = vals.Get("active") == "true"

	var err error
	p.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *projectGetHandler) Run(ctx context.Context) gimlet.Responder {
	projects, err := dbModel.FindNonHiddenProjects(ctx, p.key, p.limit+1, 1, p.ownerName, p.repoName, p.activeOnly)
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
	lastIndex := len(projects)
	if len(projects) > p.limit {
		lastIndex = p.limit

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         p.url,
				Key:             projects[p.limit].Identifier,
				Limit:           p.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	projects = projects[:lastIndex]
	for _, proj := range projects {
		projectModel := &model.APIProjectRef{}
		// Because this is route to accessible to non-admins, only return basic fields.
		if err = projectModel.BuildPublicFields(ctx, proj); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting project '%s' to API model", proj.Id))
		}
		if err = resp.AddData(projectModel); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for project '%s'", utility.FromStringPtr(projectModel.Id)))
		}
	}

	return resp
}

type legacyVersionsGetHandler struct {
	project string
	limit   int
	offset  int
}

func makeFetchProjectVersionsLegacy() gimlet.RouteHandler {
	return &legacyVersionsGetHandler{}
}

func (h *legacyVersionsGetHandler) Factory() gimlet.RouteHandler {
	return &legacyVersionsGetHandler{}
}

func (h *legacyVersionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.project = gimlet.GetVars(r)["project_id"]
	var query = r.URL.Query()

	limit := query.Get("limit")
	if limit != "" {
		h.limit, err = strconv.Atoi(limit)
		if err != nil {
			return errors.Wrap(err, "invalid limit")
		}
	} else {
		h.limit = 10
	}

	offset := query.Get("offset")
	if offset != "" {
		h.offset, err = strconv.Atoi(offset)
		if err != nil {
			return errors.Wrap(err, "invalid offset")
		}
	} else {
		h.offset = 0
	}

	return nil
}

func (h *legacyVersionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	projRefId, err := dbModel.GetIdForProject(ctx, h.project)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting ID for project '%s'", h.project))
	}

	_, proj, _, err := dbModel.FindLatestVersionWithValidProject(ctx, projRefId, false)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding latest version for project '%s'", projRefId))
	}

	versions, err := data.GetVersionsAndVariants(ctx, h.offset, h.limit, proj)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting versions and variants"))
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

	settings *evergreen.Settings
}

func makePatchProjectByID(settings *evergreen.Settings) gimlet.RouteHandler {
	return &projectIDPatchHandler{
		settings: settings,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Modify a project
//	@Description	Modify an existing project (restricted to project admins -- the fetch all projects route can be used for non-admins). Will enable webhooks if an enabled project, and enable PR testing and the commit queue if specified.  For lists, if there is a complementary "delete" field, then the former field indicates items to be added, while the "delete" field indicates items to be deleted. Otherwise, the given list will overwrite the original list (the only exception is for project variables -- we will ignore any empty project variables to avoid accidentally overwriting private variables).
//	@Tags			projects
//	@Router			/projects/{project_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path		string				true	"the project ID"
//	@Param			{object}	body		model.APIProjectRef	true	"parameters"
//	@Success		200			{object}	model.APIProjectRef
func (h *projectIDPatchHandler) Factory() gimlet.RouteHandler {
	return &projectIDPatchHandler{
		settings: h.settings,
	}
}

// Parse fetches the project's identifier from the http request.
func (h *projectIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	h.project = gimlet.GetVars(r)["project_id"]
	h.user = MustHaveUser(ctx)
	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading JSON request body")
	}

	oldProject, err := data.FindProjectById(ctx, h.project, false, false)
	if err != nil {
		return errors.Wrapf(err, "finding original project '%s'", h.project)
	}
	requestProjectRef := &model.APIProjectRef{}
	if err = requestProjectRef.BuildFromService(ctx, *oldProject); err != nil {
		return errors.Wrap(err, "converting original project to API model")
	}

	// erase contents so apiNewProjectRef will only be populated with new elements for these fields
	requestProjectRef.Admins = nil
	requestProjectRef.GitTagAuthorizedUsers = nil
	requestProjectRef.GitTagAuthorizedTeams = nil

	if err = json.Unmarshal(b, requestProjectRef); err != nil {
		return errors.Wrap(err, "unmarshalling modified project settings")
	}

	projectId := utility.FromStringPtr(requestProjectRef.Id)
	if projectId != oldProject.Id {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    "project ID is immutable",
		}
	}

	newProjectRef, err := requestProjectRef.ToService()
	if err != nil {
		return errors.Wrap(err, "converting new project to service model")
	}
	newProjectRef.RepoRefId = oldProject.RepoRefId // this can't be modified by users

	h.newProjectRef = newProjectRef
	h.originalProject = oldProject
	h.apiNewProjectRef = requestProjectRef // needed for the delete fields
	return nil
}

// Run updates a project by name.
func (h *projectIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	if h.newProjectRef.IsHidden() {
		return gimlet.NewJSONErrorResponse("can't patch a hidden project")
	}
	if err := h.newProjectRef.ValidateOwnerAndRepo(h.settings.GithubOrgs); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating owner and repo"))
	}
	if h.newProjectRef.Identifier != h.originalProject.Identifier {
		if err := h.newProjectRef.ValidateIdentifier(ctx); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating project identifier"))
		}
	}
	if err := h.newProjectRef.ValidateEnabledRepotracker(); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating project repotracker"))
	}

	before, err := dbModel.GetProjectSettings(ctx, h.newProjectRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting original project settings for project '%s'", h.newProjectRef.Identifier))
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
	mergedProjectRef, err := dbModel.GetProjectRefMergedWithRepo(ctx, *h.newProjectRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "merging project ref '%s' with repo settings", h.newProjectRef.Identifier))
	}

	if mergedProjectRef.Enabled {
		settings, err := evergreen.GetConfig(ctx)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting evergreen settings"))
		}
		_, err = dbModel.ValidateEnabledProjectsLimit(ctx, settings, h.originalProject, mergedProjectRef)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "validating project creation for project '%s'", h.newProjectRef.Identifier))
		}
	}

	if h.newProjectRef.Enabled {
		var hasHook bool
		hasHook, err = dbModel.SetTracksPushEvents(ctx, h.newProjectRef)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "setting project tracks push events for project '%s' in '%s/%s'", h.project, h.newProjectRef.Owner, h.newProjectRef.Repo))
		}

		var allAliases []model.APIProjectAlias
		if mergedProjectRef.AliasesNeeded() {
			allAliases, err = data.FindMergedProjectAliases(ctx, utility.FromStringPtr(h.apiNewProjectRef.Id), mergedProjectRef.RepoRefId, h.apiNewProjectRef.Aliases, false)
			if err != nil {
				return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "checking existing patch definitions for project '%s'", h.project))
			}
		}

		// verify enabling PR testing valid
		if mergedProjectRef.IsPRTestingEnabled() && !h.originalProject.IsPRTestingEnabled() {
			if !hasHook {
				return gimlet.MakeJSONErrorResponder(errors.New("cannot enable PR testing in this repo without first enabling GitHub webhooks"))
			}

			if !hasAliasDefined(allAliases, evergreen.GithubPRAlias) {
				return gimlet.MakeJSONErrorResponder(errors.New("cannot enable PR testing without a PR patch definition"))
			}

			if err = canEnablePRTesting(ctx, h.newProjectRef); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "enabling PR testing for project '%s'", h.project))
			}
		}

		// verify enabling github checks is valid
		if mergedProjectRef.IsGithubChecksEnabled() && !h.originalProject.IsGithubChecksEnabled() {
			if !hasAliasDefined(allAliases, evergreen.GithubChecksAlias) {
				return gimlet.MakeJSONErrorResponder(errors.New("cannot enable GitHub checks without a version definition"))
			}
		}

		// verify enabling git tag versions is valid
		if mergedProjectRef.IsGitTagVersionsEnabled() && !h.originalProject.IsGitTagVersionsEnabled() {
			if !hasAliasDefined(allAliases, evergreen.GitTagAlias) {
				return gimlet.MakeJSONErrorResponder(errors.New("cannot enable git tag versions without a version definition"))
			}
		}

		// verify enabling commit queue valid
		if mergedProjectRef.CommitQueue.IsEnabled() && !h.originalProject.CommitQueue.IsEnabled() {
			if !hasHook {
				return gimlet.MakeJSONErrorResponder(errors.New("cannot enable commit queue without first enabling GitHub webhooks"))
			}

			if !hasAliasDefined(allAliases, evergreen.CommitQueueAlias) {
				return gimlet.MakeJSONErrorResponder(errors.New("cannot enable commit queue without a commit queue patch definition"))
			}
			if err = canEnableCommitQueue(ctx, h.newProjectRef); err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "enabling commit queue for project '%s'", h.project))
			}
		}
	}

	// validate triggers before updating project
	catcher := grip.NewSimpleCatcher()
	for i := range h.newProjectRef.Triggers {
		catcher.Add(h.newProjectRef.Triggers[i].Validate(ctx, h.newProjectRef.Id))
	}
	for i := range h.newProjectRef.PatchTriggerAliases {
		h.newProjectRef.PatchTriggerAliases[i], err = dbModel.ValidateTriggerDefinition(ctx, h.newProjectRef.PatchTriggerAliases[i], h.newProjectRef.Id)
		catcher.Add(err)
	}
	for _, buildDef := range h.newProjectRef.PeriodicBuilds {
		catcher.Wrapf(buildDef.Validate(), "invalid periodic build definition")
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "invalid triggers"))
	}

	// Validate Parsley filters before updating project.
	err = parsley.ValidateFilters(h.newProjectRef.ParsleyFilters)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "invalid Parsley filters"))
	}

	err = dbModel.ValidateBbProject(ctx, h.newProjectRef.Id, h.newProjectRef.BuildBaronSettings, &h.newProjectRef.TaskAnnotationSettings.FileTicketWebhook)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating build baron config"))
	}

	newRevision := utility.FromStringPtr(h.apiNewProjectRef.Revision)
	if newRevision != "" {
		if err = dbModel.UpdateProjectRevision(ctx, h.project, newRevision); err != nil {
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
			err = mergedProjectRef.MakeRestricted(ctx)
		} else {
			err = mergedProjectRef.MakeUnrestricted(ctx)
		}
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// if owner/repo has changed and the project is attached to repo, update scope and repo accordingly
	if h.newProjectRef.UseRepoSettings() && h.ownerRepoChanged() {
		if err = h.newProjectRef.RemoveFromRepoScope(ctx); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "removing project from old repo scope"))
		}
		if err = h.newProjectRef.AddToRepoScope(ctx, h.user); err != nil { // will re-add using the new owner/repo
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	// complete all updates
	if err = h.newProjectRef.Replace(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating project '%s'", h.newProjectRef.Id))
	}

	if err = data.UpdateProjectVars(ctx, h.newProjectRef.Id, &h.apiNewProjectRef.Variables, false); err != nil { // destructively modifies h.apiNewProjectRef.Variables
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating variables for project '%s'", h.project))
	}
	if err = data.UpdateProjectAliases(ctx, h.newProjectRef.Id, h.apiNewProjectRef.Aliases); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating aliases for project '%s'", h.project))
	}

	if err = dbModel.UpdateAdminRoles(ctx, h.newProjectRef, adminsToAdd, adminsToDelete); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating admins for project '%s'", h.project))
	}

	// Don't use Save to delete subscriptions, since we aren't checking the
	// delete subscriptions list against the inputted list of subscriptions.
	if err = data.SaveSubscriptions(ctx, h.newProjectRef.Id, h.apiNewProjectRef.Subscriptions, true); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "saving subscriptions for project '%s'", h.project))
	}

	toDelete := []string{}
	for _, deleteSub := range h.apiNewProjectRef.DeleteSubscriptions {
		toDelete = append(toDelete, utility.FromStringPtr(deleteSub))
	}
	if err = data.DeleteSubscriptions(ctx, h.newProjectRef.Id, toDelete); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "deleting subscriptions for project '%s'", h.project))
	}

	after, err := dbModel.GetProjectSettings(ctx, h.newProjectRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting project settings after update for project '%s'", h.project))
	}
	if err = dbModel.LogProjectModified(ctx, h.newProjectRef.Id, h.user.Username(), before, after); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "logging modification event for project '%s'", h.project))
	}

	// run the repotracker for the project
	if newRevision != "" {
		ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), h.newProjectRef.Id)

		queue := evergreen.GetEnvironment().RemoteQueue()
		if err = amboy.EnqueueUniqueJob(ctx, queue, j); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "enqueueing catchup job"))
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

func (h projectIDPatchHandler) ownerRepoChanged() bool {
	return h.newProjectRef.Owner != h.originalProject.Owner || h.newProjectRef.Repo != h.originalProject.Repo
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

// canEnableCommitQueue determines if commit queue can be enabled for the given project.
func canEnableCommitQueue(ctx context.Context, projectRef *dbModel.ProjectRef) error {
	if ok, err := projectRef.CanEnableCommitQueue(ctx); err != nil {
		return errors.Wrap(err, "checking if commit queue can be enabled")
	} else if !ok {
		return errors.Errorf("cannot enable commit queue in this repo, must disable in other projects first")
	}

	return nil
}

// canEnablePRTesting determines if PR testing can be enabled for the given project.
func canEnablePRTesting(ctx context.Context, projectRef *dbModel.ProjectRef) error {
	conflicts, err := projectRef.GetGithubProjectConflicts(ctx)
	if err != nil {
		return errors.Wrap(err, "finding project refs with conflicting GitHub settings")
	}
	if len(conflicts.PRTestingIdentifiers) > 0 {
		return errors.Errorf("cannot enable PR testing in this repo, must disable in other projects first")

	}
	return nil
}

////////////////////////////////////////////////////////////////////////
//
// PUT /rest/v2/projects/{project_id}

type projectIDPutHandler struct {
	projectName string
	project     model.APIProjectRef
	body        []byte
	env         evergreen.Environment
}

func makePutProjectByID(env evergreen.Environment) gimlet.RouteHandler {
	return &projectIDPutHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Put a project
//	@Description	Create a new project with the given project ID. Restricted to super users.
//	@Tags			projects
//	@Router			/projects/{project_id} [put]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path		string				true	"the project ID"
//	@Param			{object}	body		model.APIProjectRef	false	"parameters"
//	@Success		200			{object}	model.APIProjectRef
func (h *projectIDPutHandler) Factory() gimlet.RouteHandler {
	return &projectIDPutHandler{env: h.env}
}

// Parse fetches the distroId and JSON payload from the http request.
func (h *projectIDPutHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]

	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	h.body = b

	apiProjectRef := model.APIProjectRef{}
	if err = json.Unmarshal(h.body, &apiProjectRef); err != nil {
		return errors.Wrap(err, "unmarshalling JSON request body into project ref")
	}
	h.project = apiProjectRef

	if utility.FromStringPtr(h.project.Owner) == "" || utility.FromStringPtr(h.project.Repo) == "" {
		return errors.New("Owner and repository must not be empty strings")
	}

	return nil
}

// Run creates a new resource based on the Request-URI and JSON payload and returns a http.StatusCreated (201)
func (h *projectIDPutHandler) Run(ctx context.Context) gimlet.Responder {
	p, err := data.FindProjectById(ctx, h.projectName, false, false)
	if err != nil && err.(gimlet.ErrorResponse).StatusCode != http.StatusNotFound {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", h.projectName))
	}
	if p != nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project with identifier '%s' already exists", h.projectName))
	}

	dbProjectRef := dbModel.ProjectRef{
		Identifier: h.projectName,
		Id:         utility.FromStringPtr(h.project.Id),
		Owner:      utility.FromStringPtr(h.project.Owner),
		Repo:       utility.FromStringPtr(h.project.Repo),
	}

	responder := gimlet.NewJSONResponse(struct{}{})
	if err = responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting response HTTP status code to %d", http.StatusCreated))
	}
	u := gimlet.GetUser(ctx).(*user.DBUser)

	if created, err := data.CreateProject(ctx, h.env, &dbProjectRef, u); err != nil && !created {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "creating project '%s'", h.projectName))
	}

	return responder
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/repotracker

type projectRepotrackerHandler struct {
	projectName string
}

func makeRunRepotrackerForProject() gimlet.RouteHandler {
	return &projectRepotrackerHandler{}
}

func (h *projectRepotrackerHandler) Factory() gimlet.RouteHandler {
	return &projectRepotrackerHandler{}
}

func (h *projectRepotrackerHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectRepotrackerHandler) Run(ctx context.Context) gimlet.Responder {
	projectId, err := dbModel.GetIdForProject(ctx, h.projectName)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "getting ID for project '%s'", h.projectName))
	}

	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("rest-%s", ts), projectId)

	queue := evergreen.GetEnvironment().RemoteQueue()
	if err := amboy.EnqueueUniqueJob(ctx, queue, j); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "enqueueing catchup job"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/projects/{project_id}

type projectDeleteHandler struct {
	projectName string
}

func makeDeleteProject() gimlet.RouteHandler {
	return &projectDeleteHandler{}
}

func (h *projectDeleteHandler) Factory() gimlet.RouteHandler {
	return &projectDeleteHandler{}
}

func (h *projectDeleteHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectDeleteHandler) Run(ctx context.Context) gimlet.Responder {
	if err := data.HideBranch(ctx, h.projectName); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}

type projectIDGetHandler struct {
	projectName          string
	includeRepo          bool
	includeProjectConfig bool
}

func makeGetProjectByID() gimlet.RouteHandler {
	return &projectIDGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get a project
//	@Description	Returns the project (restricted to project admins). Includes public variables, aliases, and subscriptions. Note that private variables are always redacted. If you want to use this to copy project variables, see instead the "Copy Project Variables" route.
//	@Tags			projects
//	@Router			/projects/{project_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id				path		string	true	"the project ID"
//	@Param			includeRepo				query		bool	false	"Setting to true will return the merged result of project and repo level settings. Defaults to false"
//	@Param			includeProjectConfig	query		bool	false	"Setting to true will return the merged result of the project and the config properties set in the project YAML. Defaults to false"
//	@Success		200						{object}	model.APIProjectRef
func (h *projectIDGetHandler) Factory() gimlet.RouteHandler {
	return &projectIDGetHandler{}
}

func (h *projectIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	h.includeRepo = r.URL.Query().Get("includeRepo") == "true"
	h.includeProjectConfig = r.URL.Query().Get("includeProjectConfig") == "true"
	return nil
}

func (h *projectIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	project, err := data.FindProjectById(ctx, h.projectName, h.includeRepo, h.includeProjectConfig)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", h.projectName))
	}
	if project == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("project '%s' not found", h.projectName))
	}

	projectModel := &model.APIProjectRef{}
	if err = projectModel.BuildFromService(ctx, *project); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "converting project '%s' to API model", h.projectName))
	}

	// we pass the repoId through so we don't have to re-look up the project
	repoId := ""
	if h.includeRepo {
		repoId = project.RepoRefId
	}
	variables, err := data.FindProjectVarsById(ctx, project.Id, repoId, true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding vars for project '%s'", project.Id))
	}
	projectModel.Variables = *variables
	if projectModel.Aliases, err = data.FindMergedProjectAliases(ctx, project.Id, repoId, nil, h.includeProjectConfig); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding aliases for project '%s'", project.Id))
	}
	if projectModel.Subscriptions, err = data.GetSubscriptions(ctx, project.Id, event.OwnerTypeProject); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting subscriptions for project '%s'", project.Id))
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
	url         string
}

func makeGetProjectVersionsHandler() gimlet.RouteHandler {
	return &getProjectVersionsHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get versions for a project
//	@Description	Returns a paginated list of recent versions for a project. Parameters should be passed into the JSON body (the route still accepts limit and start as query parameters to support legacy behavior).
//	@Tags			versions
//	@Router			/projects/{project_id}/versions [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id			path	string	true	"the project ID"
//	@Param			skip				query	int		false	"Number of versions to skip."
//	@Param			limit				query	int		false	"The number of versions to be returned per page of pagination. Defaults to 20."
//	@Param			start				query	int		false	"The version order number to start at, for pagination. Will return the versions that are less than (and therefore older) the revision number specified."
//	@Param			revision_end		query	int		false	"Will return the versions that are greater than (and therefore more recent) or equal to revision number specified."
//	@Param			requester			query	string	false	"Returns versions for this requester only. Defaults to gitter_request (caused by git commit, aka the repotracker requester). Can also be set to patch_request, github_pull_request, trigger_request (Project Trigger versions) , github_merge_request (GitHub merge queue),, and ad_hoc (periodic builds)."
//	@Param			include_builds		query	bool	false	"If set, will return some information for each build in the version."
//	@Param			by_build_variant	query	string	false	"If set, will only include information for this build, and only return versions with this build activated. Must have include_builds set."
//	@Param			include_tasks		query	bool	false	"If set, will return some information for each task in the included builds. This is only allowed if include_builds is set."
//	@Param			by_task				query	string	false	"If set, will only include information for this task, and will only return versions with this task activated. Must have include_tasks set."
//	@Param			created_after		query	string	false	"Timestamp to look for applicable versions after or equal to create_time."
//	@Param			created_before		query	string	false	"Timestamp to look for applicable versions before or equal to create_time."
//	@Success		200					{array}	model.APIVersion
func (h *getProjectVersionsHandler) Factory() gimlet.RouteHandler {
	return &getProjectVersionsHandler{}
}

func (h *getProjectVersionsHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	params := r.URL.Query()
	h.url = util.HttpsUrl(r.Host)

	// body is optional
	b, _ := io.ReadAll(r.Body)
	if len(b) > 0 {
		if err := json.Unmarshal(b, &h.opts); err != nil {
			return errors.Wrap(err, "unmarshalling JSON request body into version options")
		}
	}

	if h.opts.IncludeTasks && !h.opts.IncludeBuilds {
		return errors.New("cannot include tasks without builds")
	}

	// get some options from the query parameters for legacy usage
	limitStr := params.Get("limit")
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return errors.Wrap(err, "invalid limit")
		}
		h.opts.Limit = limit
	}
	if h.opts.Limit == 0 {
		h.opts.Limit = defaultVersionLimit
	}
	if h.opts.Limit < 1 {
		return errors.New("limit must be a positive integer")
	}

	startStr := params.Get("start")
	if startStr != "" {
		startOrder, err := strconv.Atoi(params.Get("start"))
		if err != nil {
			return errors.Wrap(err, "invalid start query parameter")
		}
		h.opts.Start = startOrder
	}
	if h.opts.Start < 0 {
		return errors.New("start must be a non-negative integer")
	}

	if h.opts.RevisionEnd < 0 {
		return errors.New("revision_end must be a non-negative integer")
	}

	createdAfterStr := params.Get("created_after")
	if createdAfterStr != "" {
		createdAfter, err := model.ParseTime(createdAfterStr)
		if err != nil {
			return errors.Wrap(err, "invalid created_after timestamp")
		}
		h.opts.CreatedAfter = createdAfter
	}
	createdBeforeStr := params.Get("created_before")
	if createdBeforeStr != "" {
		createdBefore, err := model.ParseTime(createdBeforeStr)
		if err != nil {
			return errors.Wrap(err, "invalid created_before timestamp")
		}
		h.opts.CreatedBefore = createdBefore
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
	versions, err := data.GetProjectVersionsWithOptions(ctx, h.projectName, h.opts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting versions for project '%s'", h.projectName))
	}

	resp, err := gimlet.NewBasicResponder(http.StatusOK, gimlet.JSON, versions)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "constructing response"))
	}

	if len(versions) >= h.opts.Limit {
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start",
				BaseURL:         h.url,
				Key:             strconv.Itoa(versions[len(versions)-1].Order),
				Limit:           h.opts.Limit,
			},
		})

		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	return resp
}

// POST /rest/v2/projects/{project_id}/versions

// modifyProjectVersionsHandler is a RequestHandler for setting the priority of versions.
type modifyProjectVersionsHandler struct {
	projectId string
	url       string
	opts      dbModel.ModifyVersionsOptions
	startTime time.Time
	endTime   time.Time
}

func makeModifyProjectVersionsHandler(url string) gimlet.RouteHandler {
	return &modifyProjectVersionsHandler{url: url}
}

// Factory creates an instance of the handler.
//
//	@Summary		Modify versions for a project
//	@Description	Modifies a group of versions for a project. Parameters should be passed into the JSON body. Currently supports setting priority for all versions that the given options apply to. This route is restricted to project admins.
//	@Tags			versions
//	@Router			/projects/{project_id}/versions [patch]
//	@Security		Api-User || Api-Key
//	@Param			project_id			path	string	true	"the project ID"
//	@Param			start_time_str		query	string	true	"Timestamp to start looking for applicable versions."
//	@Param			end_time_str		query	string	false	"Timestamp to stop looking for applicable versions."
//	@Param			revision_start		query	int		false	"The version order number to start at."
//	@Param			revision_end		query	int		false	"The version order number to end at."
//	@Param			priority			query	int		true	"Priority to set for all tasks within applicable versions."
//	@Param			requester			query	string	false	"Returns versions for this requester only. Defaults to gitter_request (caused by git commit, aka the repotracker requester). Can also be set to patch_request, github_pull_request, trigger_request (Project Trigger versions) , github_merge_request (GitHub merge queue), and ad_hoc (periodic builds)."
//	@Param			by_build_variant	query	string	false	"If set, will only include information for this build, and only return versions with this build activated. Must have include_builds set."
//	@Param			by_task				query	string	false	"If set, will only include information for this task, and will only return versions with this task activated. Must have include_tasks set."
//	@Success		200
func (h *modifyProjectVersionsHandler) Factory() gimlet.RouteHandler {
	return &modifyProjectVersionsHandler{url: h.url}
}

func (h *modifyProjectVersionsHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectId = gimlet.GetVars(r)["project_id"]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "reading request body")
	}
	opts := &dbModel.ModifyVersionsOptions{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, opts); err != nil {
			return errors.Wrap(err, "unmarshalling JSON request body into version options")
		}
	}
	if opts.RevisionStart < 0 || opts.RevisionEnd < 0 {
		return errors.New("both start and end must be non-negative integers")
	}
	if opts.RevisionEnd > opts.RevisionStart {
		return errors.New("end must be less than or equal to start")
	}

	if (opts.RevisionStart > 0 || opts.RevisionEnd > 0) && (opts.StartTimeStr != "" || opts.EndTimeStr != "") {
		return errors.New("cannot specify both timestamps and order numbers")
	}

	if opts.StartTimeStr != "" && opts.RevisionStart != 0 {
		return errors.New("cannot specify both timestamps and order numbers")
	}

	if opts.StartTimeStr == "" && opts.RevisionStart == 0 {
		return errors.New("must specify either timestamps or order numbers")
	}
	if opts.Priority == nil {
		return errors.New("must specify a priority")
	}
	h.opts = *opts
	if h.opts.StartTimeStr != "" {
		h.startTime, err = model.ParseTime(h.opts.StartTimeStr)
		if err != nil {
			return errors.Wrap(err, "parsing start time")
		}
		h.endTime, err = model.ParseTime(h.opts.EndTimeStr)
		if err != nil {
			return errors.Wrap(err, "parsing end time")
		}
	}
	return nil
}

func (h *modifyProjectVersionsHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	priority := utility.FromInt64Ptr(h.opts.Priority)
	// Check for a valid priority and perform the update.
	if ok := validPriority(ctx, priority, h.projectId, user); !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message: fmt.Sprintf("insufficient privilege to set priority to %d, "+
				"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
			StatusCode: http.StatusForbidden,
		})
	}
	versions, err := dbModel.GetVersionsToModify(ctx, h.projectId, h.opts, h.startTime, h.endTime)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting versions for project '%s'", h.projectId))
	}
	var versionIds []string
	for _, v := range versions {
		versionIds = append(versionIds, v.Id)
	}
	if err = dbModel.SetVersionsPriority(ctx, versionIds, priority, user.Id); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting version priorities"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}/tasks/{task_id}

type getProjectTasksHandler struct {
	projectName string
	taskName    string
	url         string

	opts model.GetProjectTasksOpts
}

func makeGetProjectTasksHandler(url string) gimlet.RouteHandler {
	return &getProjectTasksHandler{url: url}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get tasks for a project
//	@Description	Returns the last set number of completed tasks that exist for a given project. Parameters should be passed into the JSON body. Ensure that a task name rather than a task ID is passed into the URL.
//	@Tags			tasks
//	@Router			/projects/{project_id}/tasks/{task_name} [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path	string						true	"the project ID"
//	@Param			task_name	path	string						true	"the task name"
//	@Param			{object}	body	model.GetProjectTasksOpts	false	"parameters"
//	@Success		200			{array}	model.APITask
func (h *getProjectTasksHandler) Factory() gimlet.RouteHandler {
	return &getProjectTasksHandler{url: h.url}
}

func (h *getProjectTasksHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	h.taskName = gimlet.GetVars(r)["task_name"]
	// body is optional
	b, _ := io.ReadAll(r.Body)
	if len(b) > 0 {
		if err := json.Unmarshal(b, &h.opts); err != nil {
			return errors.Wrap(err, "reading project task options from JSON request body")
		}
	}
	if h.opts.Limit < 0 {
		return errors.New("number of versions must be a positive integer")
	}
	if h.opts.Limit == 0 {
		h.opts.Limit = defaultVersionLimit
	}
	if h.opts.StartAt < 0 {
		return errors.New("start must be a non-negative integer")
	}
	for _, requester := range h.opts.Requesters {
		if !utility.StringSliceContains(evergreen.AllRequesterTypes, requester) {
			return errors.Errorf("'%s' is not a valid requester type", requester)
		}
	}

	return nil
}

func (h *getProjectTasksHandler) Run(ctx context.Context) gimlet.Responder {
	tasks, err := data.GetProjectTasksWithOptions(ctx, h.projectName, h.taskName, dbModel.GetProjectTasksOpts{
		Limit:        h.opts.Limit,
		BuildVariant: h.opts.BuildVariant,
		StartAt:      h.opts.StartAt,
		Requesters:   h.opts.Requesters,
	})
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting tasks for project '%s' and task '%s'", h.projectName, h.taskName))
	}

	return gimlet.NewJSONResponse(tasks)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/projects/{project_id}/task_executions

type getProjectTaskExecutionsHandler struct {
	projectName string
	opts        model.GetProjectTaskExecutionReq

	startTime time.Time
	endTime   time.Time
	projectId string
}

func makeGetProjectTaskExecutionsHandler() gimlet.RouteHandler {
	return &getProjectTaskExecutionsHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get execution info for a task
//	@Description	Right now, this returns the number of times the given task has executed (i.e. succeeded or failed). Parameters should be passed into the JSON body.
//	@Tags			tasks
//	@Router			/projects/{project_id}/task_executions [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id		path		string							true	"the project ID"
//	@Param			task_name		query		string							true	"The task to return execution info for."
//	@Param			build_variant	query		string							true	"The build variant to return task execution info for."
//	@Param			start_time		query		string							true	"Will only return execution info after this time. Format should be 2022-12-01T12:30:00.000Z"
//	@Param			end_time		query		string							false	"If not provided, will default to the current time."
//	@Param			requesters		query		[]string						false	"If not provided, will default to gitter_request (versions created by git commit). Can also be github_pull_request, trigger_request (Project Trigger versions) , github_merge_request (GitHub merge queue), or ad_hoc (periodic builds)"
//	@Success		200				{object}	model.ProjectTaskExecutionResp	"number completed"
func (h *getProjectTaskExecutionsHandler) Factory() gimlet.RouteHandler {
	return &getProjectTaskExecutionsHandler{}
}

func (h *getProjectTaskExecutionsHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()

	var err error
	if err = utility.ReadJSON(r.Body, &h.opts); err != nil {
		return errors.Wrap(err, "reading from JSON request body")
	}

	if h.opts.BuildVariant == "" || h.opts.TaskName == "" {
		return errors.New("'build_variant' and 'task_name' are required")
	}
	h.startTime, err = model.ParseTime(h.opts.StartTime)
	if err != nil {
		return errors.Wrapf(err, "parsing 'start_time' %s", h.opts.StartTime)
	}

	// End time isn't required, since we default to getting up to the current moment.
	if h.opts.EndTime != "" {
		h.endTime, err = model.ParseTime(h.opts.EndTime)
		if err != nil {
			return errors.Wrapf(err, "parsing 'end_time' %s", h.opts.EndTime)
		}
		if h.startTime.After(h.endTime) {
			return errors.New("'start_time' must be after 'end_time'")
		}
	}

	h.projectName = gimlet.GetVars(r)["project_id"]
	h.projectId, err = dbModel.GetIdForProject(ctx, h.projectName)
	if err != nil {
		return errors.Wrap(err, "getting id for project")
	}

	for _, requester := range h.opts.Requesters {
		if !utility.StringSliceContains(evergreen.AllRequesterTypes, requester) {
			return errors.Errorf("'%s' is not a valid requester type", requester)
		}
	}

	return nil
}

func (h *getProjectTaskExecutionsHandler) Run(ctx context.Context) gimlet.Responder {
	input := task.NumExecutionsForIntervalInput{
		ProjectId:    h.projectId,
		BuildVarName: h.opts.BuildVariant,
		TaskName:     h.opts.TaskName,
		StartTime:    h.startTime,
		EndTime:      h.endTime,
	}
	numTasks, err := task.CountNumExecutionsForInterval(ctx, input)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(err)
	}
	return gimlet.NewJSONResponse(model.ProjectTaskExecutionResp{
		NumCompleted: numTasks,
	})
}

type GetProjectAliasResultsHandler struct {
	version             string
	alias               string
	includeDependencies bool
}

func makeGetProjectAliasResultsHandler() gimlet.RouteHandler {
	return &GetProjectAliasResultsHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Check project alias results
//	@Description	Checks a specified project alias in a specified project against an Evergreen configuration, returning the tasks and variants that alias would select. Currently only supports passing in the configuration via an already-created version.
//	@Tags			projects
//	@Router			/projects/test_alias [get]
//	@Security		Api-User || Api-Key
//	@Param			version			query		string	true	"version"
//	@Param			alias			query		string	true	"alias"
//	@Param			include_deps	query		bool	false	"include dependencies"
//	@Success		200				{object}	model.APIVariantTasks
func (p *GetProjectAliasResultsHandler) Factory() gimlet.RouteHandler {
	return &GetProjectAliasResultsHandler{}
}

func (p *GetProjectAliasResultsHandler) Parse(ctx context.Context, r *http.Request) error {
	params := r.URL.Query()

	p.version = params.Get("version")
	if p.version == "" {
		return errors.New("version parameter must be specified")
	}
	p.alias = params.Get("alias")
	if p.alias == "" {
		return errors.New("alias parameter must be specified")
	}
	p.includeDependencies = params.Get("include_deps") == "true"

	return nil
}

func (p *GetProjectAliasResultsHandler) Run(ctx context.Context) gimlet.Responder {
	proj, err := dbModel.FindProjectFromVersionID(ctx, p.version)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting project for version",
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("getting project for version '%s'", p.version))
	}
	variantTasks, err := data.GetProjectAliasResults(ctx, proj, p.alias, p.includeDependencies)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting variants/tasks from for project '%s' and alias '%s'", proj.Identifier, p.alias))
	}

	return gimlet.NewJSONResponse(variantTasks)
}

// //////////////////////////////////////////////////////////////////////
//
// Handler for the patch trigger aliases defined for project
//
//	/projects/{project_id}/patch_trigger_aliases
type GetPatchTriggerAliasHandler struct {
	projectID string
}

func makeFetchPatchTriggerAliases() gimlet.RouteHandler {
	return &GetPatchTriggerAliasHandler{}
}

func (p *GetPatchTriggerAliasHandler) Factory() gimlet.RouteHandler {
	return &GetPatchTriggerAliasHandler{}
}

func (p *GetPatchTriggerAliasHandler) Parse(ctx context.Context, r *http.Request) error {
	p.projectID = gimlet.GetVars(r)["project_id"]
	return nil
}

func (p *GetPatchTriggerAliasHandler) Run(ctx context.Context) gimlet.Responder {
	proj, err := dbModel.FindMergedProjectRef(ctx, p.projectID, "", true)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting project",
			"project": p.projectID,
		}))
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", p.projectID))
	}
	if proj == nil {
		return gimlet.NewJSONErrorResponse(errors.Errorf("project '%s' not found", p.projectID))
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
}

func makeFetchParameters() gimlet.RouteHandler {
	return &projectParametersGetHandler{}
}

// Factory creates an instance of the handler.
//
//	@Summary		Get current parameters for a project
//	@Description	Returns a list of parameters for the project.
//	@Tags			projects
//	@Router			/projects/{project_id}/parameters [get]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path	string	true	"the project ID"
//	@Success		200			{array}	model.APIParameterInfo
func (h *projectParametersGetHandler) Factory() gimlet.RouteHandler {
	return &projectParametersGetHandler{}
}

func (h *projectParametersGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectName = gimlet.GetVars(r)["project_id"]
	return nil
}

func (h *projectParametersGetHandler) Run(ctx context.Context) gimlet.Responder {
	id, err := dbModel.GetIdForProject(ctx, h.projectName)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "getting ID for project '%s'", h.projectName))
	}
	_, p, _, err := dbModel.FindLatestVersionWithValidProject(ctx, id, false)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err,
			"finding project config for project '%s'", id))
	}
	if p == nil {
		return gimlet.NewJSONErrorResponse(errors.Errorf("project '%s' not found", id))
	}

	// convert to API structure
	res := make([]model.APIParameterInfo, len(p.Parameters))
	for i, param := range p.Parameters {
		var apiParam model.APIParameterInfo
		apiParam.BuildFromService(param)
		res[i] = apiParam
	}

	return gimlet.NewJSONResponse(res)
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/projects/{project_id}/backstage_variables

type backstageVariablesPostHandler struct {
	projectID string
	userID    string
	opts      backstageProjectVarsPostOptions
}

type backstageProjectVarsPostOptions struct {
	// Names of Backstage project variables to add/modify.
	Vars []backstageProjectVar `json:"vars"`
	// Names of Backstage project variables to delete.
	VarsToDelete []string `json:"vars_to_delete"`
}

type backstageProjectVar struct {
	// The name of the Backstage project variable.
	Name string `json:"name"`
	// The value of the Backstage project variable.
	Value string `json:"value"`
}

func makeBackstageVariablesPost() gimlet.RouteHandler {
	return &backstageVariablesPostHandler{}
}

// This route is intentionally not documented because only Backstage has access
// to it, not general users.
func (p *backstageVariablesPostHandler) Factory() gimlet.RouteHandler {
	return &backstageVariablesPostHandler{}
}

func (p *backstageVariablesPostHandler) Parse(ctx context.Context, r *http.Request) error {
	p.projectID = gimlet.GetVars(r)["project_id"]
	u := MustHaveUser(ctx)
	p.userID = u.Username()

	if err := utility.ReadJSON(r.Body, &p.opts); err != nil {
		return errors.Wrap(err, "reading request body")
	}

	// Only allow a few reserved variable names to be managed by Backstage so it
	// can't modify any arbitrary variable.
	allowedBackstageVariableNames := []string{
		"__default_bucket",
		"__default_bucket_role_arn",
	}
	for _, projVar := range p.opts.Vars {
		name := projVar.Name
		if !utility.StringSliceContains(allowedBackstageVariableNames, name) {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusForbidden,
				Message:    fmt.Sprintf("project variable '%s' cannot be modified by Backstage", name),
			}
		}
		if projVar.Value == "" {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("project variable '%s' cannot have an empty value", name),
			}
		}
	}
	for _, name := range p.opts.VarsToDelete {
		if !utility.StringSliceContains(allowedBackstageVariableNames, name) {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusForbidden,
				Message:    fmt.Sprintf("project variable '%s' cannot be deleted by Backstage", name),
			}
		}
	}
	if len(p.opts.Vars) == 0 && len(p.opts.VarsToDelete) == 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must specify at least one project variable to add or delete",
		}
	}

	return nil
}

func (h *backstageVariablesPostHandler) Run(ctx context.Context) gimlet.Responder {
	// This intentionally looks up the project ref without merging with its repo
	// ref because the route is supposed to modify solely the branch project's
	// own variables.
	pRef, err := dbModel.FindBranchProjectRef(ctx, h.projectID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding branch project ref '%s'", h.projectID))
	}
	if pRef == nil {
		// Fall back to checking if the project ref ID is actually a repo ref
		// ID.
		repoRef, err := dbModel.FindOneRepoRef(ctx, h.projectID)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding repo ref '%s'", h.projectID))
		}
		if repoRef == nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("project or repo ref '%s' not found", h.projectID),
			})
		}
		pRef = &repoRef.ProjectRef
	}
	before, err := dbModel.GetProjectSettings(ctx, pRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting project settings for project '%s'", h.projectID))
	}
	if before == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project settings for project '%s' not found", h.projectID),
		})
	}

	updatedVars, err := dbModel.FindOneProjectVars(ctx, pRef.Id)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding variables for project '%s'", pRef.Id))
	}
	if updatedVars == nil {
		updatedVars = &dbModel.ProjectVars{
			Id:   pRef.Id,
			Vars: map[string]string{},
		}
	}
	for _, projVar := range h.opts.Vars {
		updatedVars.Vars[projVar.Name] = projVar.Value
	}
	for _, name := range h.opts.VarsToDelete {
		delete(updatedVars.Vars, name)
	}
	if maps.Equal(updatedVars.Vars, before.Vars.Vars) {
		// No-op, no changes were made.
		return gimlet.NewJSONResponse(struct{}{})
	}

	if _, err := updatedVars.Upsert(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "upserting new vars for project '%s'", pRef.Id))
	}

	after, err := dbModel.GetProjectSettings(ctx, pRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting updated project settings for project '%s'", pRef.Id))
	}

	if err := dbModel.LogProjectModified(ctx, pRef.Id, h.userID, before, after); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "logging project settings modification for project '%s'", pRef.Id))
	}

	return gimlet.NewJSONResponse(struct{}{})
}
