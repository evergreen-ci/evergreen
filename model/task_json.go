package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/anser/bsonutil"
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

var (
	// BSON fields for the TaskJSON struct
	TaskJSONNameKey   = bsonutil.MustHaveTag(TaskJSON{}, "Name")
	TaskJSONTaskIdKey = bsonutil.MustHaveTag(TaskJSON{}, "TaskId")
)

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
