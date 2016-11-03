package taskdata

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/db"
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

// findTasksForVersion sends back the list of TaskJSON documents associated with a version id.
func findTasksForVersion(versionId, name string) ([]TaskJSON, error) {
	var jsonForTasks []TaskJSON
	err := db.FindAllQ(collection, db.Query(bson.M{VersionIdKey: versionId,
		NameKey: name}).WithFields(DataKey, TaskIdKey, TaskNameKey), &jsonForTasks)
	if err != nil {
		if err != mgo.ErrNotFound {
			return nil, err
		}
		return nil, err
	}

	return jsonForTasks, err
}

// getTasksForVersion sends back the list of TaskJSON documents associated with a version id.
func getTasksForVersion(w http.ResponseWriter, r *http.Request) {
	jsonForTasks, err := findTasksForVersion(mux.Vars(r)["version_id"], mux.Vars(r)["name"])
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

// getTasksForLatestVersion sends back the TaskJSON data associated with the latest version.
func getTasksForLatestVersion(w http.ResponseWriter, r *http.Request) {
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
	pipeline := []bson.M{
		// match on name and project
		{"$match": bson.M{
			NameKey:      name,
			ProjectIdKey: project,
		}},

		// sort on the revision number
		{"$sort": bson.M{
			RevisionOrderNumberKey: -1,
		}},

		// group by version id
		{"$group": bson.M{
			"_id": bson.M{
				"r":   "$" + RevisionOrderNumberKey,
				"vid": "$" + VersionIdKey,
			},
			"t": bson.M{
				"$push": bson.M{
					"d":    "$" + DataKey,
					"t_id": "$" + TaskIdKey,
					"tn":   "$" + TaskNameKey,
					"var":  "$" + VariantKey,
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
	err = db.Aggregate(collection, pipeline, &tasksForVersions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(tasksForVersions) == 0 {
		http.Error(w, "no tasks found", http.StatusNotFound)
		return
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if v == nil {
		http.Error(w, "{}", http.StatusNotFound)
		return
	}

	commitInfo := CommitInfo{
		Author:     v.Author,
		Message:    v.Message,
		CreateTime: v.CreateTime,
		Revision:   v.Revision,
		VersionId:  v.Id,
	}

	data := VersionData{currentVersion.Tasks, commitInfo, lastRevision}
	plugin.WriteJSON(w, http.StatusOK, data)
}
