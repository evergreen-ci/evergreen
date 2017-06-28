package service

import (
	"net/http"

	mgo "gopkg.in/mgo.v2"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
)

func (as *APIServer) getTaskJSONTagsForTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]

	tagged, err := model.GetTaskJSONTagsForTask(t.Project, t.BuildVariant, taskName, name)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, tagged)
}

func (as *APIServer) getTaskJSONTaskHistory(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	history, err := model.GetTaskJSONHistory(t, mux.Vars(r)["name"])
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, history)
}

func (as *APIServer) insertTaskJSON(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	name := mux.Vars(r)["name"]
	rawData := map[string]interface{}{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &rawData); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if err := model.InsertTaskJSON(t, name, rawData); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, "ok")
	return
}

func (as *APIServer) getTaskJSONByName(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]

	jsonForTask, err := model.GetTaskJSONByName(t.Version, t.BuildId, taskName, name)
	if err != nil {
		if err == mgo.ErrNotFound {
			as.WriteJSON(w, http.StatusNotFound, nil)
			return
		}

		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		as.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}

	as.WriteJSON(w, http.StatusOK, jsonForTask.Data)
}

// apiGetTaskForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func (as *APIServer) getTaskJSONForVariant(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]
	variantId := mux.Vars(r)["variant"]
	// Find the task for the other variant, if it exists

	jsonForTask, err := model.GetTaskJSONForVariant(t.Version, variantId, taskName, name)

	if err != nil {
		if err == mgo.ErrNotFound {
			plugin.WriteJSON(w, http.StatusNotFound, nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		plugin.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTask.Data)
}
