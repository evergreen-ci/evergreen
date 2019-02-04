package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const tsFormat = "2006-01-02.15-04-05"

// publicProjectFields are the fields needed by the UI
// on base_angular and the menu
type UIProjectFields struct {
	Identifier  string `json:"identifier"`
	DisplayName string `json:"display_name"`
	Repo        string `json:"repo_name"`
	Owner       string `json:"owner_name"`
}

// filterAuthorizedProjects iterates through a list of projects and returns a list of all the projects that a user
// is authorized to view and edit the settings of.
func (uis *UIServer) filterAuthorizedProjects(u gimlet.User) ([]model.ProjectRef, error) {
	allProjects, err := model.FindAllProjectRefs()
	if err != nil {
		return nil, err
	}
	authorizedProjects := []model.ProjectRef{}
	// only returns projects for which the user is authorized to see.
	for _, project := range allProjects {
		if uis.isSuperUser(u) || isAdmin(u, &project) {
			authorizedProjects = append(authorizedProjects, project)
		}
	}
	return authorizedProjects, nil

}
func (uis *UIServer) projectsPage(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)

	allProjects, err := uis.filterAuthorizedProjects(dbUser)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	data := struct {
		AllProjects []model.ProjectRef
		ViewData
	}{allProjects, uis.GetCommonViewData(w, r, true, true)}

	uis.render.WriteResponse(w, http.StatusOK, data, "base", "projects.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) projectPage(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveProjectContext(r)
	_ = MustHaveUser(r)

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
	projVars.RedactPrivateVars()

	projectAliases, err := model.FindAliasesForProject(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	prConflictingRefs, err := model.FindProjectRefsByRepoAndBranch(projRef.Owner, projRef.Repo, projRef.Branch)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	conflictingRefs := []string{}
	for _, ref := range prConflictingRefs {
		if ref.PRTestingEnabled && ref.Identifier != projRef.Identifier {
			conflictingRefs = append(conflictingRefs, ref.Identifier)
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

	apiHook := restModel.APIGithubHook{}
	if hook != nil {
		if err = apiHook.BuildFromService(*hook); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	subscriptions, err := event.FindSubscriptionsByOwner(projRef.Identifier, event.OwnerTypeProject)
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

	data := struct {
		ProjectRef      *model.ProjectRef
		ProjectVars     *model.ProjectVars
		ProjectAliases  []model.ProjectAlias        `json:"aliases,omitempty"`
		ConflictingRefs []string                    `json:"pr_testing_conflicting_refs,omitempty"`
		GithubHook      restModel.APIGithubHook     `json:"github_hook"`
		Subscriptions   []restModel.APISubscription `json:"subscriptions"`
	}{projRef, projVars, projectAliases, conflictingRefs, apiHook, apiSubscriptions}

	// the project context has all projects so make the ui list using all projects
	gimlet.WriteJSON(w, data)
}

// ProjectNotFound calls WriteHTML with the invalid-project page. It should be called whenever the
// project specified by the user does not exist, or when there are no projects at all.
func (uis *UIServer) ProjectNotFound(projCtx projectContext, w http.ResponseWriter, r *http.Request) {
	uis.render.WriteResponse(w, http.StatusNotFound, uis.GetCommonViewData(w, r, false, false), "base", "invalid_project.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyProject(w http.ResponseWriter, r *http.Request) {

	dbUser := MustHaveUser(r)
	_ = MustHaveProjectContext(r)
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

	responseRef := struct {
		Identifier         string               `json:"id"`
		DisplayName        string               `json:"display_name"`
		RemotePath         string               `json:"remote_path"`
		BatchTime          int                  `json:"batch_time"`
		DeactivatePrevious bool                 `json:"deactivate_previous"`
		Branch             string               `json:"branch_name"`
		ProjVarsMap        map[string]string    `json:"project_vars"`
		ProjectAliases     []model.ProjectAlias `json:"project_aliases"`
		DeleteAliases      []string             `json:"delete_aliases"`
		PrivateVars        map[string]bool      `json:"private_vars"`
		Enabled            bool                 `json:"enabled"`
		Private            bool                 `json:"private"`
		Owner              string               `json:"owner_name"`
		Repo               string               `json:"repo_name"`
		Admins             []string             `json:"admins"`
		TracksPushEvents   bool                 `json:"tracks_push_events"`
		PRTestingEnabled   bool                 `json:"pr_testing_enabled"`
		CommitQueueEnabled bool                 `json:"commitq_enabled"`
		PatchingDisabled   bool                 `json:"patching_disabled"`
		AlertConfig        map[string][]struct {
			Provider string                 `json:"provider"`
			Settings map[string]interface{} `json:"settings"`
		} `json:"alert_config"`
		NotifyOnBuildFailure  bool                        `json:"notify_on_failure"`
		SetupGithubHook       bool                        `json:"setup_github_hook"`
		ForceRepotrackerRun   bool                        `json:"force_repotracker_run"`
		Subscriptions         []restModel.APISubscription `json:"subscriptions"`
		DeleteSubscriptions   []string                    `json:"delete_subscriptions"`
		Triggers              []model.TriggerDefinition   `json:"triggers"`
		FilesIgnoredFromCache []string                    `json:"files_ignored_from_cache"`
		DisabledStatsCache    bool                        `json:"disabled_stats_cache"`
	}{}

	if err = util.ReadJSONInto(util.NewRequestReader(r), &responseRef); err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request body %v", err), http.StatusInternalServerError)
		return
	}

	if len(responseRef.Branch) == 0 {
		http.Error(w, fmt.Sprintf("no branch specified %v", err), http.StatusBadRequest)
		return
	}

	errs := []string{}
	for i, pd := range responseRef.ProjectAliases {
		if strings.TrimSpace(pd.Alias) == "" {
			errs = append(errs, fmt.Sprintf("alias name #%d can't be empty string", i+1))
		}
		if strings.TrimSpace(pd.Variant) == "" {
			errs = append(errs, fmt.Sprintf("variant regex #%d can't be empty string", i+1))
		}
		if (strings.TrimSpace(pd.Task) == "") == (len(pd.Tags) == 0) {
			errs = append(errs, fmt.Sprintf("must specify exactly one of task regex or tags on line #%d ", i+1))
		}

		if _, err = regexp.Compile(pd.Variant); err != nil {
			errs = append(errs, fmt.Sprintf("variant regex #%d is invalid", i+1))
		}
		if _, err = regexp.Compile(pd.Task); err != nil {
			errs = append(errs, fmt.Sprintf("task regex #%d is invalid", i+1))
		}
	}
	if len(errs) > 0 {
		errMsg := ""
		for _, err := range errs {
			errMsg += err + ", "
		}
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New(errMsg))

		return
	}

	if responseRef.PRTestingEnabled {
		var conflictingRefs []model.ProjectRef
		conflictingRefs, err = model.FindProjectRefsByRepoAndBranch(responseRef.Owner, responseRef.Repo, responseRef.Branch)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		for _, ref := range conflictingRefs {
			if ref.PRTestingEnabled && ref.Identifier != id {
				uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot enable PR Testing in this repo, must disable in '%s' first", ref.Identifier))
				return
			}
		}
	}

	// Prevent multiple projects tracking the same repo/branch from enabling commit queue
	if responseRef.CommitQueueEnabled {
		var projRef *model.ProjectRef
		projRef, err = model.FindOneProjectRefWithCommitQByOwnerRepoAndBranch(responseRef.Owner, responseRef.Repo, responseRef.Branch)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if projRef.CommitQueueEnabled && projRef.Identifier != id {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot enable Commit Queue in this repo, must disable in '%s' first", projRef.Identifier))
			return
		}
	}

	catcher := grip.NewSimpleCatcher()
	for i, trigger := range responseRef.Triggers {
		catcher.Add(trigger.Validate(id))
		if trigger.DefinitionID == "" {
			responseRef.Triggers[i].DefinitionID = util.RandomString()
		}
	}
	if catcher.HasErrors() {
		uis.LoggedError(w, r, http.StatusBadRequest, catcher.Resolve())
		return
	}

	origProjectRef := *projectRef

	projectRef.DisplayName = responseRef.DisplayName
	projectRef.RemotePath = responseRef.RemotePath
	projectRef.BatchTime = responseRef.BatchTime
	projectRef.Branch = responseRef.Branch
	projectRef.Enabled = responseRef.Enabled
	projectRef.Private = responseRef.Private
	projectRef.Owner = responseRef.Owner
	projectRef.DeactivatePrevious = responseRef.DeactivatePrevious
	projectRef.Repo = responseRef.Repo
	projectRef.Admins = responseRef.Admins
	projectRef.Identifier = id
	projectRef.TracksPushEvents = responseRef.TracksPushEvents
	projectRef.PRTestingEnabled = responseRef.PRTestingEnabled
	projectRef.CommitQueueEnabled = responseRef.CommitQueueEnabled
	projectRef.PatchingDisabled = responseRef.PatchingDisabled
	projectRef.NotifyOnBuildFailure = responseRef.NotifyOnBuildFailure
	projectRef.Triggers = responseRef.Triggers
	projectRef.FilesIgnoredFromCache = responseRef.FilesIgnoredFromCache
	projectRef.DisabledStatsCache = responseRef.DisabledStatsCache

	projectVars, err := model.FindOneProjectVars(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	var hook *model.GithubHook
	hook, err = model.FindGithubHook(responseRef.Owner, responseRef.Repo)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	origGithubWebhookEnabled := hook != nil
	currentGithubWebhookEnabled := origGithubWebhookEnabled
	if responseRef.SetupGithubHook && hook == nil {
		var hookID int
		hookID, err = uis.setupGithubHook(projectRef)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"source":  "project edit",
				"message": "can't setup webhook",
				"project": id,
				"owner":   responseRef.Owner,
				"repo":    responseRef.Repo,
			}))
			// don't kill it here, sometimes people change
			// Evergreen to track a personal branch,
			// one that we have no access to
			projectRef.TracksPushEvents = false
		} else {
			currentGithubWebhookEnabled = true
			hook := model.GithubHook{
				HookID: hookID,
				Owner:  responseRef.Owner,
				Repo:   responseRef.Repo,
			}
			if err = hook.Insert(); err != nil {
				// A github hook as been created, but we couldn't
				// save the hook ID in our database. This needs
				// manual attention for clean up
				grip.Alert(message.WrapError(err, message.Fields{
					"source":  "project edit",
					"message": "can't save hook",
					"project": id,
					"owner":   responseRef.Owner,
					"repo":    responseRef.Repo,
					"hook_id": hookID,
				}))
				uis.LoggedError(w, r, http.StatusInternalServerError, err)
				return
			}
		}
	}

	if responseRef.ForceRepotrackerRun {
		ts := util.RoundPartOfHour(1).Format(tsFormat)
		j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectRef.Identifier)
		if err = uis.queue.Put(j); err != nil {
			grip.Error(errors.Wrap(err, "problem creating catchup job from UI"))
		}
	}

	err = projectRef.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	origSubscriptions, _ := event.FindSubscriptionsByOwner(projectRef.Identifier, event.OwnerTypeProject)
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
				Data: projectRef.Identifier,
			},
			{
				Type: "requester",
				Data: evergreen.RepotrackerVersionRequester,
			},
		}
		subscription.OwnerType = event.OwnerTypeProject
		subscription.Owner = projectRef.Identifier
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

	origProjectVars := *projectVars
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

	_, err = projectVars.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	origProjectAliases, _ := model.FindAliasesForProject(id)
	for i := range responseRef.ProjectAliases {
		responseRef.ProjectAliases[i].ProjectID = id
		catcher.Add(responseRef.ProjectAliases[i].Upsert())
	}

	for _, alias := range responseRef.DeleteAliases {
		catcher.Add(model.RemoveProjectAlias(alias))
	}
	if catcher.HasErrors() {
		uis.LoggedError(w, r, http.StatusInternalServerError, catcher.Resolve())
		return
	}

	username := dbUser.DisplayName()

	before := model.ProjectSettingsEvent{
		ProjectRef:         origProjectRef,
		GitHubHooksEnabled: origGithubWebhookEnabled,
		Vars:               origProjectVars,
		Aliases:            origProjectAliases,
		Subscriptions:      origSubscriptions,
	}

	currentAliases, _ := model.FindAliasesForProject(id)
	currentSubscriptions, _ := event.FindSubscriptionsByOwner(projectRef.Identifier, event.OwnerTypeProject)
	after := model.ProjectSettingsEvent{
		ProjectRef:         *projectRef,
		GitHubHooksEnabled: currentGithubWebhookEnabled,
		Vars:               *projectVars,
		Aliases:            currentAliases,
		Subscriptions:      currentSubscriptions,
	}
	if err = model.LogProjectModified(id, username, before, after); err != nil {
		grip.Infof("Could not log changes to project %s", id)
	}

	allProjects, err := uis.filterAuthorizedProjects(dbUser)
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
		http.Error(w, "Project already exists", http.StatusInternalServerError)
		return
	}

	newProject := model.ProjectRef{
		Identifier: id,
		Enabled:    true,
		Tracked:    true,
		RepoKind:   "github",
	}

	err = newProject.Insert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	newProjectVars := model.ProjectVars{
		Id: newProject.Identifier,
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

	allProjects, err := uis.filterAuthorizedProjects(dbUser)

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
	ts := util.RoundPartOfHour(1).Format(tsFormat)
	j := units.NewRepotrackerJob(fmt.Sprintf("catchup-%s", ts), projectRef.Identifier)
	if err := uis.queue.Put(j); err != nil {
		grip.Error(errors.Wrap(err, "problem creating catchup job from UI"))
	}
	gimlet.WriteJSON(w, nil)
}

func (uis *UIServer) setupGithubHook(projectRef *model.ProjectRef) (int, error) {
	token, err := uis.Settings.GetGithubOauthToken()
	if err != nil {
		return 0, err
	}

	if uis.Settings.Api.GithubWebhookSecret == "" {
		return 0, errors.New("Evergreen is not configured for Github Webhooks")
	}

	httpClient, err := util.GetOAuth2HTTPClient(token)
	if err != nil {
		return 0, err
	}
	defer util.PutHTTPClient(httpClient)
	client := github.NewClient(httpClient)
	newHook := github.Hook{
		Name:   github.String("web"),
		Active: github.Bool(true),
		Events: []string{"*"},
		Config: map[string]interface{}{
			"url":          github.String(fmt.Sprintf("%s/rest/v2/hooks/github", uis.Settings.ApiUrl)),
			"content_type": github.String("json"),
			"secret":       github.String(uis.Settings.Api.GithubWebhookSecret),
			"insecure_ssl": github.String("0"),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hook, resp, err := client.Repositories.CreateHook(ctx, projectRef.Owner, projectRef.Repo, &newHook)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || hook == nil || hook.ID == nil {
		return 0, errors.New("unexpected data from github")
	}

	return *hook.ID, nil
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
	authorized := isAdmin(DBUser, projectRef) || uis.isSuperUser(DBUser)
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
