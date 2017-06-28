package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
)

func uiGetTaskHistory(w http.ResponseWriter, r *http.Request) {
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
	plugin.WriteJSON(w, http.StatusOK, history)
}
