package data

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestFindCostByVersionId(t *testing.T) {
	assert := assert.New(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindCostByVersionId")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing"+
		" '%v' collection", task.Collection)

	sc := &DBConnector{}
	numTaskSet := 10

	// Add task documents in the database
	for i := 0; i < numTaskSet; i++ {
		testTask1 := &task.Task{
			Id:        fmt.Sprintf("task_%d", i*2),
			Version:   fmt.Sprintf("%d", i),
			TimeTaken: time.Duration(i),
		}
		assert.Nil(testTask1.Insert())

		testTask2 := &task.Task{
			Id:        fmt.Sprintf("task_%d", i*2+1),
			Version:   fmt.Sprintf("%d", i),
			TimeTaken: time.Duration(i),
		}
		assert.Nil(testTask2.Insert())
	}

	// Finding each version's sum of time taken should succeed
	for i := 0; i < numTaskSet; i++ {
		found, err := sc.FindCostByVersionId(fmt.Sprintf("%d", i))
		assert.Nil(err)
		assert.Equal(found.SumTimeTaken, time.Duration(i)*2)
	}

	// Searching for a version that doesn't exist should fail with an APIError
	found, err := sc.FindCostByVersionId("fake_version")
	assert.NotNil(err)
	assert.Nil(found)
	assert.IsType(err, &rest.APIError{})
	apiErr, ok := err.(*rest.APIError)
	assert.Equal(ok, true)
	assert.Equal(apiErr.StatusCode, http.StatusNotFound)
}

type VersionConnectorSuite struct {
	ctx Connector
	suite.Suite
}

func TestVersionConnectorSuite(t *testing.T) {
	s := new(VersionConnectorSuite)
	s.ctx = &DBConnector{}

	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindCostByVersionId")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	assert := assert.New(t)
	assert.Nil(db.Clear(task.Collection))

	version1 := &version.Version{Id: "version1"}
	version2 := &version.Version{Id: "version2"}

	version1.Insert()
	version2.Insert()

	suite.Run(t, s)
}

func TestMockVersionConnectorSuite(t *testing.T) {
	s := new(VersionConnectorSuite)
	s.ctx = &MockConnector{MockVersionConnector: MockVersionConnector{
		CachedVersions: []version.Version{{Id: "version1"}, {Id: "version2"}},
	}}
	suite.Run(t, s)
}

func (s *VersionConnectorSuite) TestFindVersionByIdSuccess() {
	// Finding existing versions should succeed
	v, err := s.ctx.FindVersionById("version1")
	s.NoError(err)
	s.NotNil(v)
	s.Equal("version1", v.Id)

	v, err = s.ctx.FindVersionById("version2")
	s.NoError(err)
	s.NotNil(v)
	s.Equal("version2", v.Id)
}

func (s *VersionConnectorSuite) TestFindVersionByIdFail() {
	// Finding a non-existant version should fail
	v, err := s.ctx.FindVersionById("build3")
	s.Error(err)
	s.Nil(v)
}
