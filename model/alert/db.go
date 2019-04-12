package alert

import (
	"github.com/mongodb/anser/bsonutil"
)

const (
	// Collection is the name of the collection in MongoDB that stores alerts.
	Collection = "alerts"
)

var (
	IdKey          = bsonutil.MustHaveTag(AlertRequest{}, "Id")
	HostIdKey      = bsonutil.MustHaveTag(AlertRequest{}, "HostId")
	TriggerKey     = bsonutil.MustHaveTag(AlertRequest{}, "Trigger")
	QueueStatusKey = bsonutil.MustHaveTag(AlertRequest{}, "QueueStatus")
	TaskIdKey      = bsonutil.MustHaveTag(AlertRequest{}, "TaskId")
	BuildIdKey     = bsonutil.MustHaveTag(AlertRequest{}, "BuildId")
	VersionIdKey   = bsonutil.MustHaveTag(AlertRequest{}, "VersionId")
	ProjectIdKey   = bsonutil.MustHaveTag(AlertRequest{}, "ProjectId")
	DisplayKey     = bsonutil.MustHaveTag(AlertRequest{}, "Display")
	CreatedAtKey   = bsonutil.MustHaveTag(AlertRequest{}, "CreatedAt")
)
