package model

import (
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	TaskJSONCollection = "json"
)

type TaskJSON struct {
	Name                string                 `bson:"name" json:"name"`
	TaskName            string                 `bson:"task_name" json:"task_name"`
	ProjectId           string                 `bson:"project_id" json:"project_id"`
	TaskId              string                 `bson:"task_id" json:"task_id"`
	BuildId             string                 `bson:"build_id" json:"build_id"`
	Variant             string                 `bson:"variant" json:"variant"`
	VersionId           string                 `bson:"version_id" json:"version_id"`
	CreateTime          time.Time              `bson:"create_time" json:"create_time"`
	IsPatch             bool                   `bson:"is_patch" json:"is_patch"`
	RevisionOrderNumber int                    `bson:"order" json:"order"`
	Revision            string                 `bson:"revision" json:"revision"`
	Data                map[string]interface{} `bson:"data" json:"data"`
	Tag                 string                 `bson:"tag" json:"tag"`
}

type TagContainer struct {
	Tag string `bson:"_id" json:"tag"`
}

type TagMetaDataContainer struct {
	Created  time.Time `bson:"created" json:"created"`
	Revision string    `bson:"revision" json:"revision"`
}

var (
	TagMetaCreatedKey  = bsonutil.MustHaveTag(TagMetaDataContainer{}, "Created")
	TagMetaRevisionKey = bsonutil.MustHaveTag(TagMetaDataContainer{}, "Revision")
)

type TagWithMetaContainer struct {
	Name string               `bson:"_id" json:"name"`
	Obj  TagMetaDataContainer `bson:"obj" json:"obj"`
}

var (
	TagNameKey = bsonutil.MustHaveTag(TagWithMetaContainer{}, "Name")
	TagObjKey  = bsonutil.MustHaveTag(TagWithMetaContainer{}, "Obj")
)

var (
	// BSON fields for the TaskJSON struct
	TaskJSONNameKey                = bsonutil.MustHaveTag(TaskJSON{}, "Name")
	TaskJSONTaskNameKey            = bsonutil.MustHaveTag(TaskJSON{}, "TaskName")
	TaskJSONProjectIdKey           = bsonutil.MustHaveTag(TaskJSON{}, "ProjectId")
	TaskJSONTaskIdKey              = bsonutil.MustHaveTag(TaskJSON{}, "TaskId")
	TaskJSONBuildIdKey             = bsonutil.MustHaveTag(TaskJSON{}, "BuildId")
	TaskJSONVariantKey             = bsonutil.MustHaveTag(TaskJSON{}, "Variant")
	TaskJSONVersionIdKey           = bsonutil.MustHaveTag(TaskJSON{}, "VersionId")
	TaskJSONCreateTimeKey          = bsonutil.MustHaveTag(TaskJSON{}, "CreateTime")
	TaskJSONIsPatchKey             = bsonutil.MustHaveTag(TaskJSON{}, "IsPatch")
	TaskJSONRevisionOrderNumberKey = bsonutil.MustHaveTag(TaskJSON{}, "RevisionOrderNumber")
	TaskJSONRevisionKey            = bsonutil.MustHaveTag(TaskJSON{}, "Revision")
	TaskJSONDataKey                = bsonutil.MustHaveTag(TaskJSON{}, "Data")
	TaskJSONTagKey                 = bsonutil.MustHaveTag(TaskJSON{}, "Tag")
)

// Returns distinct list of all tags for given `projectId`
func GetDistinctTagNames(projectId string) ([]TagWithMetaContainer, error) {
	out := []TagWithMetaContainer{}

	err := db.Aggregate(TaskJSONCollection, []bson.M{
		{"$match": bson.M{
			TaskJSONProjectIdKey: projectId,
			TaskJSONTagKey:       bson.M{"$exists": true, "$ne": ""}}},
		{"$group": bson.M{
			"_id": "$" + TaskJSONTagKey,
			"obj": bson.M{"$first": bson.M{
				"created":  "$$ROOT." + TaskJSONCreateTimeKey,
				"revision": "$$ROOT." + TaskJSONRevisionKey,
			}},
		}},
		{"$sort": bson.M{"obj.created": -1}},
	}, &out)

	if err != nil {
		return nil, errors.Wrap(err, "An error occured during db query execution")
	}

	return out, nil
}

// GetTags finds TaskJSONs that have tags in the project associated with a
// given task.
func GetTaskJSONTags(taskId string) ([]TagContainer, error) {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return nil, err
	}
	tags := []TagContainer{}
	err = db.Aggregate(TaskJSONCollection, []bson.M{
		{"$match": bson.M{
			TaskJSONProjectIdKey: t.Project,
			TaskJSONTagKey:       bson.M{"$exists": true, "$ne": ""}}},
		{"$project": bson.M{TaskJSONTagKey: 1}},
		{"$group": bson.M{"_id": "$tag"}},
	}, &tags)
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// GetTaskById fetches a JSONTask with the corresponding task id.
func GetTaskJSONById(taskId, name string) (TaskJSON, error) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(TaskJSONCollection, db.Query(bson.M{TaskJSONTaskIdKey: taskId, TaskJSONNameKey: name}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

// GetTaskJSONForVariant finds a task by name and variant and finds
// the document in the json collection associated with that task's id.
func GetTaskJSONForVariant(version, variantId, taskName, name string) (TaskJSON, error) {
	foundTask, err := task.FindOne(db.Query(bson.M{task.VersionKey: version, task.BuildVariantKey: variantId,
		task.DisplayNameKey: taskName}))
	if err != nil {
		return TaskJSON{}, err
	}

	var jsonForTask TaskJSON
	err = db.FindOneQ(TaskJSONCollection, db.Query(bson.M{TaskJSONTaskIdKey: foundTask.Id, TaskJSONNameKey: name}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

// InsertTask creates a TaskJSON document in the plugin's collection.
func InsertTaskJSON(t *task.Task, name string, data map[string]interface{}) error {
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
		IsPatch:             evergreen.IsPatchRequester(t.Requester),
	}
	_, err := db.Upsert(TaskJSONCollection, bson.M{TaskJSONTaskIdKey: t.Id, TaskJSONNameKey: name}, jsonBlob)

	return errors.WithStack(err)
}

// GetTaskByName finds a taskdata document by using the name of the task
// and other identifying information.
func GetTaskJSONByName(version, buildId, taskName, name string) (TaskJSON, error) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(TaskJSONCollection, db.Query(
		bson.M{
			TaskJSONVersionIdKey: version,
			TaskJSONBuildIdKey:   buildId,
			TaskJSONNameKey:      name,
			TaskJSONTaskNameKey:  taskName}),
		&jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

// GetTaskJSONCommit gets the task data associated with a particular task by using
// the commit hash to find the data.
func GetTaskJSONCommit(projectId, revision, variant, taskName, name string) (TaskJSON, error) {
	var jsonForTask TaskJSON
	err := db.FindOneQ(TaskJSONCollection,
		db.Query(bson.M{
			TaskJSONProjectIdKey: projectId,
			TaskJSONRevisionKey: bson.RegEx{
				Pattern: "^" + regexp.QuoteMeta(revision),
				Options: "i", // make it case insensitive
			},
			TaskJSONVariantKey:  variant,
			TaskJSONTaskNameKey: taskName,
			TaskJSONNameKey:     name,
			TaskJSONIsPatchKey:  false,
		}), &jsonForTask)
	if err != nil {
		return TaskJSON{}, err
	}
	return jsonForTask, nil
}

// GetTaskJSONByTag finds a TaskJSON by a tag
func GetTaskJSONByTag(projectId, tag, variant, taskName, name string) (*TaskJSON, error) {
	jsonForTask := &TaskJSON{}
	err := db.FindOneQ(TaskJSONCollection,
		db.Query(bson.M{"project_id": projectId,
			TaskJSONTagKey:      tag,
			TaskJSONVariantKey:  variant,
			TaskJSONTaskNameKey: taskName,
			TaskJSONNameKey:     name,
		}), jsonForTask)
	if err != nil {
		return nil, err
	}
	return jsonForTask, nil
}

// GetTagsForTask gets all of the tasks with tags for the given task name and
// build variant.
func GetTaskJSONTagsForTask(project, buildVariant, taskName, name string) ([]TaskJSON, error) {
	tagged := []TaskJSON{}
	jsonQuery := db.Query(bson.M{
		TaskJSONProjectIdKey: project,
		TaskJSONVariantKey:   buildVariant,
		TaskJSONTaskNameKey:  taskName,
		TaskJSONTagKey:       bson.M{"$exists": true, "$ne": ""},
		TaskJSONNameKey:      name})
	err := db.FindAllQ(TaskJSONCollection, jsonQuery, &tagged)
	if err != nil {
		return nil, err
	}
	return tagged, nil
}

func DeleteTaskJSONTagFromTask(taskId, name string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = db.UpdateAll(TaskJSONCollection,
		bson.M{TaskJSONVersionIdKey: t.Version, TaskJSONNameKey: name},
		bson.M{"$unset": bson.M{TaskJSONTagKey: 1}})

	return errors.WithStack(err)
}

func SetTaskJSONTagForTask(taskId, name, tag string) error {
	t, err := task.FindOne(task.ById(taskId))
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = db.UpdateAll(TaskJSONCollection,
		bson.M{TaskJSONVersionIdKey: t.Version, TaskJSONNameKey: name},
		bson.M{"$set": bson.M{TaskJSONTagKey: tag}})

	return errors.WithStack(err)
}

func GetTaskJSONHistory(t *task.Task, name string) ([]TaskJSON, error) {
	var t2 *task.Task = t
	var err error
	if evergreen.IsPatchRequester(t.Requester) {
		t2, err = t.FindTaskOnBaseCommit()
		if err != nil {
			return nil, err
		}
		if t2 == nil {
			return nil, errors.New("could not find task on base commit")
		}
		t.RevisionOrderNumber = t2.RevisionOrderNumber
	}

	before := []TaskJSON{}
	jsonQuery := db.Query(bson.M{
		TaskJSONProjectIdKey:           t.Project,
		TaskJSONVariantKey:             t.BuildVariant,
		TaskJSONRevisionOrderNumberKey: bson.M{"$lte": t.RevisionOrderNumber},
		TaskJSONTaskNameKey:            t.DisplayName,
		TaskJSONIsPatchKey:             false,
		TaskJSONNameKey:                name,
	})
	jsonQuery = jsonQuery.Sort([]string{"-order"}).Limit(100)
	err = db.FindAllQ(TaskJSONCollection, jsonQuery, &before)
	if err != nil {
		return nil, err
	}
	//reverse order of "before" because we had to sort it backwards to apply the limit correctly:
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}

	after := []TaskJSON{}
	jsonAfterQuery := db.Query(bson.M{
		TaskJSONProjectIdKey:           t.Project,
		TaskJSONVariantKey:             t.BuildVariant,
		TaskJSONRevisionOrderNumberKey: bson.M{"$gt": t.RevisionOrderNumber},
		TaskJSONTaskNameKey:            t.DisplayName,
		TaskJSONIsPatchKey:             false,
		TaskJSONNameKey:                name}).Sort([]string{"order"}).Limit(100)
	err = db.FindAllQ(TaskJSONCollection, jsonAfterQuery, &after)
	if err != nil {
		return nil, err
	}

	//concatenate before + after
	before = append(before, after...)

	// if our task was a patch, replace the base commit's info in the history with the patch
	if evergreen.IsPatchRequester(t.Requester) {
		before, err = fixPatchInHistory(t.Id, t2, before)
		if err != nil {
			return nil, err
		}
	}
	return before, nil
}

func fixPatchInHistory(taskId string, base *task.Task, history []TaskJSON) ([]TaskJSON, error) {
	var jsonForTask *TaskJSON
	err := db.FindOneQ(TaskJSONCollection, db.Query(bson.M{TaskJSONTaskIdKey: taskId}), &jsonForTask)
	if err != nil {
		return nil, err
	}
	if base != nil {
		jsonForTask.RevisionOrderNumber = base.RevisionOrderNumber
	}
	if jsonForTask == nil {
		return history, nil
	}

	found := false
	for i, item := range history {
		if item.Revision == base.Revision {
			history[i] = *jsonForTask
			found = true
		}
	}
	// if found is false, it means we don't have json on the base commit, so it was
	// not replaced and we must add it explicitly
	if !found {
		history = append(history, *jsonForTask)
	}
	return history, nil
}
