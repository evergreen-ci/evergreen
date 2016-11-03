package taskdata

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
)

// getTaskById sends back a JSONTask with the corresponding task id.
func getTaskById(w http.ResponseWriter, r *http.Request) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(collection, db.Query(bson.M{TaskIdKey: mux.Vars(r)["task_id"], NameKey: mux.Vars(r)["name"]}), &jsonForTask)
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

// getTask finds a json document by using thex task that is in the plugin.
func getTaskByName(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]

	var jsonForTask TaskJSON
	err := db.FindOneQ(collection, db.Query(bson.M{VersionIdKey: t.Version, BuildIdKey: t.BuildId, NameKey: name,
		TaskNameKey: taskName}), &jsonForTask)
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

// getTaskForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func getTaskForVariant(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]
	variantId := mux.Vars(r)["variant"]
	// Find the task for the other variant, if it exists
	ts, err := task.Find(db.Query(bson.M{task.VersionKey: t.Version, task.BuildVariantKey: variantId,
		task.DisplayNameKey: taskName}).Limit(1))
	if err != nil {
		if err == mgo.ErrNotFound {
			plugin.WriteJSON(w, http.StatusNotFound, nil)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(ts) == 0 {
		plugin.WriteJSON(w, http.StatusNotFound, nil)
		return
	}
	otherVariantTask := ts[0]

	var jsonForTask TaskJSON
	err = db.FindOneQ(collection, db.Query(bson.M{TaskIdKey: otherVariantTask.Id, NameKey: name}), &jsonForTask)
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

// getTaskByTag returns a TaskJSON with a specific task name and tag
func getTaskByTag(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	tagged := []TaskJSON{}
	jsonQuery := db.Query(bson.M{
		ProjectIdKey: t.Project,
		VariantKey:   t.BuildVariant,
		TaskNameKey:  mux.Vars(r)["task_name"],
		TagKey:       bson.M{"$exists": true, "$ne": ""},
		NameKey:      mux.Vars(r)["name"]})
	err := db.FindAllQ(collection, jsonQuery, &tagged)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, tagged)
}

// insertTask creates a TaskJSON document with the data sent in the request body.
func insertTask(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	rawData := map[string]interface{}{}
	err := util.ReadJSONInto(r.Body, &rawData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonBlob := TaskJSON{
		TaskId:              t.Id,
		TaskName:            t.DisplayName,
		Name:                name,
		BuildId:             t.BuildId,
		Variant:             t.BuildVariant,
		ProjectId:           t.Project,
		VersionId:           t.Version,
		CreateTime:          t.CreateTime,
		Revision:            t.Revision,
		RevisionOrderNumber: t.RevisionOrderNumber,
		Data:                rawData,
		IsPatch:             t.Requester == evergreen.PatchVersionRequester,
	}
	_, err = db.Upsert(collection, bson.M{TaskIdKey: t.Id, NameKey: name}, jsonBlob)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, "ok")
	return
}
