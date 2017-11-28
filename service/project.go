package service

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
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

	data := struct {
		ProjectRef  *model.ProjectRef
		ProjectVars *model.ProjectVars
	}{projRef, projVars}

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
		PatchVariants      []model.PatchVariant `json:"patch_variants"`
		PrivateVars        map[string]bool      `json:"private_vars"`
		Enabled            bool                 `json:"enabled"`
		Private            bool                 `json:"private"`
		Owner              string               `json:"owner_name"`
		Repo               string               `json:"repo_name"`
		Admins             []string             `json:"admins"`
		AlertConfig        map[string][]struct {
			Provider string                 `json:"provider"`
			Settings map[string]interface{} `json:"settings"`
		} `json:"alert_config"`
	}{}

	if err = util.ReadJSONInto(util.NewRequestReader(r), &responseRef); err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request body %v", err), http.StatusInternalServerError)
		return
	}

	for _, pv := range responseRef.PatchVariants {
		if _, err := regexp.Compile(pv.Variant); err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if _, err := regexp.Compile(pv.Task); err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
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

	err = projectRef.Upsert()

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	//modify project vars if necessary
	projectVars := model.ProjectVars{id, responseRef.ProjVarsMap, responseRef.PrivateVars, responseRef.PatchVariants}
	_, err = projectVars.Upsert()

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
