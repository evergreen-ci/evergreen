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
)

var numDistros = 10

func TestFindAllDistros(t *testing.T) {
	assert := assert.New(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindAllDistros")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	testutil.HandleTestingErr(session.DB(testConfig.Database.DB).DropDatabase(), t, "Error dropping database")

	sc := &DBConnector{}

	var distros []*distro.Distro
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

func TestFindCostByDistroId(t *testing.T) {
	assert := assert.New(t)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFindCostByDistroId")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	assert.Nil(db.Clear(task.Collection))

	sc := &DBConnector{}
	numTaskSet := 10

	// Add task documents in the database
	for i := 0; i < numTaskSet; i++ {
		testTask1 := &task.Task{
			Id:        fmt.Sprintf("task_%d", i*2),
			DistroId:  fmt.Sprintf("%d", i),
			TimeTaken: time.Duration(i),
		}
		assert.Nil(testTask1.Insert())

		testTask2 := &task.Task{
			Id:        fmt.Sprintf("task_%d", i*2+1),
			DistroId:  fmt.Sprintf("%d", i),
			TimeTaken: time.Duration(i),
		}
		assert.Nil(testTask2.Insert())
	}

	// Finding each distro's sum of time taken should succeed
	for i := 0; i < numTaskSet; i++ {
		found, err := sc.FindCostByDistroId(fmt.Sprintf("%d", i))
		assert.Nil(err)
		assert.Equal(found.SumTimeTaken, time.Duration(i)*2)
	}

	// Searching for a distro that doesn't exist should fail with an APIError
	found, err := sc.FindCostByDistroId("fake_distro")
	assert.NotNil(err)
	assert.Nil(found)
	assert.IsType(err, &rest.APIError{})
	apiErr, ok := err.(*rest.APIError)
	assert.Equal(ok, true)
	assert.Equal(apiErr.StatusCode, http.StatusNotFound)
}
