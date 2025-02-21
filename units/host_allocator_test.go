package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSingleTaskDistroHostAllocatorJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	assert := assert.New(t)
	require := require.New(t)

	env := &mock.Environment{}
	require.NoError(env.Configure(ctx))

	require.NoError(db.ClearCollections(distro.Collection, model.TaskQueuesCollection, host.Collection))
	defer func() {
		assert.NoError(db.ClearCollections(distro.Collection, model.TaskQueuesCollection, host.Collection))
	}()

	d := distro.Distro{
		Id:               "d",
		SingleTaskDistro: true,
	}
	require.NoError(d.Insert(ctx))

	tq := model.TaskQueue{
		Distro: d.Id,
		Queue: []model.TaskQueueItem{
			{
				Id: "t1",
			},
			{
				Id: "t2",
			},
			{
				Id: "t3",
			},
		},
		DistroQueueInfo: model.DistroQueueInfo{
			Length:                    3,
			LengthWithDependenciesMet: 2,
		},
	}
	require.NoError(tq.Save())

	h := host.Host{
		Id:     "h1",
		Distro: d,
		Status: evergreen.HostProvisioning,
	}
	require.NoError(h.Insert(ctx))

	j := NewHostAllocatorJob(env, d.Id, time.Now())

	j.Run(ctx)

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	hosts, err := host.AllActiveHosts(ctx, d.Id)
	require.NoError(err)
	require.Len(hosts, 2)
}
