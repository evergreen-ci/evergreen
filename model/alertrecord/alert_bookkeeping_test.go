package alertrecord

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

func TestAlertRecord(t *testing.T) {
	suite.Run(t, &alertRecordSuite{})
}

type alertRecordSuite struct {
	suite.Suite
}

func (s *alertRecordSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection))
}

func (s *alertRecordSuite) TestInsertNewTaskRegressionByTestRecord() {
	const (
		sub             = "test-sub"
		taskID          = "task0"
		testName        = "test"
		taskDisplayName = "task"
		variant         = "variant"
		projectID       = "project"
	)
	beforeRevision := 2
	s.NoError(InsertNewTaskRegressionByTestRecord(sub, taskID, testName, taskDisplayName, variant, projectID, beforeRevision))
	beforeRevision = 5
	s.NoError(InsertNewTaskRegressionByTestRecord(sub, taskID, testName, taskDisplayName, variant, projectID, beforeRevision))

	record, err := FindByLastTaskRegressionByTest(sub, testName, taskDisplayName, variant, projectID)
	s.NoError(err)
	s.Require().NotNil(record)
	s.True(record.Id.Valid())
	s.Equal(taskRegressionByTest, record.Type)
	s.Empty(record.HostId)
	s.Equal("task0", record.TaskId)
	s.Empty(record.TaskStatus)
	s.Equal("project", record.ProjectId)
	s.Empty(record.VersionId)
	s.Equal("task", record.TaskName)
	s.Equal("variant", record.Variant)
	s.Equal("test", record.TestName)
	s.Equal(5, record.RevisionOrderNumber)
	s.WithinDuration(time.Now(), record.AlertTime, time.Second)
}

func (s *alertRecordSuite) TestByLastFailureTransition() {
	alert1 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                TaskFailTransitionId,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t1",
		RevisionOrderNumber: 1,
	}
	s.NoError(alert1.Insert())
	alert2 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
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
		Id:                  mgobson.NewObjectId(),
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
		Id:                  mgobson.NewObjectId(),
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
		Id:                  mgobson.NewObjectId(),
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
		"_id":                  mgobson.NewObjectId(),
		TypeKey:                TaskFailTransitionId,
		TaskNameKey:            "task",
		VariantKey:             "variant",
		ProjectIdKey:           "project",
		RevisionOrderNumberKey: 0,
	}
	oldStyle1 := bson.M{
		"_id":                  mgobson.NewObjectId(),
		TypeKey:                TaskFailTransitionId,
		TaskNameKey:            "task",
		VariantKey:             "variant",
		ProjectIdKey:           "project",
		RevisionOrderNumberKey: 1,
	}
	oldStyle3 := bson.M{
		"_id":                  mgobson.NewObjectId(),
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
		Id:                  mgobson.NewObjectId(),
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

func (s *alertRecordSuite) TestFindByLastTaskRegressionByTest() {
	alert1 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                taskRegressionByTest,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t1",
		TestName:            "test",
		RevisionOrderNumber: 1,
	}
	s.NoError(alert1.Insert())
	alert2 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                taskRegressionByTest,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t2",
		TestName:            "test",
		RevisionOrderNumber: 2,
	}
	s.NoError(alert2.Insert())

	//test that sorting by revision works
	alert, err := FindByLastTaskRegressionByTest("", "test", "t", "v", "p")
	s.NoError(err)
	s.Equal("t2", alert.TaskId)

	// test that sorting by time and revision with some times missing works
	alert3 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                taskRegressionByTest,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t3",
		TestName:            "test",
		RevisionOrderNumber: 3,
		AlertTime:           time.Now(),
	}
	s.NoError(alert3.Insert())
	alert, err = FindByLastTaskRegressionByTest("", "test", "t", "v", "p")
	s.NoError(err)
	s.Equal("t3", alert.TaskId)

	// test that an earlier alert for a later commit returns the latest alert
	alert4 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                taskRegressionByTest,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t4",
		TestName:            "test",
		RevisionOrderNumber: 4,
		AlertTime:           time.Now().Add(-1 * time.Hour),
	}
	s.NoError(alert4.Insert())
	alert, err = FindByLastTaskRegressionByTest("", "test", "t", "v", "p")
	s.NoError(err)
	s.Equal("t3", alert.TaskId)
}

func (s *alertRecordSuite) TestFindByTaskRegressionByTaskTest() {
	alert1 := AlertRecord{
		Id:        mgobson.NewObjectId(),
		Type:      taskRegressionByTest,
		TaskName:  "t",
		Variant:   "v",
		ProjectId: "p",
		TaskId:    "t1",
		TestName:  "test",
		AlertTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
	}
	s.NoError(alert1.Insert())
	alert2 := AlertRecord{
		Id:        mgobson.NewObjectId(),
		Type:      taskRegressionByTest,
		TaskName:  "t",
		Variant:   "v",
		ProjectId: "p",
		TaskId:    "t2",
		TestName:  "test",
		AlertTime: time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC),
	}
	s.NoError(alert2.Insert())

	alert, err := FindByTaskRegressionByTaskTest("", "test", "t", "v", "p", "t2")
	s.NoError(err)
	s.True(time.Date(2010, time.November, 10, 23, 0, 0, 0, time.UTC).Equal(alert.AlertTime))
}

func (s *alertRecordSuite) TestFindByTaskRegressionTestAndOrderNumber() {
	alert1 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                taskRegressionByTest,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t1",
		TestName:            "test",
		RevisionOrderNumber: 1,
	}
	s.NoError(alert1.Insert())
	alert2 := AlertRecord{
		Id:                  mgobson.NewObjectId(),
		Type:                taskRegressionByTest,
		TaskName:            "t",
		Variant:             "v",
		ProjectId:           "p",
		TaskId:              "t2",
		TestName:            "test",
		RevisionOrderNumber: 2,
	}
	s.NoError(alert2.Insert())

	alert, err := FindByTaskRegressionTestAndOrderNumber("", "test", "t", "v", "p", 1)
	s.NoError(err)
	s.Equal("t1", alert.TaskId)
}
