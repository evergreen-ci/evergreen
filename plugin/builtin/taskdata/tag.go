package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	mgo "gopkg.in/mgo.v2"
)

func uiGetTags(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	tags, err := model.GetTaskJSONTags(taskId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, tags)
}

// handleTaskTags will update the TaskJSON's tags depending on the request.
func uiHandleTaskTag(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	name := mux.Vars(r)["name"]

	var err error
	switch r.Method {
	case evergreen.MethodDelete:
		err = model.DeleteTaskJSONTagFromTask(taskId, name)
	case evergreen.MethodPost:
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

	plugin.WriteJSON(w, http.StatusOK, "")
}

func apiGetTagsForTask(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]
	tagged, err := model.GetTaskJSONTagsForTask(t.Project, t.BuildVariant, taskName, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, tagged)
}

func uiGetTaskJSONByTag(w http.ResponseWriter, r *http.Request) {
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
		plugin.WriteJSON(w, http.StatusOK, jsonForTask)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTask)
}
