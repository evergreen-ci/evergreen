package data

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
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

////////////////////////////////////////////////////////////////////////////////

type VersionConnectorSuite struct {
	ctx    Connector
	isMock bool

	suite.Suite
}

//----------------------------------------------------------------------------//
//   Initialize the ConnectorSuites                                           //
//----------------------------------------------------------------------------//
func TestVersionConnectorSuite(t *testing.T) {
	assert := assert.New(t) // nolint

	// Set up
	s := new(VersionConnectorSuite)
	s.ctx = &DBConnector{}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestVersionConnectorSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	// Tear down
	assert.Nil(db.Clear(task.Collection))
	assert.Nil(db.Clear(version.Collection))
	assert.Nil(db.Clear(build.Collection))

	s.isMock = false

	// Insert data for the test paths
	versions := []*version.Version{
		{Id: "version1"},
		{Id: "version2"},
	}

	tasks := []*task.Task{
		{Id: "task1", Version: "version1", Aborted: false, Status: evergreen.TaskStarted},
		{Id: "task2", Version: "version1", Aborted: false, Status: evergreen.TaskDispatched},
		{Id: "task3", Version: "version1", Aborted: true, Status: evergreen.TaskInactive},
		{Id: "task4", Version: "version2", Aborted: false, Status: evergreen.TaskStarted},
		{Id: "task5", Version: "version3", Aborted: false, Status: evergreen.TaskSucceeded, BuildId: "build1"},
	}

	builds := []*build.Build{
		{Id: "build1", Tasks: []build.TaskCache{{Id: "task5"}}},
	}

	for _, item := range versions {
		assert.NoError(item.Insert())
	}
	for _, item := range tasks {
		assert.NoError(item.Insert())
	}
	for _, item := range builds {
		assert.NoError(item.Insert())
	}

	// Run the suite
	suite.Run(t, s)
}

func TestMockVersionConnectorSuite(t *testing.T) {
	s := new(VersionConnectorSuite)
	s.ctx = &MockConnector{
		MockVersionConnector: MockVersionConnector{
			CachedVersions: []version.Version{{Id: "version1"}, {Id: "version2"}},
			CachedTasks: []task.Task{
				{Id: "task1", Version: "version1", Aborted: false, Status: evergreen.TaskStarted},
				{Id: "task2", Version: "version1", Aborted: false, Status: evergreen.TaskDispatched},
				{Id: "task3", Version: "version1", Aborted: true, Status: evergreen.TaskInactive},
				{Id: "task4", Version: "version2", Aborted: false, Status: evergreen.TaskStarted},
			},
			CachedRestartedVersions: make(map[string]string),
		},
	}
	s.isMock = true
	suite.Run(t, s)
}

//----------------------------------------------------------------------------//
//   Test cases                                                               //
//----------------------------------------------------------------------------//

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
	// Finding a non-existent version should fail
	v, err := s.ctx.FindVersionById("build3")
	s.Error(err)
	s.Nil(v)
}

func (s *VersionConnectorSuite) TestAbortVersion() {
	versionId := "version1"
	err := s.ctx.AbortVersion(versionId)
	s.NoError(err)

	// NOTE: TestAbort() has been written in this following way because FindTaskbyVersionId()
	// has not been implemented yet. FindTaskByVersionId() would eliminate the need to
	// separate the case when the connector is a mock from the case when the connector
	// is backed by the DB.

	// Iterate through each task and check values.
	// Task1 and Task2, which are of the aborted version and tasks with abortable statuses
	// should be aborted. Task3 have been already aborted. Task4 is of another version and should
	// not have been aborted.
	if s.isMock {
		cachedTasks := s.ctx.(*MockConnector).MockVersionConnector.CachedTasks
		s.Equal(true, cachedTasks[0].Aborted)
		s.Equal(true, cachedTasks[1].Aborted)
		s.Equal(true, cachedTasks[2].Aborted)
		s.Equal(false, cachedTasks[3].Aborted)
	} else {
		t1, _ := s.ctx.FindTaskById("task1")
		s.Equal(versionId, t1.Version)
		s.Equal(true, t1.Aborted)

		t2, _ := s.ctx.FindTaskById("task2")
		s.Equal(versionId, t2.Version)
		s.Equal(true, t2.Aborted)

		t3, _ := s.ctx.FindTaskById("task3")
		s.Equal(versionId, t3.Version)
		s.Equal(true, t3.Aborted)

		t4, _ := s.ctx.FindTaskById("task4")
		s.NotEqual(true, t4.Aborted)
	}
}

func (s *VersionConnectorSuite) TestRestartVersion() {
	if s.isMock {
		// Testing with versions that have tasks under them should succeed.
		err := s.ctx.RestartVersion("version1", "caller1")
		s.NoError(err)
		s.Equal(s.ctx.(*MockConnector).CachedRestartedVersions["version1"], "caller1")

		err = s.ctx.RestartVersion("version2", "caller2")
		s.NoError(err)
		s.Equal(s.ctx.(*MockConnector).CachedRestartedVersions["version2"], "caller2")

	} else {
		versionId := "version3"
		err := s.ctx.RestartVersion(versionId, "caller3")
		s.NoError(err)

		// When a version is restarted, all of its completed tasks should be reset.
		// (task.Status should be undispatched)
		t5, _ := s.ctx.FindTaskById("task5")
		s.Equal(versionId, t5.Version)
		s.Equal(evergreen.TaskUndispatched, t5.Status)

		// Build status for all builds containing the tasks that we touched
		// should be updated.
		b1, _ := s.ctx.FindBuildById("build1")
		s.Equal(evergreen.BuildStarted, b1.Status)
		s.Equal("caller3", b1.ActivatedBy)
	}
}
