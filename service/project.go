package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/gorilla/mux"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

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
func (uis *UIServer) filterAuthorizedProjects(u *user.DBUser) ([]model.ProjectRef, error) {
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

	// construct a json-marshaling friendly representation of our supported triggers
	allTaskTriggers := []interface{}{}
	for _, taskTrigger := range alerts.AvailableTaskFailTriggers {
		allTaskTriggers = append(allTaskTriggers, struct {
			Id      string `json:"id"`
			Display string `json:"display"`
		}{taskTrigger.Id(), taskTrigger.Display()})
	}

	data := struct {
		AllProjects       []model.ProjectRef
		AvailableTriggers []interface{}
		ViewData
	}{allProjects, allTaskTriggers, uis.GetCommonViewData(w, r, true, true)}

	uis.WriteHTML(w, http.StatusOK, data, "base", "projects.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) projectPage(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveProjectContext(r)
	_ = MustHaveUser(r)

	vars := mux.Vars(r)
	id := vars["project_id"]

	projRef, err := model.FindOneProjectRef(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
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

	hook, err := model.FindGithubHook(projRef.Owner, projRef.Repo)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	apiHook := restModel.APIGithubHook{}
	if hook != nil {
		apiHook.BuildFromService(hook)
	}

	data := struct {
		ProjectRef      *model.ProjectRef
		ProjectVars     *model.ProjectVars
		ProjectAliases  []model.ProjectAlias    `json:"aliases,omitempty"`
		ConflictingRefs []string                `json:"pr_testing_conflicting_refs,omitempty"`
		GithubHook      restModel.APIGithubHook `json:"github_hook,omitempty"`
	}{projRef, projVars, projectAliases, conflictingRefs, apiHook}

	// the project context has all projects so make the ui list using all projects
	uis.WriteJSON(w, http.StatusOK, data)
}

// ProjectNotFound calls WriteHTML with the invalid-project page. It should be called whenever the
// project specified by the user does not exist, or when there are no projects at all.
func (uis *UIServer) ProjectNotFound(projCtx projectContext, w http.ResponseWriter, r *http.Request) {
	uis.WriteHTML(w, http.StatusNotFound, uis.GetCommonViewData(w, r, false, false), "base", "invalid_project.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyProject(w http.ResponseWriter, r *http.Request) {

	dbUser := MustHaveUser(r)
	_ = MustHaveProjectContext(r)

	vars := mux.Vars(r)
	id := vars["project_id"]

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
		AlertConfig        map[string][]struct {
			Provider string                 `json:"provider"`
			Settings map[string]interface{} `json:"settings"`
		} `json:"alert_config"`
		SetupGithubHook     bool `json:"setup_github_hook"`
		ForceRepotrackerRun bool `json:"force_repotracker_run"`
	}{}

	if err = util.ReadJSONInto(util.NewRequestReader(r), &responseRef); err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request body %v", err), http.StatusInternalServerError)
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
		if strings.TrimSpace(pd.Task) == "" && len(pd.Tags) == 0 {
			errs = append(errs, fmt.Sprintf("must specify either task regex or tags on line #%d ", i+1))
		}

		if _, err := regexp.Compile(pd.Variant); err != nil {
			errs = append(errs, fmt.Sprintf("variant regex #%d is invalid", i+1))
		}
		if _, err := regexp.Compile(pd.Task); err != nil {
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
		conflictingRefs, err := model.FindProjectRefsByRepoAndBranch(responseRef.Owner, responseRef.Repo, responseRef.Branch)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		for _, ref := range conflictingRefs {
			if ref.Identifier != id {
				uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Cannot enable PR Testing in this repo, must disable in '%s' first", id))
				return
			}
		}
	}

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

	projectRef.Alerts = map[string][]model.AlertConfig{}
	for triggerId, alerts := range responseRef.AlertConfig {
		//TODO validate the triggerID, provider, and settings.
		for _, alert := range alerts {
			projectRef.Alerts[triggerId] = append(projectRef.Alerts[triggerId], model.AlertConfig{
				Provider: alert.Provider,
				Settings: bson.M(alert.Settings),
			})
		}
	}

	if !projectRef.Enabled {
		projectRef.PRTestingEnabled = false
	}

	projectVars, err := model.FindOneProjectVars(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if responseRef.SetupGithubHook {
		hook, err := model.FindGithubHook(responseRef.Owner, responseRef.Repo)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		if hook != nil {
			hookID, err := uis.setupGithubHook(projectRef)
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

			} else {
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
	}

	if responseRef.ForceRepotrackerRun {
		j := units.NewRepotrackerJob(fmt.Sprintf("ui-triggered-job-%d", job.GetNumber()), projectRef.Identifier)
		if err = uis.queue.Put(j); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	err = projectRef.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	//modify project vars if necessary
	projectVars.Vars = responseRef.ProjVarsMap
	projectVars.PrivateVars = responseRef.PrivateVars

	_, err = projectVars.Upsert()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	catcher := grip.NewSimpleCatcher()
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

	allProjects, err := uis.filterAuthorizedProjects(dbUser)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	data := struct {
		AllProjects []model.ProjectRef
	}{allProjects}

	uis.WriteJSON(w, http.StatusOK, data)
}

func (uis *UIServer) addProject(w http.ResponseWriter, r *http.Request) {

	dbUser := MustHaveUser(r)
	_ = MustHaveProjectContext(r)

	vars := mux.Vars(r)
	id := vars["project_id"]

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

	uis.WriteJSON(w, http.StatusOK, data)
}

// setRevision sets the latest revision in the Repository
// database to the revision sent from the projects page.
func (uis *UIServer) setRevision(w http.ResponseWriter, r *http.Request) {
	MustHaveUser(r)

	vars := mux.Vars(r)
	id := vars["project_id"]

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

	uis.WriteJSON(w, http.StatusOK, nil)
}

func (uis *UIServer) setupGithubHook(projectRef *model.ProjectRef) (int, error) {
	token, err := uis.Settings.GetGithubOauthToken()
	if err != nil {
		return 0, err
	}

	if uis.Settings.Api.GithubWebhookSecret == "" {
		return 0, errors.New("Evergreen is not configured for Github Webhooks")
	}

	httpClient, err := util.GetHttpClientForOauth2(token)
	if err != nil {
		return 0, err
	}
	defer util.PutHttpClientForOauth2(httpClient)
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
