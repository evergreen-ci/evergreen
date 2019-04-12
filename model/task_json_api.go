package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// CommitInfo represents the information about the commit
// associated with the version of a given project. This is displayed
// at the top of each header for each project.
type PerfCommitInfo struct {
	Author     string    `json:"author"`
	Message    string    `json:"message"`
	CreateTime time.Time `json:"create_time"`
	Revision   string    `json:"revision"`
	VersionId  string    `json:"version_id"`
}

// TaskForVersionId contains the id information about the group of tasks
// including the version id and the revision order number.
type PerfTaskForVersionId struct {
	VersionId           string `bson:"vid"`
	RevisionOrderNumber int    `bson:"r"`
}

// TaskInfo contains the relevant TaskJSON information the tasks associated with a Version
type PerfTaskInfo struct {
	TaskId   string      `bson:"t_id" json:"task_id"`
	TaskName string      `bson:"tn" json:"task_name"`
	Variant  string      `bson:"var" json:"variant"`
	Data     interface{} `bson:"d" json:"data"`
}

// TasksForVersion is a list of tasks that are associated with a given version id.
type PerfTasksForVersion struct {
	Id    PerfTaskForVersionId `bson:"_id" json:"id"`
	Tasks []PerfTaskInfo       `bson:"t" json:"tasks"`
}

// VersionData includes the TaskInfo, and Commit information for a given Version
type PerfVersionData struct {
	JSONTasks    []PerfTaskInfo `json:"json_tasks"`
	Commit       PerfCommitInfo `json:"commit_info"`
	LastRevision bool           `json:"last_revision"`
}

// getTasksForVersion sends back the list of TaskJSON documents associated with a version id.
func PerfGetTasksForVersion(versionId, name string) ([]TaskJSON, error) {
	var jsonForTasks []TaskJSON
	err := db.FindAllQ(TaskJSONCollection,
		db.Query(bson.M{
			TaskJSONVersionIdKey: versionId,
			TaskJSONNameKey:      name}).WithFields(TaskJSONDataKey, TaskJSONTaskIdKey, TaskJSONTaskNameKey), &jsonForTasks)
	if err != nil {
		if err != mgo.ErrNotFound {
			return nil, err
		}
		return nil, err
	}

	return jsonForTasks, err
}

// getTasksForLatestVersion sends back the TaskJSON data associated with the latest Version
func PerfGetTasksForLatestVersion(project, name string, skip int) (*PerfVersionData, error) {
	pipeline := []bson.M{
		// match on name and project
		{"$match": bson.M{
			TaskJSONNameKey:      name,
			TaskJSONProjectIdKey: project,
		}},

		// sort on the revision number
		{"$sort": bson.M{
			TaskJSONRevisionOrderNumberKey: -1,
		}},

		// group by version id
		{"$group": bson.M{
			"_id": bson.M{
				"r":   "$" + TaskJSONRevisionOrderNumberKey,
				"vid": "$" + TaskJSONVersionIdKey,
			},
			"t": bson.M{
				"$push": bson.M{
					"d":    "$" + TaskJSONDataKey,
					"t_id": "$" + TaskJSONTaskIdKey,
					"tn":   "$" + TaskJSONTaskNameKey,
					"var":  "$" + TaskJSONVariantKey,
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

	tasksForVersions := []PerfTasksForVersion{}
	err := db.Aggregate(TaskJSONCollection, pipeline, &tasksForVersions)
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
	v, err := VersionFindOne(VersionById(currentVersion.Id.VersionId))
	if err != nil {
		return nil, err
	}

	commitInfo := PerfCommitInfo{
		Author:     v.Author,
		Message:    v.Message,
		CreateTime: v.CreateTime,
		Revision:   v.Revision,
		VersionId:  v.Id,
	}
	data := &PerfVersionData{currentVersion.Tasks, commitInfo, lastRevision}
	return data, nil
}
