package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
)

// uiGetTaskById sends back a JSONTask with the corresponding task id.
func uiGetTaskById(w http.ResponseWriter, r *http.Request) {
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
	plugin.WriteJSON(w, http.StatusOK, jsonForTask)
}
