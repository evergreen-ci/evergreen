package taskdata

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// CommitInfo represents the information about the commit
// associated with the version of a given project. This is displayed
// at the top of each header for each project.
type CommitInfo struct {
	Author     string    `json:"author"`
	Message    string    `json:"message"`
	CreateTime time.Time `json:"create_time"`
	Revision   string    `json:"revision"`
	VersionId  string    `json:"version_id"`
}

// TaskForVersionId contains the id information about the group of tasks
// including the version id and the revision order number.
type TaskForVersionId struct {
	VersionId           string `bson:"vid"`
	RevisionOrderNumber int    `bson:"r"`
}

// TaskInfo contains the relevant TaskJSON information the tasks associated with a version.
type TaskInfo struct {
	TaskId   string      `bson:"t_id" json:"task_id"`
	TaskName string      `bson:"tn" json:"task_name"`
	Variant  string      `bson:"var" json:"variant"`
	Data     interface{} `bson:"d" json:"data"`
}

// TasksForVersion is a list of tasks that are associated with a given version id.
type TasksForVersion struct {
	Id    TaskForVersionId `bson:"_id" json:"id"`
	Tasks []TaskInfo       `bson:"t" json:"tasks"`
}

// VersionData includes the TaskInfo, and Commit information for a given version.
type VersionData struct {
	JSONTasks    []TaskInfo `json:"json_tasks"`
	Commit       CommitInfo `json:"commit_info"`
	LastRevision bool       `json:"last_revision"`
}

// getVersion returns a StatusOK if the route is hit
func getVersion(w http.ResponseWriter, r *http.Request) {
	plugin.WriteJSON(w, http.StatusOK, "1")
}

// GetTasksForVersion sends back the list of TaskJSON documents associated with a version id.
func GetTasksForVersion(versionId, name string) ([]model.TaskJSON, error) {
	var jsonForTasks []model.TaskJSON
	err := db.FindAllQ(model.TaskJSONCollection,
		db.Query(bson.M{
			model.TaskJSONVersionIdKey: versionId,
			model.TaskJSONNameKey:      name}).WithFields(model.TaskJSONDataKey, model.TaskJSONTaskIdKey, model.TaskJSONTaskNameKey), &jsonForTasks)
	if err != nil {
		if err != mgo.ErrNotFound {
			return nil, err
		}
		return nil, err
	}

	return jsonForTasks, err
}

func uiGetTasksForVersion(w http.ResponseWriter, r *http.Request) {
	jsonForTasks, err := GetTasksForVersion(mux.Vars(r)["version_id"], mux.Vars(r)["name"])
	if jsonForTasks == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	plugin.WriteJSON(w, http.StatusOK, jsonForTasks)
	return
}

func uiGetTasksForLatestVersion(w http.ResponseWriter, r *http.Request) {
	project := mux.Vars(r)["project_id"]
	name := mux.Vars(r)["name"]
	skip, err := util.GetIntValue(r, "skip", 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if skip < 0 {
		http.Error(w, "negative skip", http.StatusBadRequest)
		return
	}

	data, err := GetTasksForLatestVersion(project, name, skip)
	if err != nil {
		if err == mgo.ErrNotFound {
			http.Error(w, "{}", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	plugin.WriteJSON(w, http.StatusOK, data)
}

// GetTasksForLatestVersion sends back the TaskJSON data associated with the latest version.
func GetTasksForLatestVersion(project, name string, skip int) (*VersionData, error) {
	pipeline := []bson.M{
		// match on name and project
		{"$match": bson.M{
			model.TaskJSONNameKey:      name,
			model.TaskJSONProjectIdKey: project,
		}},

		// sort on the revision number
		{"$sort": bson.M{
			model.TaskJSONRevisionOrderNumberKey: -1,
		}},

		// group by version id
		{"$group": bson.M{
			"_id": bson.M{
				"r":   "$" + model.TaskJSONRevisionOrderNumberKey,
				"vid": "$" + model.TaskJSONVersionIdKey,
			},
			"t": bson.M{
				"$push": bson.M{
					"d":    "$" + model.TaskJSONDataKey,
					"t_id": "$" + model.TaskJSONTaskIdKey,
					"tn":   "$" + model.TaskJSONTaskNameKey,
					"var":  "$" + model.TaskJSONVariantKey,
				},
			},
		}},

		// sort on the _id
		{"$sort": bson.M{
			"_id.r": -1,
		}},

		{"$skip": skip},
		{"$limit": 2},
	}

	tasksForVersions := []TasksForVersion{}
	err := db.Aggregate(model.TaskJSONCollection, pipeline, &tasksForVersions)
	if err != nil {
		return nil, err
	}
	if len(tasksForVersions) == 0 {
		return nil, mgo.ErrNotFound
	}

	// we default have another revision
	lastRevision := false
	currentVersion := tasksForVersions[0]

	// if there is only one version, then we are at the last revision.
	if len(tasksForVersions) < 2 {
		lastRevision = true
	}

	// get the version commit info
	v, err := version.FindOne(version.ById(currentVersion.Id.VersionId))
	if err != nil {
		return nil, err
	}

	commitInfo := CommitInfo{
		Author:     v.Author,
		Message:    v.Message,
		CreateTime: v.CreateTime,
		Revision:   v.Revision,
		VersionId:  v.Id,
	}
	data := &VersionData{currentVersion.Tasks, commitInfo, lastRevision}
	return data, nil

}
