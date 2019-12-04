package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordTaskCost(t *testing.T) {
	require.NoError(t, db.ClearCollections(host.Collection, task.Collection))
	h1 := &host.Host{
		Id:     "h1",
		Distro: distro.Distro{Provider: evergreen.ProviderNameMock},
	}
	assert.NoError(t, h1.Insert())
	t1 := &task.Task{
		Id:         "t1",
		StartTime:  time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
		FinishTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC).Add(time.Duration(time.Hour)),
	}
	assert.NoError(t, t1.Insert())

	j := newTaskEndJob()
	j.env = evergreen.GetEnvironment()
	j.host = h1
	j.task = t1

	assert.NoError(t, j.recordTaskCost(context.Background()))

	h1, err := host.FindOneId(h1.Id)
	assert.NoError(t, err)
	assert.EqualValues(t, 60, h1.TotalCost)

	t1, err = task.FindOneId(t1.Id)
	assert.NoError(t, err)
	assert.EqualValues(t, 60, t1.Cost)
}
