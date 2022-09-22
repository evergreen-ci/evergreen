package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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

type TaskJSONTagMeta struct {
	Created   time.Time `bson:"created" json:"created"`
	Revision  string    `bson:"revision" json:"revision"`
	VersionId string    `bson:"version_id" json:"version_id"`
}

var (
	TaskJSONTagMetaCreatedKey   = bsonutil.MustHaveTag(TaskJSONTagMeta{}, "Created")
	TaskJSONTagMetaRevisionKey  = bsonutil.MustHaveTag(TaskJSONTagMeta{}, "Revision")
	TaskJSONTagMetaVersionIdKey = bsonutil.MustHaveTag(TaskJSONTagMeta{}, "VersionId")
)

type TaskJSONTag struct {
	Name string          `bson:"_id" json:"name"`
	Obj  TaskJSONTagMeta `bson:"obj" json:"obj"`
}

var (
	TaskJSONTagNameKey = bsonutil.MustHaveTag(TaskJSONTag{}, "Name")
	TaskJSONTagObjKey  = bsonutil.MustHaveTag(TaskJSONTag{}, "Obj")
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

// TODO EVG-17643 investigate if we can remove this function
func GetTaskJSONHistory(t *task.Task, name string) ([]TaskJSON, error) {
	var baseVersion *Version
	var err error

	revisionOrderNumber := t.RevisionOrderNumber
	if evergreen.IsPatchRequester(t.Requester) {
		baseVersion, err = VersionFindOne(BaseVersionByProjectIdAndRevision(t.Project, t.Revision))
		if err != nil {
			return nil, errors.Wrapf(err, "finding base version for revision '%s'", t.Revision)
		}
		if baseVersion == nil {
			return nil, errors.Errorf("base version does not exist for revision '%s'", t.Revision)
		}
		revisionOrderNumber = baseVersion.RevisionOrderNumber
	}

	before := []TaskJSON{}
	jsonQuery := db.Query(bson.M{
		TaskJSONProjectIdKey:           t.Project,
		TaskJSONVariantKey:             t.BuildVariant,
		TaskJSONRevisionOrderNumberKey: bson.M{"$lte": revisionOrderNumber},
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
		TaskJSONRevisionOrderNumberKey: bson.M{"$gt": revisionOrderNumber},
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
		before, err = fixPatchInHistory(t, baseVersion, before)
		if err != nil {
			return nil, err
		}
	}
	return before, nil
}

func fixPatchInHistory(t *task.Task, baseVersion *Version, history []TaskJSON) ([]TaskJSON, error) {
	var jsonForTask *TaskJSON
	if err := db.FindOneQ(TaskJSONCollection, db.Query(bson.M{TaskJSONTaskIdKey: t.Id}), &jsonForTask); err != nil {
		if adb.ResultsNotFound(err) {
			return history, nil
		}
		return nil, errors.Wrap(err, "getting JSON for patch task")
	}
	jsonForTask.RevisionOrderNumber = baseVersion.RevisionOrderNumber

	found := false
	for i, item := range history {
		if item.Revision == baseVersion.Revision {
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
