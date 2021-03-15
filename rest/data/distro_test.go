package data

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindDistroById(t *testing.T) {
	assert := assert.New(t)
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	assert.NoError(err)
	require.NotNil(t, session)
	defer session.Close()

	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase(), "Error dropping database")

	sc := &DBConnector{}
	id := fmt.Sprintf("distro_%d", rand.Int())
	d := &distro.Distro{
		Id: id,
	}
	assert.Nil(d.Insert())
	found, err := sc.FindDistroById(id)
	assert.NoError(err)
	assert.Equal(found.Id, id, "The _ids should match")
	assert.NotEqual(found.Id, -1, "The _ids should not match")
}

func TestFindAllDistros(t *testing.T) {
	assert := assert.New(t)
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	assert.NoError(err)
	require.NotNil(t, session)
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase(), "Error dropping database")

	sc := &DBConnector{}

	numDistros := 10
	for i := 0; i < numDistros; i++ {
		d := &distro.Distro{
			Id: fmt.Sprintf("distro_%d", rand.Int()),
		}
		assert.Nil(d.Insert())
	}

	found, err := sc.FindAllDistros()
	assert.NoError(err)
	assert.Len(found, numDistros)
}

func TestDeleteDistroById(t *testing.T) {
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	defer func() {
		assert.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	}()

	sc := &DBConnector{}

	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert())

	queue := model.TaskQueue{
		Distro: d.Id,
		Queue:  []model.TaskQueueItem{{Id: "task"}},
	}
	require.NoError(t, queue.Save())

	require.NoError(t, sc.DeleteDistroById(d.Id))

	dbDistro, err := sc.FindDistroById(d.Id)
	assert.Error(t, err)
	assert.Zero(t, dbDistro)

	dbQueue, err := model.LoadTaskQueue(queue.Distro)
	require.NoError(t, err)
	assert.Empty(t, dbQueue.Queue)
}
