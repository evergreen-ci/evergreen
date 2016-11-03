package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// getTags finds TaskJSONs that have tags
func getTags(w http.ResponseWriter, r *http.Request) {
	t, err := task.FindOne(task.ById(mux.Vars(r)["task_id"]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tags := []struct {
		Tag string `bson:"_id" json:"tag"`
	}{}
	err = db.Aggregate(collection, []bson.M{
		{"$match": bson.M{ProjectIdKey: t.Project, TagKey: bson.M{"$exists": true, "$ne": ""}}},
		{"$project": bson.M{TagKey: 1}}, bson.M{"$group": bson.M{"_id": "$tag"}},
	}, &tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, tags)
}

// handleTaskTags will update the TaskJSON's tags depending on the request.
func handleTaskTag(w http.ResponseWriter, r *http.Request) {
	t, err := task.FindOne(task.ById(mux.Vars(r)["task_id"]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if r.Method == "DELETE" {
		if _, err = db.UpdateAll(collection,
			bson.M{VersionIdKey: t.Version, NameKey: mux.Vars(r)["name"]},
			bson.M{"$unset": bson.M{TagKey: 1}}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		plugin.WriteJSON(w, http.StatusOK, "")
	}
	inTag := struct {
		Tag string `json:"tag"`
	}{}
	err = util.ReadJSONInto(r.Body, &inTag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(inTag.Tag) == 0 {
		http.Error(w, "tag must not be blank", http.StatusBadRequest)
		return
	}

	_, err = db.UpdateAll(collection,
		bson.M{VersionIdKey: t.Version, NameKey: mux.Vars(r)["name"]},
		bson.M{"$set": bson.M{TagKey: inTag.Tag}})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, "")
}

// getTaskJSONByTag finds a TaskJSON by a tag
func getTaskJSONByTag(w http.ResponseWriter, r *http.Request) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(collection,
		db.Query(bson.M{"project_id": mux.Vars(r)["project_id"],
			TagKey:      mux.Vars(r)["tag"],
			VariantKey:  mux.Vars(r)["variant"],
			TaskNameKey: mux.Vars(r)["task_name"],
			NameKey:     mux.Vars(r)["name"],
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
