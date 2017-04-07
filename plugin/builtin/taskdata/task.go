package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// uiGetTaskById sends back a JSONTask with the corresponding task id.
func uiGetTaskById(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	name := mux.Vars(r)["name"]
	jsonForTask, err := GetTaskById(taskId, name)
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

// GetTaskById fetches a JSONTask with the corresponding task id.
func GetTaskById(taskId, name string) (TaskJSON, error) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(collection, db.Query(bson.M{TaskIdKey: taskId, NameKey: name}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

func apiGetTaskByName(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]

	jsonForTask, err := GetTaskByName(t.Version, t.BuildId, taskName, name)
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

// GetTaskByName finds a taskdata document by using the name of the task
// and other identifying information.
func GetTaskByName(version, buildId, taskName, name string) (TaskJSON, error) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(collection, db.Query(bson.M{VersionIdKey: version, BuildIdKey: buildId, NameKey: name,
		TaskNameKey: taskName}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

// apiGetTaskForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func apiGetTaskForVariant(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	name := mux.Vars(r)["name"]
	taskName := mux.Vars(r)["task_name"]
	variantId := mux.Vars(r)["variant"]
	// Find the task for the other variant, if it exists

	jsonForTask, err := GetTaskForVariant(t.Version, variantId, taskName, name)

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

// GetTaskForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func GetTaskForVariant(version, variantId, taskName, name string) (TaskJSON, error) {
	foundTask, err := task.FindOne(db.Query(bson.M{task.VersionKey: version, task.BuildVariantKey: variantId,
		task.DisplayNameKey: taskName}))
	if err != nil {
		return TaskJSON{}, err
	}

	var jsonForTask TaskJSON
	err = db.FindOneQ(collection, db.Query(bson.M{TaskIdKey: foundTask.Id, NameKey: name}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

func apiInsertTask(w http.ResponseWriter, r *http.Request) {
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
	err = InsertTask(t, name, rawData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, "ok")
	return
}

// InsertTask creates a TaskJSON document in the plugin's collection.
func InsertTask(t *task.Task, name string, data map[string]interface{}) error {
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
		Data:                data,
		IsPatch:             t.Requester == evergreen.PatchVersionRequester,
	}
	_, err := db.Upsert(collection, bson.M{TaskIdKey: t.Id, NameKey: name}, jsonBlob)

	return errors.WithStack(err)
}
