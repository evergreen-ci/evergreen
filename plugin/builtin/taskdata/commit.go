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

func getCommit(w http.ResponseWriter, r *http.Request) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(collection,
		db.Query(bson.M{ProjectIdKey: mux.Vars(r)["project_id"],
			RevisionKey: bson.RegEx{"^" + regexp.QuoteMeta(mux.Vars(r)["revision"]), "i"},
			VariantKey:  mux.Vars(r)["variant"],
			TaskNameKey: mux.Vars(r)["task_name"],
			NameKey:     mux.Vars(r)["name"],
			IsPatchKey:  false,
		}), &jsonForTask)
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
