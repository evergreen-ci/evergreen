package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastContainerFinishTimeJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	mockCloud := cloud.GetMockProvider()
	mockCloud.Reset()

	startTimeOne := time.Now()
	startTimeTwo := time.Now().Add(-10 * time.Minute)
	durationOne := 5 * time.Minute
	durationTwo := 30 * time.Minute

	require.NoError(t, db.ClearCollections(host.Collection, task.Collection))

	p1 := &host.Host{
		Id:            "p1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	assert.NoError(p1.Insert())

	h1 := &host.Host{
		Id:          "h1",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t1",
	}
	assert.NoError(h1.Insert())
	h2 := &host.Host{
		Id:          "h2",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t2",
	}
	assert.NoError(h2.Insert())

	t1 := &task.Task{
		Id: "t1",
		DurationPrediction: util.CachedDurationValue{
			Value: durationOne,
		},
		StartTime: startTimeOne,
	}
	assert.NoError(t1.Insert())
	t2 := &task.Task{
		Id: "t2",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		},
		StartTime: startTimeTwo,
	}
	assert.NoError(t2.Insert())

	j := NewLastContainerFinishTimeJob("one")
	assert.False(j.Status().Completed)

	j.Run(context.Background())

	assert.NoError(j.Error())
	assert.True(j.Status().Completed)

	parent1, err := host.FindOne(ctx, host.ById("p1"))
	assert.NoError(err)
	assert.WithinDuration(startTimeTwo.Add(durationTwo), parent1.LastContainerFinishTime, time.Millisecond, "parent host's last container finish time should be set to latest finish time")

}
