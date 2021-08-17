package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
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

// FIXME Returns random object that matches conditions
//       Given set of parameters is not specific enough
func perfGetCommit(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)

	projectName := vars["project_id"]
	revision := vars["revision"]
	variant := vars["variant"]
	taskName := vars["task_name"]
	name := vars["name"]

	projectId, err := model.GetIdForProject(projectName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonForTask, err := model.GetTaskJSONCommit(projectId, revision, variant, taskName, name)
	if err != nil {
		if !adb.ResultsNotFound(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		gimlet.WriteJSON(w, jsonForTask)
		return
	}
	gimlet.WriteJSON(w, jsonForTask)
}

func perfGetTaskHistory(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)

	t, err := task.FindOneId(vars["task_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}

	history, err := model.GetTaskJSONHistory(t, vars["name"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gimlet.WriteJSON(w, history)
}

// uiGetTaskById sends back a JSONTask with the corresponding task id.
func perfGetTaskById(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)

	taskId := vars["task_id"]
	name := vars["name"]
	jsonForTask, err := model.GetTaskJSONById(taskId, name)
	if err != nil {
		if !adb.ResultsNotFound(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	gimlet.WriteJSON(w, jsonForTask)
}

func perfGetTags(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	tags, err := model.GetTaskJSONTags(taskId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gimlet.WriteJSON(w, tags)
}

// handleTaskTags will update the TaskJSON's tags depending on the request.
func perfHandleTaskTag(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	taskId := vars["task_id"]
	name := vars["name"]

	var err error
	switch r.Method {
	case http.MethodDelete:
		err = model.DeleteTaskJSONTagFromTask(taskId, name)
	case http.MethodPost:
		tc := model.TagContainer{}
		err = utility.ReadJSON(util.NewRequestReader(r), &tc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if tc.Tag == "" {
			http.Error(w, "tag must not be blank", http.StatusBadRequest)
			return
		}
		err = model.SetTaskJSONTagForTask(taskId, name, tc.Tag)
	}

	if err != nil {
		if adb.ResultsNotFound(err) {
			http.Error(w, "{}", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	gimlet.WriteJSON(w, "")
}

// Wrapper `GetDistinctTagNames` model function
// requires HTTP GET param `project_id`
// Return distinct list of tags for given `project_id`
func perfGetProjectTags(w http.ResponseWriter, r *http.Request) {
	projectIdVals, ok := r.URL.Query()["project_id"]

	if !ok || len(projectIdVals) != 1 {
		http.Error(w, "'project_id' param is required", http.StatusBadRequest)
	}

	projectId, err := model.GetIdForProject(projectIdVals[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := model.GetDistinctTagNames(projectId)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	gimlet.WriteJSON(w, data)
}

func perfGetTaskJSONByTag(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	projectName := vars["project_id"]
	tag := vars["tag"]
	variant := vars["variant"]
	taskName := vars["task_name"]
	name := vars["name"]

	projectId, err := model.GetIdForProject(projectName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonForTask, err := model.GetTaskJSONByTag(projectId, tag, variant, taskName, name)

	if err != nil {
		if !adb.ResultsNotFound(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		gimlet.WriteJSON(w, jsonForTask)
		return
	}
	gimlet.WriteJSON(w, jsonForTask)
}
