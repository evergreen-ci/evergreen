package data

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
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
		assert.Nil(d.Insert())
	}

	found, err := sc.FindAllDistros()
	assert.Nil(err)
	assert.Len(found, numDistros)
}

////////////////////////////////////////////////////////////////////////////////
type DistroCostConnectorSuite struct {
	ctx       Connector
	isMock    bool
	numLoop   int
	starttime time.Time

	suite.Suite
}

// Initialize the ConnectorSuites
func TestDistroCostConnectorSuite(t *testing.T) {
	assert := assert.New(t) // nolint

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
	s.starttime = time.Now()

	// Insert data
	for i := 0; i < s.numLoop; i++ {
		testTask1 := &task.Task{
			Id:         fmt.Sprintf("task_%d", i*2),
			DistroId:   fmt.Sprintf("%d", i),
			TimeTaken:  time.Duration(i),
			StartTime:  s.starttime,
			FinishTime: s.starttime.Add(time.Duration(1)),
		}
		assert.NoError(testTask1.Insert())

		testTask2 := &task.Task{
			Id:         fmt.Sprintf("task_%d", i*2+1),
			DistroId:   fmt.Sprintf("%d", i),
			TimeTaken:  time.Duration(i),
			StartTime:  s.starttime,
			FinishTime: s.starttime.Add(time.Duration(1)),
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

	suite.Run(t, s)
}

func TestMockDistroConnectorSuite(t *testing.T) {
	s := new(DistroCostConnectorSuite)

	var settings = make(map[string]interface{})
	settings["instance_type"] = "type"

	s.ctx = &MockConnector{
		MockDistroConnector: MockDistroConnector{
			CachedTasks: []task.Task{
				{Id: "task1", DistroId: "distro1", TimeTaken: time.Duration(1),
					StartTime: s.starttime, FinishTime: s.starttime.Add(time.Duration(1))},
				{Id: "task2", DistroId: "distro2", TimeTaken: time.Duration(1),
					StartTime: s.starttime, FinishTime: s.starttime.Add(time.Duration(1))},
				{Id: "task3", DistroId: "distro2", TimeTaken: time.Duration(1),
					StartTime: s.starttime, FinishTime: s.starttime.Add(time.Duration(1))},
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
		found, err := s.ctx.FindCostByDistroId("distro1", s.starttime, time.Duration(1))
		s.NoError(err)
		s.Equal(found.SumTimeTaken, time.Duration(1))
		s.Equal(found.Provider, "ec2")
		s.Equal(found.ProviderSettings["instance_type"], "type")

		found, err = s.ctx.FindCostByDistroId("distro2", s.starttime, time.Duration(1))
		s.NoError(err)
		s.Equal(found.SumTimeTaken, time.Duration(1)*2)
		s.Equal(found.Provider, "gce")
		s.Equal(found.ProviderSettings["instance_type"], "type")

		// Searching for a distro that exists but all tasks have not yet finished
		// should not fail, but be "empty"
		found, err = s.ctx.FindCostByDistroId("distro2", time.Now().AddDate(0, -1, 0), time.Duration(1))
		s.NoError(err)
		s.Equal(time.Duration(0), found.SumTimeTaken)
		s.Equal("", found.Provider)
		s.Equal(0, len(found.ProviderSettings))
	} else {
		// Finding each distro's sum of time taken should succeed
		for i := 0; i < s.numLoop; i++ {
			found, err := s.ctx.FindCostByDistroId(fmt.Sprintf("%d", i), s.starttime, time.Duration(1))
			s.NoError(err)
			s.Equal(time.Duration(i)*2, found.SumTimeTaken)
			s.Equal("provider", found.Provider)
			s.Equal(fmt.Sprintf("value_%d", i), found.ProviderSettings["instance_type"])

			// Searching for a distro that exists but all tasks have not yet finished
			// should not fail, but be "empty"
			found, err = s.ctx.FindCostByDistroId(fmt.Sprintf("%d", i), time.Now().AddDate(0, -1, 0), time.Duration(1))
			s.NoError(err)
			s.Equal(time.Duration(0), found.SumTimeTaken)
			s.Equal("", found.Provider)
			s.Equal(0, len(found.ProviderSettings))
		}

		// Searching for a distro that doesn't exist should return an error
		found, err := s.ctx.FindCostByDistroId("fake_distro", s.starttime, time.Duration(1))
		s.Error(err)
		s.Nil(found)
	}
}
