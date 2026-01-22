package alertrecord

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	Collection = "alertrecord"
)

// Task triggers
const (
	FirstVersionFailureId    = "first_version_failure"
	FirstVariantFailureId    = "first_variant_failure"
	FirstTaskTypeFailureId   = "first_tasktype_failure"
	TaskFailTransitionId     = "task_transition_failure"
	FirstRegressionInVersion = "first_regression_in_version"
	taskRegressionByTest     = "task-regression-by-test"
)

// Host triggers
const (
	spawnHostWarningTemplate              = "spawn_%dhour"
	hostTemporaryExemptionWarningTemplate = "temporary_exemption_%dhour"
	volumeWarningTemplate                 = "volume_%dhour"
	alertableInstanceTypeWarning          = "alertable_instance_type"
)

const legacyAlertsSubscription = "legacy-alerts"

type AlertRecord struct {
	Id                  mgobson.ObjectId `bson:"_id"`
	SubscriptionID      string           `bson:"subscription_id"`
	Type                string           `bson:"type"`
	HostId              string           `bson:"host_id,omitempty"`
	VolumeId            string           `bson:"volume_id,omitempty"`
	TaskId              string           `bson:"task_id,omitempty"`
	TaskStatus          string           `bson:"task_status,omitempty"`
	ProjectId           string           `bson:"project_id,omitempty"`
	VersionId           string           `bson:"version_id,omitempty"`
	TaskName            string           `bson:"task_name,omitempty"`
	Variant             string           `bson:"variant,omitempty"`
	TestName            string           `bson:"test_name,omitempty"`
	RevisionOrderNumber int              `bson:"order,omitempty"`
	AlertTime           time.Time        `bson:"alert_time,omitempty"`
}

func (ar *AlertRecord) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(ar) }
func (ar *AlertRecord) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, ar) }

func (ar *AlertRecord) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, ar)
}

var (
	IdKey                  = bsonutil.MustHaveTag(AlertRecord{}, "Id")
	subscriptionIDKey      = bsonutil.MustHaveTag(AlertRecord{}, "SubscriptionID")
	TypeKey                = bsonutil.MustHaveTag(AlertRecord{}, "Type")
	TaskIdKey              = bsonutil.MustHaveTag(AlertRecord{}, "TaskId")
	HostIdKey              = bsonutil.MustHaveTag(AlertRecord{}, "HostId")
	VolumeIdKey            = bsonutil.MustHaveTag(AlertRecord{}, "VolumeId")
	TaskNameKey            = bsonutil.MustHaveTag(AlertRecord{}, "TaskName")
	VariantKey             = bsonutil.MustHaveTag(AlertRecord{}, "Variant")
	ProjectIdKey           = bsonutil.MustHaveTag(AlertRecord{}, "ProjectId")
	VersionIdKey           = bsonutil.MustHaveTag(AlertRecord{}, "VersionId")
	testNameKey            = bsonutil.MustHaveTag(AlertRecord{}, "TestName")
	RevisionOrderNumberKey = bsonutil.MustHaveTag(AlertRecord{}, "RevisionOrderNumber")
	AlertTimeKey           = bsonutil.MustHaveTag(AlertRecord{}, "AlertTime")
)

// FindOne gets one AlertRecord for the given query.
func FindOne(ctx context.Context, query db.Q) (*AlertRecord, error) {
	alertRec := &AlertRecord{}
	err := db.FindOneQContext(ctx, Collection, query, alertRec)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return alertRec, err
}

func subscriptionIDQuery(subID string) bson.M {
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
	return db.Query(q).Sort([]string{"-" + AlertTimeKey, "-" + RevisionOrderNumberKey}).Limit(1)
}

func ByFirstFailureInVersion(subscriptionID, versionId string) db.Q {
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

func FindByFirstRegressionInVersion(ctx context.Context, subscriptionID, versionId string) (*AlertRecord, error) {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = FirstRegressionInVersion
	q[VersionIdKey] = versionId
	return FindOne(ctx, db.Query(q).Limit(1))
}

func FindByLastTaskRegressionByTest(ctx context.Context, subscriptionID, testName, taskDisplayName, variant, projectID string) (*AlertRecord, error) {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = taskRegressionByTest
	q[testNameKey] = testName
	q[TaskNameKey] = taskDisplayName
	q[VariantKey] = variant
	q[ProjectIdKey] = projectID
	return FindOne(ctx, db.Query(q).Sort([]string{"-" + AlertTimeKey, "-" + RevisionOrderNumberKey}))
}

func FindByTaskRegressionByTaskTest(ctx context.Context, subscriptionID, testName, taskDisplayName, variant, projectID, taskID string) (*AlertRecord, error) {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = taskRegressionByTest
	q[testNameKey] = testName
	q[TaskNameKey] = taskDisplayName
	q[VariantKey] = variant
	q[ProjectIdKey] = projectID
	q[TaskIdKey] = taskID
	return FindOne(ctx, db.Query(q).Sort([]string{"-" + AlertTimeKey}))
}

func FindByTaskRegressionTestAndOrderNumber(ctx context.Context, subscriptionID, testName, taskDisplayName, variant, projectID string, revisionOrderNumber int) (*AlertRecord, error) {
	q := subscriptionIDQuery(subscriptionID)
	q[TypeKey] = taskRegressionByTest
	q[testNameKey] = testName
	q[TaskNameKey] = taskDisplayName
	q[VariantKey] = variant
	q[ProjectIdKey] = projectID
	q[RevisionOrderNumberKey] = revisionOrderNumber
	return FindOne(ctx, db.Query(q))
}

// FindByMostRecentSpawnHostExpirationWithHours finds the most recent alert
// record for a spawn host that is about to expire.
func FindByMostRecentSpawnHostExpirationWithHours(ctx context.Context, hostID string, hours int) (*AlertRecord, error) {
	alertType := fmt.Sprintf(spawnHostWarningTemplate, hours)
	q := subscriptionIDQuery(legacyAlertsSubscription)
	q[TypeKey] = alertType
	q[HostIdKey] = hostID
	return FindOne(ctx, db.Query(q).Sort([]string{"-" + AlertTimeKey}).Limit(1))
}

// FindByMostRecentTemporaryExemptionExpirationWithHours finds the most recent
// alert record for a spawn host's temporary exemption that is about to expire.
func FindByMostRecentTemporaryExemptionExpirationWithHours(ctx context.Context, hostID string, hours int) (*AlertRecord, error) {
	alertType := fmt.Sprintf(hostTemporaryExemptionWarningTemplate, hours)
	q := subscriptionIDQuery(legacyAlertsSubscription)
	q[TypeKey] = alertType
	q[HostIdKey] = hostID
	return FindOne(ctx, db.Query(q).Sort([]string{"-" + AlertTimeKey}).Limit(1))
}

func FindByVolumeExpirationWithHours(ctx context.Context, volumeID string, hours int) (*AlertRecord, error) {
	alertType := fmt.Sprintf(volumeWarningTemplate, hours)
	q := subscriptionIDQuery(legacyAlertsSubscription)
	q[TypeKey] = alertType
	q[VolumeIdKey] = volumeID
	return FindOne(ctx, db.Query(q).Limit(1))
}

// FindByMostRecentAlertableInstanceType finds the most recent alert
// record for a host using alertable instance types.
func FindByMostRecentAlertableInstanceType(ctx context.Context, hostID string) (*AlertRecord, error) {
	q := subscriptionIDQuery(legacyAlertsSubscription)
	q[TypeKey] = alertableInstanceTypeWarning
	q[HostIdKey] = hostID
	return FindOne(ctx, db.Query(q).Sort([]string{"-" + AlertTimeKey}).Limit(1))
}

func InsertNewTaskRegressionByTestRecord(ctx context.Context, subscriptionID, taskID, testName, taskDisplayName, variant, projectID string, revision int) error {
	record := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		SubscriptionID:      subscriptionID,
		Type:                taskRegressionByTest,
		TaskId:              taskID,
		ProjectId:           projectID,
		TaskName:            taskDisplayName,
		Variant:             variant,
		TestName:            testName,
		RevisionOrderNumber: revision,
		AlertTime:           time.Now(),
	}

	return errors.Wrapf(record.Insert(ctx), "inserting alert record '%s'", taskRegressionByTest)
}

func InsertNewSpawnHostExpirationRecord(ctx context.Context, hostID string, hours int) error {
	alertType := fmt.Sprintf(spawnHostWarningTemplate, hours)
	record := AlertRecord{
		Id:             mgobson.NewObjectId(),
		SubscriptionID: legacyAlertsSubscription,
		Type:           alertType,
		HostId:         hostID,
		AlertTime:      time.Now(),
	}

	return errors.Wrapf(record.Insert(ctx), "inserting alert record '%s'", alertType)
}

// InsertNewTemporaryExemptionExpirationRecord inserts a new alert record for a
// temporary exemption that is about to expire.
func InsertNewHostTemporaryExemptionExpirationRecord(ctx context.Context, hostID string, hours int) error {
	alertType := fmt.Sprintf(hostTemporaryExemptionWarningTemplate, hours)
	record := AlertRecord{
		Id:             mgobson.NewObjectId(),
		SubscriptionID: legacyAlertsSubscription,
		Type:           alertType,
		HostId:         hostID,
		AlertTime:      time.Now(),
	}

	return errors.Wrapf(record.Insert(ctx), "inserting alert record '%s'", alertType)
}

func InsertNewVolumeExpirationRecord(ctx context.Context, volumeID string, hours int) error {
	alertType := fmt.Sprintf(volumeWarningTemplate, hours)
	record := AlertRecord{
		Id:             mgobson.NewObjectId(),
		SubscriptionID: legacyAlertsSubscription,
		Type:           alertType,
		VolumeId:       volumeID,
		AlertTime:      time.Now(),
	}

	return errors.Wrapf(record.Insert(ctx), "inserting alert record '%s'", alertType)
}

// InsertNewAlertableInstanceTypeRecord inserts a new alert record for a host that's using an alertable instance type.
func InsertNewAlertableInstanceTypeRecord(ctx context.Context, hostID string) error {
	record := AlertRecord{
		Id:             mgobson.NewObjectId(),
		SubscriptionID: legacyAlertsSubscription,
		Type:           alertableInstanceTypeWarning,
		HostId:         hostID,
		AlertTime:      time.Now(),
	}

	return errors.Wrapf(record.Insert(ctx), "inserting alert record '%s'", alertableInstanceTypeWarning)
}
