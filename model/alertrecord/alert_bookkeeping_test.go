package alertrecord

import (
	"testing"

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
	s.NoError(db.Insert(Collection, &oldStyle0))
	s.NoError(db.Insert(Collection, &oldStyle1))

	rec, err := FindOne(ByLastFailureTransition("legacy-alerts", "task", "variant", "project"))
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

	rec, err = FindOne(ByLastFailureTransition("legacy-alerts", "task", "variant", "project"))
	s.NoError(err)
	s.Require().NotNil(rec)
	s.Equal(2, rec.RevisionOrderNumber)
}
