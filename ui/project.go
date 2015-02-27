package ui

import (
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
)

type projectSettings struct {
	ProjectRef  *model.ProjectRef  `json:"proj_ref"`
	ProjectVars *model.ProjectVars `json:"project_vars"`
}

func (uis *UIServer) projectsPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	allProjects, err := model.FindAllProjectRefs()

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	data := struct {
		ProjectData projectContext
		User        *model.DBUser
		AllProjects []model.ProjectRef
	}{projCtx, GetUser(r), allProjects}

	uis.WriteHTML(w, http.StatusOK, data, "base", "projects.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) projectPage(w http.ResponseWriter, r *http.Request) {

	projCtx := MustHaveProjectContext(r)
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
		ProjectData projectContext
		User        *model.DBUser
		ProjectRef  *model.ProjectRef
		ProjectVars *model.ProjectVars
	}{projCtx, GetUser(r), projRef, projVars}

	// the project context has all projects so make the ui list using all projects
	uis.WriteJSON(w, http.StatusOK, data)
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
		Identifier  string            `json:"id"`
		DisplayName string            `json:"display_name"`
		RemotePath  string            `json:"remote_path"`
		BatchTime   int               `json:"batch_time"`
		Branch      string            `json:"branch_name"`
		ProjVarsMap map[string]string `json:"project_vars"`
		Enabled     bool              `json:"enabled"`
		Owner       string            `json:"owner_name"`
		Repo        string            `json:"repo_name"`
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
	projectRef.Owner = responseRef.Owner
	projectRef.Repo = responseRef.Repo

	projectRef.Identifier = id

	err = projectRef.Upsert()

	if err != nil {
		http.Error(w, "Error parsing request body", http.StatusInternalServerError)
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
		http.Error(w, "Error finding tracked projects", http.StatusInternalServerError)
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
