package service

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
)

func (as *APIServer) getTaskJSONTagsForTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	vars := gimlet.GetVars(r)
	taskName := vars["task_name"]
	name := vars["name"]

	tagged, err := model.GetTaskJSONTagsForTask(t.Project, t.BuildVariant, taskName, name)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, tagged)
}

func (as *APIServer) getTaskJSONTaskHistory(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	history, err := model.GetTaskJSONHistory(t, gimlet.GetVars(r)["name"])
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, history)
}

func (as *APIServer) insertTaskJSON(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	name := gimlet.GetVars(r)["name"]
	rawData := map[string]interface{}{}

	if err := utility.ReadJSON(utility.NewRequestReader(r), &rawData); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if err := model.InsertTaskJSON(t, name, rawData); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, "ok")
}

func (as *APIServer) getTaskJSONByName(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	vars := gimlet.GetVars(r)
	name := vars["name"]
	taskName := vars["task_name"]

	jsonForTask, err := model.GetTaskJSONByName(t.Version, t.BuildId, taskName, name)
	if err != nil {
		if adb.ResultsNotFound(err) {
			gimlet.WriteJSONResponse(w, http.StatusNotFound, nil)
			return
		}

		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		gimlet.WriteJSON(w, jsonForTask)
		return
	}

	gimlet.WriteJSON(w, jsonForTask.Data)
}

// apiGetTaskForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func (as *APIServer) getTaskJSONForVariant(w http.ResponseWriter, r *http.Request) {
	t := GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	vars := gimlet.GetVars(r)
	name := vars["name"]
	taskName := vars["task_name"]
	variantId := vars["variant"]
	// Find the task for the other variant, if it exists

	jsonForTask, err := model.GetTaskJSONForVariant(t.Version, variantId, taskName, name)

	if err != nil {
		if adb.ResultsNotFound(err) {
			gimlet.WriteJSONResponse(w, http.StatusNotFound, nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(r.FormValue("full")) != 0 { // if specified, include the json data's container as well
		gimlet.WriteJSON(w, jsonForTask)
		return
	}
	gimlet.WriteJSON(w, jsonForTask.Data)
}
