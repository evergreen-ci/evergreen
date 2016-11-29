package taskdata

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"regexp"
)

func uiGetCommit(w http.ResponseWriter, r *http.Request) {
	projectId := mux.Vars(r)["project_id"]
	revision := mux.Vars(r)["revision"]
	variant := mux.Vars(r)["variant"]
	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]
	jsonForTask, err := GetCommit(projectId, revision, variant, taskName, name)
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

// GetCommit gets the task data associated with a particular task by using
// the commit hash to find the data.
func GetCommit(projectId, revision, variant, taskName, name string) (TaskJSON, error) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(collection,
		db.Query(bson.M{ProjectIdKey: projectId,
			RevisionKey: bson.RegEx{"^" + regexp.QuoteMeta(revision), "i"}, // make it case insensitive
			VariantKey:  variant,
			TaskNameKey: taskName,
			NameKey:     name,
			IsPatchKey:  false,
		}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}
