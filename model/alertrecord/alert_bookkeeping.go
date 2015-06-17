package alertrecord

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection = "alertrecord"
)

// Task triggers
var (
	TaskFailedId           = "task_failed"
	FirstVersionFailureId  = "first_version_failure"
	FirstVariantFailureId  = "first_variant_failure"
	FirstTaskTypeFailureId = "first_tasktype_failure"
	TaskFailTransitionId   = "task_transition_failure"
	LastRevisionNotFound   = "last_revision_not_found"
)

// Host triggers
var (
	SpawnHostTwoHourWarning    = "spawn_twohour"
	SpawnHostTwelveHourWarning = "spawn_twelvehour"
	SlowProvisionWarning       = "slow_provision"
	ProvisionFailed            = "provision_failed"
)

type AlertRecord struct {
	Id                  bson.ObjectId `bson:"_id"`
	Type                string        `bson:"type"`
	HostId              string        `bson:"host_id,omitempty"`
	TaskId              string        `bson:"task_id,omitempty"`
	ProjectId           string        `bson:"project_id,omitempty"`
	VersionId           string        `bson:"version_id,omitempty"`
	TaskName            string        `bson:"task_name,omitempty"`
	Variant             string        `bson:"variant,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty"`
}

var (
	IdKey                  = bsonutil.MustHaveTag(AlertRecord{}, "Id")
	TypeKey                = bsonutil.MustHaveTag(AlertRecord{}, "Type")
	TaskIdKey              = bsonutil.MustHaveTag(AlertRecord{}, "TaskId")
	HostIdKey              = bsonutil.MustHaveTag(AlertRecord{}, "HostId")
	TaskNameKey            = bsonutil.MustHaveTag(AlertRecord{}, "TaskName")
	VariantKey             = bsonutil.MustHaveTag(AlertRecord{}, "Variant")
	ProjectIdKey           = bsonutil.MustHaveTag(AlertRecord{}, "ProjectId")
	VersionIdKey           = bsonutil.MustHaveTag(AlertRecord{}, "VersionId")
	RevisionOrderNumberKey = bsonutil.MustHaveTag(AlertRecord{}, "RevisionOrderNumber")
)

// FindOne gets one AlertRecord for the given query.
func FindOne(query db.Q) (*AlertRecord, error) {
	alertRec := &AlertRecord{}
	err := db.FindOneQ(Collection, query, alertRec)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return alertRec, err
}

// ByPreviousFailureTransition finds the last alert record that was stored for a task/variant
// within a given project.
// This can be used to determine whether or not a newly detected failure transition should trigger
// an alert. If an alert was already sent for a failed task, which transitioned from a succsesfulwhose commit order number is
func ByLastFailureTransition(taskName, variant, projectId string) db.Q {
	return db.Query(bson.M{
		TypeKey:      TaskFailTransitionId,
		TaskNameKey:  taskName,
		VariantKey:   variant,
		ProjectIdKey: projectId,
	}).Sort([]string{"-" + RevisionOrderNumberKey}).Limit(1)
}

func ByFirstFailureInVersion(projectId, versionId string) db.Q {
	return db.Query(bson.M{
		TypeKey:      FirstVersionFailureId,
		VersionIdKey: versionId,
	}).Limit(1)
}

func ByFirstFailureInVariant(versionId, variant string) db.Q {
	return db.Query(bson.M{
		TypeKey:      FirstVariantFailureId,
		VersionIdKey: versionId,
		VariantKey:   variant,
	}).Limit(1)
}
func ByFirstFailureInTaskType(versionId, taskName string) db.Q {
	return db.Query(bson.M{
		TypeKey:      FirstTaskTypeFailureId,
		VersionIdKey: versionId,
		TaskNameKey:  taskName,
	}).Limit(1)
}

func ByHostAlertRecordType(hostId, triggerId string) db.Q {
	return db.Query(bson.M{
		TypeKey:   triggerId,
		HostIdKey: hostId,
	}).Limit(1)
}

func ByLastRevNotFound(projectId, versionId string) db.Q {
	return db.Query(bson.M{
		TypeKey:      LastRevisionNotFound,
		ProjectIdKey: projectId,
		VersionIdKey: versionId,
	}).Limit(1)
}

func (ar *AlertRecord) Insert() error {
	return db.Insert(Collection, ar)
}
