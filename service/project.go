package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// filterViewableProjects iterates through a list of projects and returns a list of all the projects that a user
// is authorized to view
func (uis *UIServer) filterViewableProjects(u gimlet.User) ([]model.ProjectRef, error) {
	allProjects, err := model.FindAllMergedProjectRefs()
	if err != nil {
		return nil, err
	}
	authorizedProjects := []model.ProjectRef{}
	// only returns projects for which the user is authorized to see.
	for _, project := range allProjects {
		if hasViewPermission(u, &project) {
			authorizedProjects = append(authorizedProjects, project)
		}
	}
	return authorizedProjects, nil

}

func (uis *UIServer) projectsPage(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)

	allProjects, err := uis.filterViewableProjects(dbUser)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	opts := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionProjectCreate,
		RequiredLevel: evergreen.ProjectCreate.Value,
	}
	canCreate := dbUser.HasPermission(opts)

	data := struct {
		AllProjects []model.ProjectRef
		CanCreate   bool
		ViewData
	}{allProjects, canCreate, uis.GetCommonViewData(w, r, true, true)}

	uis.render.WriteResponse(w, http.StatusOK, data, "base", "projects.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) projectPage(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveProjectContext(r)
	u := MustHaveUser(r)

	id := gimlet.GetVars(r)["project_id"]

	projRef, err := model.FindOneProjectRef(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if projRef == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound,
			gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("project '%s' is not found", id),
			})
		return
	}

	projVars, err := model.FindOneProjectVars(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	projVars = projVars.RedactPrivateVars()

	projectAliases, err := model.FindAliasesForProject(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	matchingRefs, err := model.FindMergedProjectRefsByRepoAndBranch(projRef.Owner, projRef.Repo, projRef.Branch)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	PRConflictingRefs := []string{}
	CQConflictingRefs := []string{}
	for _, ref := range matchingRefs {
		if ref.PRTestingEnabled && ref.Id != projRef.Id {
			PRConflictingRefs = append(PRConflictingRefs, ref.Id)
		}
		if ref.CommitQueue.Enabled && ref.Id != projRef.Id {
			CQConflictingRefs = append(CQConflictingRefs, ref.Id)
		}
	}

	var hook *model.GithubHook
	if projRef.Owner != "" && projRef.Repo != "" {
		hook, err = model.FindGithubHook(projRef.Owner, projRef.Repo)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	subscriptions, err := event.FindSubscriptionsByOwner(projRef.Id, event.OwnerTypeProject)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	apiSubscriptions := make([]restModel.APISubscription, len(subscriptions))
	for i := range subscriptions {
		if err = apiSubscriptions[i].BuildFromService(subscriptions[i]); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}
	opts := gimlet.PermissionOpts{Resource: projRef.Id, ResourceType: evergreen.ProjectResourceType}
	permissions, err := rolemanager.HighestPermissionsForRoles(u.Roles(), evergreen.GetEnvironment().RoleManager(), opts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := struct {
		ProjectRef            *model.ProjectRef
		ProjectVars           *model.ProjectVars
		ProjectAliases        []model.ProjectAlias        `json:"aliases,omitempty"`
		PRConflictingRefs     []string                    `json:"pr_testing_conflicting_refs,omitempty"`
		CQConflictingRefs     []string                    `json:"commit_queue_conflicting_refs,omitempty"`
		GitHubWebhooksEnabled bool                        `json:"github_webhooks_enabled"`
		GithubValidOrgs       []string                    `json:"github_valid_orgs"`
		Subscriptions         []restModel.APISubscription `json:"subscriptions"`
		Permissions           gimlet.Permissions          `json:"permissions"`
	}{projRef, projVars, projectAliases, PRConflictingRefs, CQConflictingRefs, hook != nil, settings.GithubOrgs, apiSubscriptions, permissions}

	// the project context has all projects so make the ui list using all projects
	gimlet.WriteJSON(w, data)
}

// ProjectNotFound calls WriteHTML with the invalid-project page. It should be called whenever the
// project specified by the user does not exist, or when there are no projects at all.
func (uis *UIServer) ProjectNotFound(w http.ResponseWriter, r *http.Request) {
	uis.projectNotFoundBase(w, r, uis.GetCommonViewData(w, r, false, false))
}

func (uis *UIServer) projectNotFoundBase(w http.ResponseWriter, r *http.Request, data interface{}) {
	uis.render.WriteResponse(w, http.StatusNotFound, data, "base", "invalid_project.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyProject(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)
	_ = MustHaveProjectContext(r)
	id := gimlet.GetVars(r)["project_id"]

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	projectRef, err := model.FindOneProjectRef(id)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if projectRef == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	origProjectRef := *projectRef

	responseRef := struct {
		Identifier              string                         `json:"identifier"`
		DisplayName             string                         `json:"display_name"`
		RemotePath              string                         `json:"remote_path"`
		SpawnHostScriptPath     string                         `json:"spawn_host_script_path"`
		BatchTime               int                            `json:"batch_time"`
		DeactivatePrevious      bool                           `json:"deactivate_previous"`
		Branch                  string                         `json:"branch_name"`
		ProjVarsMap             map[string]string              `json:"project_vars"`
		GitHubAliases           []model.ProjectAlias           `json:"github_aliases"`
		CommitQueueAliases      []model.ProjectAlias           `json:"commit_queue_aliases"`
		PatchAliases            []model.ProjectAlias           `json:"patch_aliases"`
		GitTagAliases           []model.ProjectAlias           `json:"git_tag_aliases"`
		DeleteAliases           []string                       `json:"delete_aliases"`
		DefaultLogger           string                         `json:"default_logger"`
		CedarTestResultsEnabled bool                           `json:"cedar_test_results_enabled"`
		PrivateVars             map[string]bool                `json:"private_vars"`
		RestrictedVars          map[string]bool                `json:"restricted_vars"`
		Enabled                 bool                           `json:"enabled"`
		Private                 bool                           `json:"private"`
		Restricted              bool                           `json:"restricted"`
		Owner                   string                         `json:"owner_name"`
		Repo                    string                         `json:"repo_name"`
		Admins                  []string                       `json:"admins"`
		GitTagAuthorizedUsers   []string                       `json:"git_tag_authorized_users"`
		GitTagAuthorizedTeams   []string                       `json:"git_tag_authorized_teams"`
		PRTestingEnabled        bool                           `json:"pr_testing_enabled"`
		GitTagVersionsEnabled   bool                           `json:"git_tag_versions_enabled"`
		UseRepoSettings         bool                           `json:"use_repo_settings"`
		CommitQueue             restModel.APICommitQueueParams `json:"commit_queue"`
		TaskSync                restModel.APITaskSyncOptions   `json:"task_sync"`
		PatchingDisabled        bool                           `json:"patching_disabled"`
		RepotrackerDisabled     bool                           `json:"repotracker_disabled"`
		DispatchingDisabled     bool                           `json:"dispatching_disabled"`
		AlertConfig             map[string][]struct {
			Provider string                 `json:"provider"`
			Settings map[string]interface{} `json:"settings"`
		} `json:"alert_config"`
		NotifyOnBuildFailure  bool                             `json:"notify_on_failure"`
		ForceRepotrackerRun   bool                             `json:"force_repotracker_run"`
		Subscriptions         []restModel.APISubscription      `json:"subscriptions"`
		DeleteSubscriptions   []string                         `json:"delete_subscriptions"`
		Triggers              []model.TriggerDefinition        `json:"triggers"`
		PatchTriggerAliases   []patch.PatchTriggerDefinition   `json:"patch_trigger_aliases"`
		FilesIgnoredFromCache []string                         `json:"files_ignored_from_cache"`
		DisabledStatsCache    bool                             `json:"disabled_stats_cache"`
		PeriodicBuilds        []*model.PeriodicBuildDefinition `json:"periodic_builds"`
		WorkstationConfig     restModel.APIWorkstationConfig   `json:"workstation_config"`
	}{}

	if err = utility.ReadJSON(util.NewRequestReader(r), &responseRef); err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request body %v", err), http.StatusInternalServerError)
		return
	}

	if len(responseRef.Owner) == 0 || len(responseRef.Repo) == 0 {
		http.Error(w, "no owner/repo specified", http.StatusBadRequest)
		return
	}
	if len(responseRef.Branch) == 0 {
		http.Error(w, "no branch specified", http.StatusBadRequest)
		return
	}

	if len(uis.Settings.GithubOrgs) > 0 && !utility.StringSliceContains(uis.Settings.GithubOrgs, responseRef.Owner) {
		http.Error(w, "owner not validated in settings", http.StatusBadRequest)
		return
	}

	errs := []string{}
	errs = append(errs, model.ValidateProjectAliases(responseRef.GitHubAliases, "GitHub Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(responseRef.CommitQueueAliases, "Commit Queue Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(responseRef.PatchAliases, "Patch Aliases")...)
	errs = append(errs, model.ValidateProjectAliases(responseRef.GitTagAliases, "Git Tag Aliases")...)
	if len(errs) > 0 {
		errMsg := ""
		for _, err := range errs {
			errMsg += err + ", "
		}
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New(errMsg))

		return
	}

	var hook *model.GithubHook
	hook, err = model.FindGithubHook(responseRef.Owner, responseRef.Repo)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	origGithubWebhookEnabled := (hook != nil)
	if hook == nil {
		hook, err = model.SetupNewGithubHook(ctx, uis.Settings, responseRef.Owner, responseRef.Repo)
		if err == nil || strings.Contains(err.Error(), "Hook already exists on this repository") {
			if err != nil {
				hook, err = model.GetExistingGithubHook(ctx, uis.Settings, responseRef.Owner, responseRef.Repo)
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"source":  "project edit",
						"message": "can't get existing webhook",
						"project": id,
						"owner":   responseRef.Owner,
						"repo":    responseRef.Repo,
					}))
					uis.LoggedError(w, r, http.StatusInternalServerError, err)
					return
				}
			}
			if err = hook.Insert(); err != nil {
				// A github hook as been created, but we couldn't
				// save the hook ID in our database. This needs
				// manual attention for clean up
				grip.Error(message.WrapError(err, message.Fields{
					"source":  "project edit",
					"message": "can't save hook to db",
					"project": id,
					"owner":   responseRef.Owner,
					"repo":    responseRef.Repo,
					"hook_id": hook.HookID,
				}))
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
		} else {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "project edit",
				"message": "can't setup webhook",
				"project": id,
				"owner":   responseRef.Owner,
				"repo":    responseRef.Repo,
			}))
			// don't return here:
			// sometimes people change a project to track a personal
			// branch we don't have access to
		}
	}
	var aliasesDefined bool
	if responseRef.PRTestingEnabled {
		if hook == nil {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("Cannot enable PR Testing in this repo, must enable GitHub webhooks first"))
			return
		}
		var conflictingRefs []model.ProjectRef
		conflictingRefs, err = model.FindMergedProjectRefsByRepoAndBranch(responseRef.Owner, responseRef.Repo, responseRef.Branch)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		for _, ref := range conflictingRefs {
			if ref.PRTestingEnabled && ref.Id != id {
				uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot enable PR Testing in this repo, must disable in '%s' first", ref.Id))
				return
			}
		}

		// verify there are PR aliases defined
		aliasesDefined, err = verifyAliasExists(evergreen.GithubAlias, projectRef.Id, responseRef.GitHubAliases, responseRef.DeleteAliases)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "can't check if GitHub aliases are set"))
			return
		}
		if !aliasesDefined {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("cannot enable PR testing without patch definitions"))
			return
		}
	}
	// verify git tag alias parameters
	if responseRef.GitTagVersionsEnabled {
		aliasesDefined, err = verifyAliasExists(evergreen.GitTagAlias, projectRef.Id, responseRef.GitTagAliases, responseRef.DeleteAliases)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "can't check if GitHub aliases are set"))
			return
		}
		if !aliasesDefined {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("cannot enable git tag versions without version definitions"))
			return
		}
		if len(responseRef.GitTagAuthorizedUsers) == 0 && len(responseRef.GitTagAuthorizedTeams) == 0 {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("must authorize users or teams to create git tag versions"))
			return
		}
	}

	// Prevent multiple projects tracking the same repo/branch from enabling commit queue
	commitQueueParamsInterface, err := responseRef.CommitQueue.ToService()
	commitQueueParams, ok := commitQueueParamsInterface.(model.CommitQueueParams)
	if err != nil || !ok {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot read Commit Queue into model"))
		return
	}

	if commitQueueParams.Enabled {
		var projRef *model.ProjectRef
		projRef, err = model.FindOneProjectRefWithCommitQueueByOwnerRepoAndBranch(responseRef.Owner, responseRef.Repo, responseRef.Branch)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if projRef != nil && projRef.CommitQueue.Enabled && projRef.Id != id {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot enable Commit Queue in this repo, must disable in '%s' first", projRef.Id))
			return
		}

		if hook == nil {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot enable Commit Queue without github webhooks enabled for the project"))
			return
		}

		// verify there are commit queue aliases defined
		var exists bool
		exists, err = verifyAliasExists(evergreen.CommitQueueAlias, projectRef.Id, responseRef.CommitQueueAliases, responseRef.DeleteAliases)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "can't check if commit queue aliases are set"))
			return
		}
		if !exists {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("cannot enable commit queue without patch definitions"))
			return
		}
		var cq *commitqueue.CommitQueue
		cq, err = commitqueue.FindOneId(projectRef.Id)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if cq == nil {
			cq := &commitqueue.CommitQueue{ProjectID: projectRef.Id}
			if err = commitqueue.InsertQueue(cq); err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
		}
	}

	i, err := responseRef.TaskSync.ToService()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "cannot convert API task sync options to service representation"))
		return
	}
	taskSync, ok := i.(model.TaskSyncOptions)
	if !ok {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("expected task sync options but was actually '%T'", i))
		return
	}

	catcher := grip.NewSimpleCatcher()
	for i, t := range responseRef.Triggers {
		catcher.Add(t.Validate(id))
		if t.DefinitionID == "" {
			responseRef.Triggers[i].DefinitionID = utility.RandomString()
		}
	}
	for _, t := range responseRef.PatchTriggerAliases {
		catcher.Add(model.ValidateTriggerDefinition(t, id))
	}
	for i, buildDef := range responseRef.PeriodicBuilds {
		catcher.Wrapf(buildDef.Validate(), "invalid periodic build definition on line %d", i+1)
	}
	if catcher.HasErrors() {
		uis.LoggedError(w, r, http.StatusBadRequest, catcher.Resolve())
		return
	}

	projectRef.DisplayName = responseRef.DisplayName
	projectRef.RemotePath = responseRef.RemotePath
	projectRef.SpawnHostScriptPath = responseRef.SpawnHostScriptPath
	projectRef.BatchTime = responseRef.BatchTime
	projectRef.Branch = responseRef.Branch
	projectRef.Enabled = responseRef.Enabled
	projectRef.DefaultLogger = responseRef.DefaultLogger
	projectRef.CedarTestResultsEnabled = responseRef.CedarTestResultsEnabled
	projectRef.Private = responseRef.Private
	projectRef.Restricted = responseRef.Restricted
	projectRef.Owner = responseRef.Owner
	projectRef.DeactivatePrevious = responseRef.DeactivatePrevious
	projectRef.Repo = responseRef.Repo
	projectRef.Admins = responseRef.Admins
	projectRef.GitTagAuthorizedUsers = responseRef.GitTagAuthorizedUsers
	projectRef.GitTagAuthorizedTeams = responseRef.GitTagAuthorizedTeams
	projectRef.GitTagVersionsEnabled = responseRef.GitTagVersionsEnabled
	projectRef.UseRepoSettings = responseRef.UseRepoSettings
	projectRef.Id = id
	projectRef.PRTestingEnabled = responseRef.PRTestingEnabled
	projectRef.CommitQueue = commitQueueParams
	projectRef.TaskSync = taskSync
	projectRef.PatchingDisabled = responseRef.PatchingDisabled
	projectRef.DispatchingDisabled = responseRef.DispatchingDisabled
	projectRef.RepotrackerDisabled = responseRef.RepotrackerDisabled
	projectRef.NotifyOnBuildFailure = responseRef.NotifyOnBuildFailure
	projectRef.Triggers = responseRef.Triggers
	projectRef.PatchTriggerAliases = responseRef.PatchTriggerAliases
	projectRef.FilesIgnoredFromCache = responseRef.FilesIgnoredFromCache
	projectRef.DisabledStatsCache = responseRef.DisabledStatsCache
	projectRef.PeriodicBuilds = []model.PeriodicBuildDefinition{}
	projectRef.TracksPushEvents = hook != nil
	for _, periodicBuild := range responseRef.PeriodicBuilds {
		projectRef.PeriodicBuilds = append(projectRef.PeriodicBuilds, *periodicBuild)
	}

	i, err = responseRef.WorkstationConfig.ToService()
	catcher.Add(err)
	config, ok := i.(model.WorkstationConfig)
	if !ok {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("expected workstation config but was actually '%T'", i))
		return
	}
	projectRef.WorkstationConfig = config

	if responseRef.ForceRepotrackerRun {
		ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectRef.Id)
		if err = uis.queue.Put(ctx, j); err != nil {
			grip.Error(errors.Wrap(err, "problem creating catchup job from UI"))
		}
	}

	err = projectRef.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	origSubscriptions, _ := event.FindSubscriptionsByOwner(projectRef.Id, event.OwnerTypeProject)
	for _, apiSubscription := range responseRef.Subscriptions {
		var subscriptionIface interface{}
		subscriptionIface, err = apiSubscription.ToService()
		if err != nil {
			catcher.Add(err)
		}
		subscription := subscriptionIface.(event.Subscription)
		subscription.Selectors = []event.Selector{
			{
				Type: "project",
				Data: projectRef.Id,
			},
		}
		if subscription.TriggerData != nil && subscription.TriggerData[event.SelectorRequester] != "" {
			subscription.Selectors = append(subscription.Selectors, event.Selector{
				Type: "requester",
				Data: subscription.TriggerData[event.SelectorRequester],
			})
		} else {
			subscription.Selectors = append(subscription.Selectors, event.Selector{
				Type: "requester",
				Data: evergreen.RepotrackerVersionRequester,
			})
		}
		subscription.OwnerType = event.OwnerTypeProject
		if subscription.Owner != projectRef.Id {
			subscription.Owner = projectRef.Id
			subscription.ID = ""
		}
		if !trigger.ValidateTrigger(subscription.ResourceType, subscription.Trigger) {
			catcher.Add(errors.Errorf("subscription type/trigger is invalid: %s/%s", subscription.ResourceType, subscription.Trigger))
			continue
		}
		if err = subscription.Upsert(); err != nil {
			catcher.Add(err)
		}
	}

	for _, id := range responseRef.DeleteSubscriptions {
		catcher.Add(event.RemoveSubscription(id))
	}

	if catcher.HasErrors() {
		uis.LoggedError(w, r, http.StatusInternalServerError, catcher.Resolve())
		return
	}

	projectVars, err := model.FindOneProjectVars(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectVars == nil {
		projectVars = &model.ProjectVars{}
	}
	origProjectVars := *projectVars

	// if we're copying a project we should use the variables from the DB for that project,
	// in order to get/store the correct values for private variables.
	if responseRef.Identifier != id && responseRef.Identifier != "" {
		projectVars, err = model.FindOneProjectVars(responseRef.Identifier)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "problem getting variables for project '%s'", responseRef.Identifier))
			return
		}
		projectVars.Id = id
	} else {
		// If the variable is private, and if the variable in the submission is
		// empty, then do not modify it. This variable has been redacted and is
		// therefore empty in the submission, since the client does not have
		// access to it, and we should not overwrite it.
		for k, v := range projectVars.Vars {
			if _, ok := projectVars.PrivateVars[k]; ok {
				if val, ok := responseRef.ProjVarsMap[k]; ok && val == "" {
					responseRef.ProjVarsMap[k] = v
				}
			}
		}
		projectVars.Vars = responseRef.ProjVarsMap
		projectVars.PrivateVars = responseRef.PrivateVars
		projectVars.RestrictedVars = responseRef.RestrictedVars
	}

	_, err = projectVars.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	origProjectAliases, _ := model.FindAliasesForProject(id)
	var projectAliases []model.ProjectAlias
	projectAliases = append(projectAliases, responseRef.GitHubAliases...)
	projectAliases = append(projectAliases, responseRef.CommitQueueAliases...)
	projectAliases = append(projectAliases, responseRef.PatchAliases...)
	projectAliases = append(projectAliases, responseRef.GitTagAliases...)

	catcher.Add(model.UpsertAliasesForProject(projectAliases, id))

	for _, alias := range responseRef.DeleteAliases {
		catcher.Add(model.RemoveProjectAlias(alias))
	}
	if catcher.HasErrors() {
		uis.LoggedError(w, r, http.StatusInternalServerError, catcher.Resolve())
		return
	}

	username := dbUser.DisplayName()

	before := &model.ProjectSettingsEvent{
		ProjectRef:         origProjectRef,
		GitHubHooksEnabled: origGithubWebhookEnabled,
		Vars:               origProjectVars,
		Aliases:            origProjectAliases,
		Subscriptions:      origSubscriptions,
	}

	currentAliases, _ := model.FindAliasesForProject(id)
	currentSubscriptions, _ := event.FindSubscriptionsByOwner(projectRef.Id, event.OwnerTypeProject)
	after := &model.ProjectSettingsEvent{
		ProjectRef:         *projectRef,
		GitHubHooksEnabled: hook != nil,
		Vars:               *projectVars,
		Aliases:            currentAliases,
		Subscriptions:      currentSubscriptions,
	}
	if err = model.LogProjectModified(id, username, before, after); err != nil {
		grip.Infof("Could not log changes to project %s", id)
	}

	if origProjectRef.Restricted != projectRef.Restricted {
		if projectRef.Restricted {
			err = projectRef.MakeRestricted(ctx)
		} else {
			err = projectRef.MakeUnrestricted(ctx)
		}
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	// if owner/repo has changed or we're toggling repo settings off, update scope
	if projectRef.Owner != origProjectRef.Owner || projectRef.Repo != origProjectRef.Repo ||
		(!projectRef.UseRepoSettings && origProjectRef.UseRepoSettings) {
		if err = projectRef.RemoveFromRepoScope(); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error removing project ref from old repo scope"))
			return
		}
		projectRef.RepoRefId = "" // if using repo settings, will reassign this in the next block
	}
	if projectRef.UseRepoSettings && projectRef.RepoRefId == "" {
		if err = projectRef.AddToRepoScope(dbUser); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	toAdd, toRemove := utility.StringSliceSymmetricDifference(projectRef.Admins, origProjectRef.Admins)
	if err = projectRef.UpdateAdminRoles(toAdd, toRemove); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	allProjects, err := uis.filterViewableProjects(dbUser)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := struct {
		AllProjects []model.ProjectRef
	}{allProjects}

	gimlet.WriteJSON(w, data)
}

func (uis *UIServer) addProject(w http.ResponseWriter, r *http.Request) {

	dbUser := MustHaveUser(r)
	_ = MustHaveProjectContext(r)

	id := gimlet.GetVars(r)["project_id"]

	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectRef != nil {
		http.Error(w, "Project already exists", http.StatusBadRequest)
		return
	}

	newProject := model.ProjectRef{
		Id:         id,
		Identifier: id,
		Tracked:    true,
		RepoKind:   "github",
	}

	err = newProject.Add(dbUser)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error adding project",
		}))
		errMsg := fmt.Sprintf("error adding project")
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New(errMsg))
		return
	}

	newProjectVars := model.ProjectVars{
		Id: newProject.Id,
	}

	err = newProjectVars.Insert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	username := dbUser.DisplayName()
	if err = model.LogProjectAdded(id, username); err != nil {
		grip.Infof("Could not log new project %s", id)
	}

	allProjects, err := uis.filterViewableProjects(dbUser)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	data := struct {
		Available   bool
		ProjectId   string
		AllProjects []model.ProjectRef
	}{true, id, allProjects}

	gimlet.WriteJSON(w, data)
}

// setRevision sets the latest revision in the Repository
// database to the revision sent from the projects page.
func (uis *UIServer) setRevision(w http.ResponseWriter, r *http.Request) {
	MustHaveUser(r)

	id := gimlet.GetVars(r)["project_id"]

	body := util.NewRequestReader(r)
	defer body.Close()

	data, err := ioutil.ReadAll(body)
	if err != nil {
		uis.LoggedError(w, r, http.StatusNotFound, err)
		return
	}

	revision := string(data)
	if revision == "" {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("revision sent was empty"))
		return
	}

	// update the latest revision to be the revision id
	err = model.UpdateLastRevision(id, revision)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// update the projectRef too
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	projectRef.RepotrackerError.Exists = false
	projectRef.RepotrackerError.InvalidRevision = ""
	projectRef.RepotrackerError.MergeBaseRevision = ""
	err = projectRef.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// run the repotracker for the project
	ts := utility.RoundPartOfHour(1).Format(units.TSFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectRef.Id)
	if err := uis.queue.Put(r.Context(), j); err != nil {
		grip.Error(errors.Wrap(err, "problem creating catchup job from UI"))
	}
	gimlet.WriteJSON(w, nil)
}

func (uis *UIServer) projectEvents(w http.ResponseWriter, r *http.Request) {
	// Validate the project exists
	id := gimlet.GetVars(r)["project_id"]
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectRef == nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	DBUser := MustHaveUser(r)
	authorized := isAdmin(DBUser, projectRef)
	template := "not_admin.html"
	if authorized {
		template = "project_events.html"
	}

	data := struct {
		Project string
		ViewData
	}{id, uis.GetCommonViewData(w, r, true, true)}
	uis.render.WriteResponse(w, http.StatusOK, data, "base", template, "base_angular.html", "menu.html")
}

func verifyAliasExists(alias, projectIdentifier string, newAliasDefinitions []model.ProjectAlias, deletedAliasDefinitionIDs []string) (bool, error) {
	if len(newAliasDefinitions) > 0 {
		return true, nil
	}

	existingAliasDefinitions, err := model.FindAliasInProject(projectIdentifier, alias)
	if err != nil {
		return false, errors.Wrap(err, "error checking for existing aliases")
	}

	for _, a := range existingAliasDefinitions {
		// only consider aliases that won't be deleted
		if !utility.StringSliceContains(deletedAliasDefinitionIDs, a.ID.Hex()) {
			return true, nil
		}
	}
	return false, nil
}
