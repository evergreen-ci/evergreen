package task

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestExpectedDuration(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	index := db.Index{Key: db.IndexKey{Fields: []db.IndexFieldSpec{{Name: "branch", Value: 1}, {Name: "build_variant", Value: 1}, {Name: "display_name", Value: 1}, {Name: "status", Value: 1}, {Name: "finish_time", Value: 1}, {Name: "start_time", Value: 1}}}}
	assert.NoError(db.CreateIndexes(context.Background(), Collection, index))
	bv := "bv"
	project := "proj"
	now := time.Now()

	t1 := Task{
		Id:           "t1",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-10 * time.Minute),
		TimeTaken:    10 * time.Minute,
	}
	assert.NoError(t1.Insert())
	t2 := Task{
		Id:           "t2",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-30 * time.Minute),
		TimeTaken:    30 * time.Minute,
	}
	assert.NoError(t2.Insert())
	t3 := Task{
		Id:           "t3",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-35 * time.Minute),
		TimeTaken:    35 * time.Minute,
	}
	assert.NoError(t3.Insert())
	t4 := Task{
		Id:           "t4",
		BuildVariant: bv,
		Project:      project,
		Status:       evergreen.TaskSucceeded,
		FinishTime:   now,
		StartTime:    now.Add(-25 * time.Minute),
		TimeTaken:    25 * time.Minute,
	}
	assert.NoError(t4.Insert())

	results, err := getExpectedDurationsForWindow("", project, bv, now.Add(-1*time.Hour), now)
	assert.NoError(err)
	assert.EqualValues(25*time.Minute, results[0].ExpectedDuration)
	assert.InDelta(9.35*float64(time.Minute), results[0].StdDev, 0.01*float64(time.Minute))
}
