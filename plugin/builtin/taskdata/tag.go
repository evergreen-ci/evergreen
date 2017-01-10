package taskdata

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type TagContainer struct {
	Tag string `bson:"_id" json:"tag"`
}

func uiGetTags(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	tags, err := GetTags(taskId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, tags)
}

// GetTags finds TaskJSONs that have tags in the project associated with a
// given task.
func GetTags(taskId string) ([]TagContainer, error) {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return nil, err
	}
	tags := []TagContainer{}
	err = db.Aggregate(collection, []bson.M{
		{"$match": bson.M{ProjectIdKey: t.Project, TagKey: bson.M{"$exists": true, "$ne": ""}}},
		{"$project": bson.M{TagKey: 1}}, bson.M{"$group": bson.M{"_id": "$tag"}},
	}, &tags)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// handleTaskTags will update the TaskJSON's tags depending on the request.
func uiHandleTaskTag(w http.ResponseWriter, r *http.Request) {
	taskId := mux.Vars(r)["task_id"]
	name := mux.Vars(r)["name"]

	var err error
	switch r.Method {
	case evergreen.MethodDelete:
		err = DeleteTagFromTask(taskId, name)
	case evergreen.MethodPost:
		tc := TagContainer{}
		err = util.ReadJSONInto(r.Body, &tc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if tc.Tag == "" {
			http.Error(w, "tag must not be blank", http.StatusBadRequest)
			return
		}
		err = SetTagForTask(taskId, name, tc.Tag)
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

func DeleteTagFromTask(taskId, name string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
	}

	_, err = db.UpdateAll(collection,
		bson.M{VersionIdKey: t.Version, NameKey: name},
		bson.M{"$unset": bson.M{TagKey: 1}})
	if err != nil {
		return err
	}
	return nil
}

func SetTagForTask(taskId, name, tag string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return err
	}
	_, err = db.UpdateAll(collection,
		bson.M{VersionIdKey: t.Version, NameKey: name},
		bson.M{"$set": bson.M{TagKey: tag}})
	if err != nil {
		return err
	}
	return nil
}
func apiGetTagsForTask(w http.ResponseWriter, r *http.Request) {
	t := plugin.GetTask(r)
	if t == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]
	tagged, err := GetTagsForTask(t.Project, t.BuildVariant, taskName, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, tagged)
}

// GetTagsForTask gets all of the tasks with tags for the given task name and
// build variant.
func GetTagsForTask(project, buildVariant, taskName, name string) ([]TaskJSON, error) {
	tagged := []TaskJSON{}
	jsonQuery := db.Query(bson.M{
		ProjectIdKey: project,
		VariantKey:   buildVariant,
		TaskNameKey:  taskName,
		TagKey:       bson.M{"$exists": true, "$ne": ""},
		NameKey:      name})
	err := db.FindAllQ(collection, jsonQuery, &tagged)
	if err != nil {
		return nil, err
	}
	return tagged, nil
}

func uiGetTaskJSONByTag(w http.ResponseWriter, r *http.Request) {
	projectId := mux.Vars(r)["project_id"]
	tag := mux.Vars(r)["tag"]
	variant := mux.Vars(r)["variant"]
	taskName := mux.Vars(r)["task_name"]
	name := mux.Vars(r)["name"]

	jsonForTask, err := GetTaskJSONByTag(projectId, tag, variant, taskName, name)

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

// GetTaskJSONByTag finds a TaskJSON by a tag
func GetTaskJSONByTag(projectId, tag, variant, taskName, name string) (*TaskJSON, error) {
	jsonForTask := &TaskJSON{}
	err := db.FindOneQ(collection,
		db.Query(bson.M{"project_id": projectId,
			TagKey:      tag,
			VariantKey:  variant,
			TaskNameKey: taskName,
			NameKey:     name,
		}), jsonForTask)
	if err != nil {
		return nil, err
	}
	return jsonForTask, nil
}
