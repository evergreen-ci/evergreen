package data

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteDistroById(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	session, _, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(t, err)
	defer session.Close()
	require.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	defer func() {
		assert.NoError(t, session.DB(testConfig.Database.DB).DropDatabase())
	}()

	d := distro.Distro{
		Id: "distro",
	}
	require.NoError(t, d.Insert())

	queue := model.TaskQueue{
		Distro: d.Id,
		Queue:  []model.TaskQueueItem{{Id: "task"}},
	}
	require.NoError(t, queue.Save())

	require.NoError(t, DeleteDistroById(d.Id))

	dbDistro, err := distro.FindOneId(d.Id)
	assert.Error(t, err)
	assert.Zero(t, dbDistro)

	dbQueue, err := model.LoadTaskQueue(queue.Distro)
	require.NoError(t, err)
	assert.Empty(t, dbQueue.Queue)
}
