package taskdata

import (
	"time"

	"github.com/evergreen-ci/evergreen/db/bsonutil"
)

const (
	collection = "json"
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
	NameKey                = bsonutil.MustHaveTag(TaskJSON{}, "Name")
	TaskNameKey            = bsonutil.MustHaveTag(TaskJSON{}, "TaskName")
	ProjectIdKey           = bsonutil.MustHaveTag(TaskJSON{}, "ProjectId")
	TaskIdKey              = bsonutil.MustHaveTag(TaskJSON{}, "TaskId")
	BuildIdKey             = bsonutil.MustHaveTag(TaskJSON{}, "BuildId")
	VariantKey             = bsonutil.MustHaveTag(TaskJSON{}, "Variant")
	VersionIdKey           = bsonutil.MustHaveTag(TaskJSON{}, "VersionId")
	CreateTimeKey          = bsonutil.MustHaveTag(TaskJSON{}, "CreateTime")
	IsPatchKey             = bsonutil.MustHaveTag(TaskJSON{}, "IsPatch")
	RevisionOrderNumberKey = bsonutil.MustHaveTag(TaskJSON{}, "RevisionOrderNumber")
	RevisionKey            = bsonutil.MustHaveTag(TaskJSON{}, "Revision")
	DataKey                = bsonutil.MustHaveTag(TaskJSON{}, "Data")
	TagKey                 = bsonutil.MustHaveTag(TaskJSON{}, "Tag")
)
