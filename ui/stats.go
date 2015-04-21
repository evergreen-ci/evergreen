package ui

import (
	"10gen.com/mci/model"
	"10gen.com/mci/model/user"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
)

func (uis *UIServer) taskTimingPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	if projCtx.Project == nil {
		http.Error(w, "not found", http.StatusNotFound)
	}

	type UIBuildVariant struct {
		Name      string
		TaskNames []string
	}

	type UIProject struct {
		Name          string
		BuildVariants []UIBuildVariant
	}

	var allProjects []UIProject
	for _, p := range projCtx.AllProjects {
		proj, err := model.FindProject("", &p)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		uip := UIProject{p.Identifier, []UIBuildVariant{}}
		for _, bv := range projCtx.Project.BuildVariants {
			newBv := UIBuildVariant{bv.Name, []string{}}
			for _, task := range proj.Tasks {
				newBv.TaskNames = append(newBv.TaskNames, task.Name)
			}
			uip.BuildVariants = append(uip.BuildVariants, newBv)
		}

		allProjects = append(allProjects, uip)
	}

	newProject := UIProject{projCtx.Project.Identifier, []UIBuildVariant{}}

	data := struct {
		ProjectData projectContext
		User        *user.DBUser
		Project     UIProject
		AllProjects []UIProject
	}{projCtx, GetUser(r), newProject, allProjects}

	uis.WriteHTML(w, http.StatusOK, data, "base", "task_timing.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) taskTimingJSON(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	beforeTaskId := r.FormValue("before")

	limit, err := getIntValue(r, "limit", 50)
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	buildVariant := mux.Vars(r)["build_variant"]
	taskName := mux.Vars(r)["task_name"]

	if projCtx.Project == nil {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}

	bv := projCtx.Project.FindBuildVariant(buildVariant)
	if bv == nil {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("build variant %v not found", buildVariant))
		return
	}

	foundTask := false
	if taskName == "compile" || taskName == "push" {
		foundTask = true
	} else {
		for _, task := range bv.Tasks {
			if task.Name == taskName {
				foundTask = true
				break
			}
		}
	}
	if !foundTask {
		uis.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("no task named '%v'", taskName))
		return
	}

	tasks, err := model.FindCompletedTasksByVariantAndName(projCtx.Project.Identifier,
		buildVariant, taskName, limit, beforeTaskId)

	if err != nil {
		uis.LoggedError(w, r, http.StatusNotFound, err)
		return
	}
	uis.WriteJSON(w, http.StatusOK, tasks)
}
