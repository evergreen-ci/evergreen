package data

import (
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestFindAllDistros(t *testing.T) {
	assert := assert.New(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindAllDistros")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(session.DB(testConfig.Database.DB).DropDatabase(), t, "Error dropping database")

	sc := &DBConnector{}

	var distros []*distro.Distro
	numDistros := 10
	for i := 0; i < numDistros; i++ {
		d := &distro.Distro{
			Id: fmt.Sprintf("distro_%d", rand.Int()),
		}
		distros = append(distros, d)
		assert.Nil(d.Insert())
	}

	found, err := sc.FindAllDistros()
	assert.Nil(err)
	assert.Len(found, numDistros)
}

////////////////////////////////////////////////////////////////////////////////
type DistroCostConnectorSuite struct {
	ctx     Connector
	isMock  bool
	numLoop int

	suite.Suite
}

// Initialize the ConnectorSuites
func TestDistroCostConnectorSuite(t *testing.T) {
	assert := assert.New(t)

	// Set up
	s := new(DistroCostConnectorSuite)
	s.ctx = &DBConnector{}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestDistroCostConnectorSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	// Tear down
	assert.NoError(db.Clear(task.Collection))
	assert.NoError(db.Clear(distro.Collection))

	s.isMock = false
	s.numLoop = 10

	// Insert data
	for i := 0; i < s.numLoop; i++ {
		testTask1 := &task.Task{
			Id:        fmt.Sprintf("task_%d", i*2),
			DistroId:  fmt.Sprintf("%d", i),
			TimeTaken: time.Duration(i),
		}
		assert.NoError(testTask1.Insert())

		testTask2 := &task.Task{
			Id:        fmt.Sprintf("task_%d", i*2+1),
			DistroId:  fmt.Sprintf("%d", i),
			TimeTaken: time.Duration(i),
		}
		assert.NoError(testTask2.Insert())

		var settings = make(map[string]interface{})
		settings["instance_type"] = fmt.Sprintf("value_%d", i)
		testDistro := &distro.Distro{
			Id:               fmt.Sprintf("%d", i),
			Provider:         "provider",
			ProviderSettings: &settings,
		}
		assert.NoError(testDistro.Insert())
	}
}

func TestMockDistroConnectorSuite(t *testing.T) {
	s := new(DistroCostConnectorSuite)

	var settings = make(map[string]interface{})
	settings["instance_type"] = "type"

	s.ctx = &MockConnector{
		MockDistroConnector: MockDistroConnector{
			CachedTasks: []task.Task{
				{Id: "task1", DistroId: "distro1", TimeTaken: time.Duration(1)},
				{Id: "task2", DistroId: "distro2", TimeTaken: time.Duration(1)},
				{Id: "task3", DistroId: "distro2", TimeTaken: time.Duration(1)},
			},
			CachedDistros: []distro.Distro{
				{Id: "distro1", Provider: "ec2", ProviderSettings: &settings},
				{Id: "distro2", Provider: "gce", ProviderSettings: &settings},
			},
		},
	}
	s.isMock = true
	suite.Run(t, s)
}

func (s *DistroCostConnectorSuite) TestFindCostByDistroId() {
	if s.isMock {
		found, err := s.ctx.FindCostByDistroId("distro1")
		s.NoError(err)
		s.Equal(found.SumTimeTaken, time.Duration(1))
		s.Equal(found.Provider, "ec2")
		s.Equal(found.ProviderSettings["instance_type"], "type")

		found, err = s.ctx.FindCostByDistroId("distro2")
		s.NoError(err)
		s.Equal(found.SumTimeTaken, time.Duration(1)*2)
		s.Equal(found.Provider, "gce")
		s.Equal(found.ProviderSettings["instance_type"], "type")

	} else {
		// Finding each distro's sum of time taken should succeed
		for i := 0; i < s.numLoop; i++ {
			found, err := s.ctx.FindCostByDistroId(fmt.Sprintf("%d", i))
			s.NoError(err)
			s.Equal(found.SumTimeTaken, time.Duration(i)*2)
			s.Equal(found.Provider, "provider")
			s.Equal(found.ProviderSettings["instance_type"], fmt.Sprintf("value_%d", i))
		}

		// Searching for a distro that doesn't exist should fail with an APIError
		found, err := s.ctx.FindCostByDistroId("fake_distro")
		s.NoError(err)
		s.Nil(found)
		s.IsType(err, &rest.APIError{})
		apiErr, ok := err.(*rest.APIError)
		s.Equal(ok, true)
		s.Equal(apiErr.StatusCode, http.StatusNotFound)
	}
}
