package alertrecord

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestAlertRecord(t *testing.T) {
	suite.Run(t, &alertRecordSuite{})
}

type alertRecordSuite struct {
	suite.Suite
}

func (s *alertRecordSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *alertRecordSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection))
}

func (s *alertRecordSuite) TestInsertNewTaskRegressionByTestRecord() {
	const (
		sub             = "test-sub"
		testName        = "test"
		taskDisplayName = "task"
		variant         = "variant"
		projectID       = "project"
	)
	beforeRevision := 2
	s.NoError(InsertNewTaskRegressionByTestRecord(sub, testName, taskDisplayName, variant, projectID, beforeRevision))
	beforeRevision = 5
	s.NoError(InsertNewTaskRegressionByTestRecord(sub, testName, taskDisplayName, variant, projectID, beforeRevision))

	record, err := FindByLastTaskRegressionByTest(sub, testName, taskDisplayName, variant, projectID, beforeRevision)
	s.NoError(err)
	s.Require().NotNil(record)
	s.True(record.Id.Valid())
	s.Equal(taskRegressionByTest, record.Type)
	s.Empty(record.HostId)
	s.Empty(record.TaskId)
	s.Empty(record.TaskStatus)
	s.Equal("project", record.ProjectId)
	s.Empty(record.VersionId)
	s.Equal("task", record.TaskName)
	s.Equal("variant", record.Variant)
	s.Equal("test", record.TestName)
	s.Equal(5, record.RevisionOrderNumber)
	s.InDelta(time.Now().UnixNano(), record.AlertTime.UnixNano(), float64(10*time.Millisecond))
}

func (s *alertRecordSuite) TestInsertNewTaskRegressionByTestWithNoTestsRecord() {
	const (
		sub             = "test-sub"
		taskDisplayName = "task"
		taskStatus      = "something"
		variant         = "variant"
		projectID       = "project"
	)
	beforeRevision := 2
	s.NoError(InsertNewTaskRegressionByTestWithNoTestsRecord(sub, taskDisplayName, taskStatus, variant, projectID, beforeRevision))
	beforeRevision = 5
	s.NoError(InsertNewTaskRegressionByTestWithNoTestsRecord(sub, taskDisplayName, taskStatus, variant, projectID, beforeRevision))

	record, err := FindByLastTaskRegressionByTestWithNoTests(sub, taskDisplayName, variant, projectID, beforeRevision)

	s.NoError(err)
	s.Require().NotNil(record)
	s.True(record.Id.Valid())
	s.Equal(taskRegressionByTestWithNoTests, record.Type)
	s.Empty(record.HostId)
	s.Empty(record.TaskId)
	s.Equal("something", record.TaskStatus)
	s.Equal("project", record.ProjectId)
	s.Empty(record.VersionId)
	s.Equal("task", record.TaskName)
	s.Equal("variant", record.Variant)
	s.Empty(record.TestName)
	s.Equal(5, record.RevisionOrderNumber)
	s.InDelta(time.Now().UnixNano(), record.AlertTime.UnixNano(), float64(10*time.Millisecond))
}

func (s *alertRecordSuite) ByLastFailureTransition() {
	alert1 := AlertRecord{
		Id:                  bson.NewObjectId(),
		Type:                TaskFailTransitionId,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t1",
		RevisionOrderNumber: 1,
	}
	s.NoError(alert1.Insert())
	alert2 := AlertRecord{
		Id:                  bson.NewObjectId(),
		Type:                TaskFailTransitionId,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t2",
		RevisionOrderNumber: 2,
	}
	s.NoError(alert2.Insert())
	alert, err := FindOne(ByLastFailureTransition("", "t", "v", "p"))
	s.NoError(err)
	s.Equal("t2", alert.TaskId)

	alert3 := AlertRecord{
		Id:                  bson.NewObjectId(),
		Type:                TaskFailTransitionId,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t3",
		RevisionOrderNumber: 3,
	}
	s.NoError(alert3.Insert())
	alert, err = FindOne(ByLastFailureTransition("", "t", "v", "p"))
	s.NoError(err)
	s.Equal("t3", alert.TaskId)

	alert4 := AlertRecord{
		Id:                  bson.NewObjectId(),
		Type:                TaskFailTransitionId,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t4",
		RevisionOrderNumber: 4,
		AlertTime:           time.Now(),
	}
	s.NoError(alert4.Insert())
	alert, err = FindOne(ByLastFailureTransition("", "t", "v", "p"))
	s.NoError(err)
	s.Equal("t4", alert.TaskId)

	alert5 := AlertRecord{
		Id:                  bson.NewObjectId(),
		Type:                TaskFailTransitionId,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t5",
		RevisionOrderNumber: 5,
		AlertTime:           time.Now().Add(-1 * time.Hour),
	}
	s.NoError(alert5.Insert())
	alert, err = FindOne(ByLastFailureTransition("", "t", "v", "p"))
	s.NoError(err)
	s.Equal("t4", alert.TaskId)
}

func (s *alertRecordSuite) TestFindOneWithUnsetIDQuery() {
	oldStyle0 := bson.M{
		"_id":                  bson.NewObjectId(),
		TypeKey:                TaskFailTransitionId,
		TaskNameKey:            "task",
		VariantKey:             "variant",
		ProjectIdKey:           "project",
		RevisionOrderNumberKey: 0,
	}
	oldStyle1 := bson.M{
		"_id":                  bson.NewObjectId(),
		TypeKey:                TaskFailTransitionId,
		TaskNameKey:            "task",
		VariantKey:             "variant",
		ProjectIdKey:           "project",
		RevisionOrderNumberKey: 1,
	}
	oldStyle3 := bson.M{
		"_id":                  bson.NewObjectId(),
		TypeKey:                TaskFailTransitionId,
		TaskNameKey:            "othertask",
		VariantKey:             "othervariant",
		ProjectIdKey:           "otherproject",
		RevisionOrderNumberKey: 2,
	}
	s.NoError(db.Insert(Collection, &oldStyle0))
	s.NoError(db.Insert(Collection, &oldStyle1))
	s.NoError(db.Insert(Collection, &oldStyle3))

	rec, err := FindOne(ByLastFailureTransition(legacyAlertsSubscription, "task", "variant", "project"))
	s.NoError(err)
	s.Require().NotNil(rec)

	s.Equal(1, rec.RevisionOrderNumber)
	newStyle := AlertRecord{
		Id:                  bson.NewObjectId(),
		SubscriptionID:      legacyAlertsSubscription,
		Type:                TaskFailTransitionId,
		TaskName:            "task",
		Variant:             "variant",
		ProjectId:           "project",
		RevisionOrderNumber: 2,
	}
	s.NoError(newStyle.Insert())

	rec, err = FindOne(ByLastFailureTransition(legacyAlertsSubscription, "task", "variant", "project"))
	s.NoError(err)
	s.Require().NotNil(rec)
	s.Equal(2, rec.RevisionOrderNumber)

	records := []AlertRecord{}
	err = db.FindAllQ(Collection, ByLastFailureTransition(legacyAlertsSubscription, "task", "variant", "project").Limit(999), &records)
	s.NoError(err)
	s.Len(records, 3)
}
