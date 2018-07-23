package alertrecord

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection = "alertrecord"
)

// Task triggers
const (
	FirstVersionFailureId  = "first_version_failure"
	FirstVariantFailureId  = "first_variant_failure"
	FirstTaskTypeFailureId = "first_tasktype_failure"
	TaskFailTransitionId   = "task_transition_failure"
	// TODO: EVG-3408
	TaskFailedId                    = "task_failed"
	LastRevisionNotFound            = "last_revision_not_found"
	taskRegressionByTest            = "task-regression-by-test"
	taskRegressionByTestWithNoTests = "task-regression-by-test-with-no-tests"

	legacyAlertsSubscription = "legacy-alerts"
)

// Host triggers
const (
	SpawnFailed                = "spawn_failed"
	SpawnHostTwoHourWarning    = "spawn_twohour"
	SpawnHostTwelveHourWarning = "spawn_twelvehour"
	SlowProvisionWarning       = "slow_provision"
	ProvisionFailed            = "provision_failed"
)

type AlertRecord struct {
	Id                  bson.ObjectId `bson:"_id"`
	SubscriptionID      string        `bson:"subscription_id"`
	Type                string        `bson:"type"`
	HostId              string        `bson:"host_id,omitempty"`
	TaskId              string        `bson:"task_id,omitempty"`
	TaskStatus          string        `bson:"task_status,omitempty"`
	ProjectId           string        `bson:"project_id,omitempty"`
	VersionId           string        `bson:"version_id,omitempty"`
	TaskName            string        `bson:"task_name,omitempty"`
	Variant             string        `bson:"variant,omitempty"`
	TestName            string        `bson:"test_name,omitempty"`
	RevisionOrderNumber int           `bson:"order,omitempty"`
}

//nolint: deadcode, megacheck
var (
	IdKey                  = bsonutil.MustHaveTag(AlertRecord{}, "Id")
	subscriptionIDKey      = bsonutil.MustHaveTag(AlertRecord{}, "SubscriptionID")
	TypeKey                = bsonutil.MustHaveTag(AlertRecord{}, "Type")
	TaskIdKey              = bsonutil.MustHaveTag(AlertRecord{}, "TaskId")
	taskStatusKey          = bsonutil.MustHaveTag(AlertRecord{}, "TaskStatus")
	HostIdKey              = bsonutil.MustHaveTag(AlertRecord{}, "HostId")
	TaskNameKey            = bsonutil.MustHaveTag(AlertRecord{}, "TaskName")
	VariantKey             = bsonutil.MustHaveTag(AlertRecord{}, "Variant")
	ProjectIdKey           = bsonutil.MustHaveTag(AlertRecord{}, "ProjectId")
	VersionIdKey           = bsonutil.MustHaveTag(AlertRecord{}, "VersionId")
	testNameKey            = bsonutil.MustHaveTag(AlertRecord{}, "TestName")
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

func subscriptionIDQuery(subID string) bson.M {
	if subID == legacyAlertsSubscription {
		return bson.M{
			"$or": []bson.M{
				{
					subscriptionIDKey: subID,
				},
				{
					subscriptionIDKey: bson.M{
						"$exists": false,
					},
				},
			},
		}
	}

	return bson.M{
		subscriptionIDKey: subID,
	}
}

// ByPreviousFailureTransition finds the last alert record that was stored for a task/variant
// within a given project.
// This can be used to determine whether or not a newly detected failure transition should trigger
// an alert. If an alert was already sent for a failed task, which transitioned from a succsesfulwhose commit order number is
func ByLastFailureTransition(subscriptionID, taskName, variant, projectId string) db.Q {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = TaskFailTransitionId
	q[TaskNameKey] = taskName
	q[VariantKey] = variant
	q[ProjectIdKey] = projectId
	return db.Query(q).Sort([]string{"-" + RevisionOrderNumberKey}).Limit(1)
}

func ByFirstFailureInVersion(subscriptionID, projectId, versionId string) db.Q {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = FirstVersionFailureId
	q[VersionIdKey] = versionId
	return db.Query(q).Limit(1)
}

func ByFirstFailureInVariant(subscriptionID, versionId, variant string) db.Q {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = FirstVariantFailureId
	q[VersionIdKey] = versionId
	q[VariantKey] = variant
	return db.Query(q).Limit(1)
}
func ByFirstFailureInTaskType(subscriptionID, versionId, taskName string) db.Q {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = FirstTaskTypeFailureId
	q[VersionIdKey] = versionId
	q[TaskNameKey] = taskName
	return db.Query(q).Limit(1)
}

func ByHostAlertRecordType(subscriptionID, hostId, triggerId string) db.Q {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = triggerId
	q[HostIdKey] = hostId
	return db.Query(q).Limit(1)
}

func ByLastRevNotFound(subscriptionID, projectId, versionId string) db.Q {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = LastRevisionNotFound
	q[ProjectIdKey] = projectId
	q[VersionIdKey] = versionId
	return db.Query(q).Limit(1)
}

func FindByLastTaskRegressionByTest(subscriptionID, testName, taskDisplayName, variant, projectID string, beforeRevision int) (*AlertRecord, error) {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = taskRegressionByTest
	q[testNameKey] = testName
	q[TaskNameKey] = taskDisplayName
	q[VariantKey] = variant
	q[ProjectIdKey] = projectID
	q[RevisionOrderNumberKey] = bson.M{
		"$lte": beforeRevision,
	}
	return FindOne(db.Query(q).Sort([]string{"-" + RevisionOrderNumberKey}))
}

func FindByLastTaskRegressionByTestWithNoTests(subscriptionID, taskDisplayName, variant, projectID string, revision int) (*AlertRecord, error) {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = taskRegressionByTestWithNoTests
	q[TaskNameKey] = taskDisplayName
	q[VariantKey] = variant
	q[ProjectIdKey] = projectID
	q[RevisionOrderNumberKey] = bson.M{
		"$lte": revision,
	}
	return FindOne(db.Query(q).Sort([]string{"-" + RevisionOrderNumberKey}))
}

func (ar *AlertRecord) Insert() error {
	return db.Insert(Collection, ar)
}

func InsertNewTaskRegressionByTestRecord(subscriptionID, testName, taskDisplayName, variant, projectID string, revision int) error {
	record := AlertRecord{
		Id:                  bson.NewObjectId(),
		SubscriptionID:      subscriptionID,
		Type:                taskRegressionByTest,
		ProjectId:           projectID,
		TaskName:            taskDisplayName,
		Variant:             variant,
		TestName:            testName,
		RevisionOrderNumber: revision,
	}

	return errors.Wrapf(record.Insert(), "failed to insert alert record %s", taskRegressionByTest)
}

func InsertNewTaskRegressionByTestWithNoTestsRecord(subscriptionID, taskDisplayName, taskStatus, variant, projectID string, revision int) error {
	record := AlertRecord{
		Id:                  bson.NewObjectId(),
		SubscriptionID:      subscriptionID,
		Type:                taskRegressionByTestWithNoTests,
		ProjectId:           projectID,
		TaskName:            taskDisplayName,
		TaskStatus:          taskStatus,
		Variant:             variant,
		RevisionOrderNumber: revision,
	}

	return errors.Wrapf(record.Insert(), "failed to insert alert record %s", taskRegressionByTestWithNoTests)
}
