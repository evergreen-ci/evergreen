package alertrecord

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
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

func (s *alertRecordSuite) TestInsertNewTaskRegssionByTestRecord() {
	testName := "test"
	taskDisplayName := "task"
	variant := "variant"
	projectID := "project"
	beforeRevision := 2
	s.NoError(InsertNewTaskRegressionByTestRecord(testName, taskDisplayName, variant, projectID, beforeRevision))
	beforeRevision = 5
	s.NoError(InsertNewTaskRegressionByTestRecord(testName, taskDisplayName, variant, projectID, beforeRevision))

	record, err := FindByLastRegressionByTest(testName, taskDisplayName, variant, projectID, beforeRevision)
	s.NoError(err)
	s.Require().NotNil(record)
	s.Equal(5, record.RevisionOrderNumber)
	s.Equal(testName, record.TestName)
	s.Equal(taskDisplayName, record.TaskName)
	s.Equal(variant, record.Variant)
	s.Equal(projectID, record.ProjectId)
	s.Equal(taskRegressionByTest, record.Type)
}
