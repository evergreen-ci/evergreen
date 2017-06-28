package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
)

func uiGetCommit(w http.ResponseWriter, r *http.Request) {
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
		plugin.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTask)
}
