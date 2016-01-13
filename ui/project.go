package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
	"io/ioutil"
	"net/http"
)

type projectSettings struct {
	ProjectRef  *model.ProjectRef  `json:"proj_ref"`
	ProjectVars *model.ProjectVars `json:"project_vars"`
}

func (uis *UIServer) projectsPage(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveUser(r)
	projCtx := MustHaveProjectContext(r)
	allProjects, err := model.FindAllProjectRefs()

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
		ProjectData       projectContext
		User              *user.DBUser
		AllProjects       []model.ProjectRef
		AvailableTriggers []interface{}
	}{projCtx, GetUser(r), allProjects, allTaskTriggers}

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
	uis.WriteHTML(w, http.StatusNotFound, struct {
		ProjectData projectContext
		User        *user.DBUser
	}{projCtx, GetUser(r)}, "base", "invalid_project.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyProject(w http.ResponseWriter, r *http.Request) {

	_ = MustHaveUser(r)

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
		Identifier         string            `json:"id"`
		DisplayName        string            `json:"display_name"`
		RemotePath         string            `json:"remote_path"`
		BatchTime          int               `json:"batch_time"`
		DeactivatePrevious bool              `json:"deactivate_previous"`
		Branch             string            `json:"branch_name"`
		ProjVarsMap        map[string]string `json:"project_vars"`
		Enabled            bool              `json:"enabled"`
		Private            bool              `json:"private"`
		Owner              string            `json:"owner_name"`
		Repo               string            `json:"repo_name"`
		AlertConfig        map[string][]struct {
			Provider string                 `json:"provider"`
			Settings map[string]interface{} `json:"settings"`
		} `json:"alert_config"`
	}{}

	err = util.ReadJSONInto(r.Body, &responseRef)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request body %v", err), http.StatusInternalServerError)
		return
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
	projectVars := model.ProjectVars{id, responseRef.ProjVarsMap}
	_, err = projectVars.Upsert()

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	allProjects, err := model.FindAllProjectRefs()

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

	_ = MustHaveUser(r)

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

	allProjects, err := model.FindAllProjectRefs()

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

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		uis.LoggedError(w, r, http.StatusNotFound, err)
		return
	}
	revision := string(data)
	if revision == "" {
		uis.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("revision sent was empty"))
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
