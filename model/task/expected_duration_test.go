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
	indices := []map[string]interface{}{{"name": "branch_1_build_variant_1_display_name_1_status_1_finish_time_1_start_time_1", "key": map[string]int{"branch": 1}}}
	_ = evergreen.GetEnvironment().DB().RunCommand(context.Background(), map[string]interface{}{"createIndexes": Collection, "indexes": indices})
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
