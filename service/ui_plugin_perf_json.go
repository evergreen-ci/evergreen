package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	mgo "gopkg.in/mgo.v2"
)

func perfGetTasksForVersion(w http.ResponseWriter, r *http.Request) {
	jsonForTasks, err := model.PerfGetTasksForVersion(mux.Vars(r)["version_id"], mux.Vars(r)["name"])
	if jsonForTasks == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	util.WriteJSON(w, http.StatusOK, jsonForTasks)
}

func perfGetTasksForLatestVersion(w http.ResponseWriter, r *http.Request) {
	project := mux.Vars(r)["project_id"]
	name := mux.Vars(r)["name"]
	skip, err := util.GetIntValue(r, "skip", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if skip < 0 {
		http.Error(w, "negative skip", http.StatusBadRequest)
		return
	}

	data, err := model.PerfGetTasksForLatestVersion(project, name, skip)
	if err != nil {
		if err == mgo.ErrNotFound {
			http.Error(w, "{}", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	util.WriteJSON(w, http.StatusOK, data)
}

// getVersion returns a StatusOK if the route is hit
func perfGetVersion(w http.ResponseWriter, r *http.Request) {
	util.WriteJSON(w, http.StatusOK, "1")
}

// FIXME Returns random object that matches conditions
//       Given set of parameters is not specific enough
func perfGetCommit(w http.ResponseWriter, r *http.Request) {
	projectId := mux.Vars(r)["project_id"]
	revision := mux.Vars(r)["revision"]
	variant := mux.Vars(r)["variant"]
	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]
	jsonForTask, err := model.GetTaskJSONCommit(projectId, revision, variant, taskName, name)
	if err != nil {
		if err != mgo.ErrNotFound {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		util.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	util.WriteJSON(w, http.StatusOK, jsonForTask)
}

func perfGetTaskHistory(w http.ResponseWriter, r *http.Request) {
	t, err := task.FindOne(task.ById(mux.Vars(r)["task_id"]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}

	history, err := model.GetTaskJSONHistory(t, mux.Vars(r)["name"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	util.WriteJSON(w, http.StatusOK, history)
}

// uiGetTaskById sends back a JSONTask with the corresponding task id.
func perfGetTaskById(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	name := mux.Vars(r)["name"]
	jsonForTask, err := model.GetTaskJSONById(taskId, name)
	if err != nil {
		if err != mgo.ErrNotFound {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	util.WriteJSON(w, http.StatusOK, jsonForTask)
}

func perfGetTags(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	tags, err := model.GetTaskJSONTags(taskId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	util.WriteJSON(w, http.StatusOK, tags)
}

// handleTaskTags will update the TaskJSON's tags depending on the request.
func perfHandleTaskTag(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	name := mux.Vars(r)["name"]

	var err error
	switch r.Method {
	case http.MethodDelete:
		err = model.DeleteTaskJSONTagFromTask(taskId, name)
	case http.MethodPost:
		tc := model.TagContainer{}
		err = util.ReadJSONInto(util.NewRequestReader(r), &tc)
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
		if err == mgo.ErrNotFound {
			http.Error(w, "{}", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	util.WriteJSON(w, http.StatusOK, "")
}

// Wrapper `GetDistinctTagNames` model function
// requires HTTP GET param `project_id`
// Return distinct list of tags for given `project_id`
func perfGetProjectTags(w http.ResponseWriter, r *http.Request) {
	projectIdVals, ok := r.URL.Query()["project_id"]

	if !ok || len(projectIdVals) != 1 {
		http.Error(w, "'project_id' param is required", http.StatusBadRequest)
	}

	data, err := model.GetDistinctTagNames(projectIdVals[0])

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	util.WriteJSON(w, http.StatusOK, data)
}

func perfGetTaskJSONByTag(w http.ResponseWriter, r *http.Request) {
	projectId := mux.Vars(r)["project_id"]
	tag := mux.Vars(r)["tag"]
	variant := mux.Vars(r)["variant"]
	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]

	jsonForTask, err := model.GetTaskJSONByTag(projectId, tag, variant, taskName, name)

	if err != nil {
		if err != mgo.ErrNotFound {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		util.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	util.WriteJSON(w, http.StatusOK, jsonForTask)
}
