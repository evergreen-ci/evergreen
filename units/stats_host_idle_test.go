package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncrementCostForDuration(t *testing.T) {
	require.NoError(t, db.Clear(host.Collection))
	h1 := &host.Host{
		Id:     "h1",
		Distro: distro.Distro{Provider: evergreen.ProviderNameMock},
	}
	assert.NoError(t, h1.Insert())

	startTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	finishTime := startTime.Add(time.Hour)

	j := newHostIdleJob()
	j.env = evergreen.GetEnvironment()
	j.settings = j.env.Settings()

	j.host = h1
	j.StartTime = startTime
	j.FinishTime = finishTime

	cost, err := j.incrementCostForDuration(context.Background())
	assert.NoError(t, err)
	assert.EqualValues(t, 60, cost)

	h1, err = host.FindOneId("h1")
	assert.NoError(t, err)
	assert.EqualValues(t, 60, h1.TotalCost)
}
