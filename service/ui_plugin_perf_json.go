package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
)

func perfGetTasksForVersion(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	jsonForTasks, err := model.PerfGetTasksForVersion(vars["version_id"], vars["name"])
	if jsonForTasks == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gimlet.WriteJSON(w, jsonForTasks)
}

func perfGetTasksForLatestVersion(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	projectName := vars["project_id"]
	name := vars["name"]
	skip, err := util.GetIntValue(r, "skip", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if skip < 0 {
		http.Error(w, "negative skip", http.StatusBadRequest)
		return
	}
	projectId, err := model.GetIdForProject(projectName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := model.PerfGetTasksForLatestVersion(projectId, name, skip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if data == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}

	gimlet.WriteJSON(w, data)
}

// getVersion returns a StatusOK if the route is hit
func perfGetVersion(w http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(w, "1")
}
