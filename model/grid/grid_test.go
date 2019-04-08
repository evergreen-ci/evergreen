package grid

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

const (
	numVersions     = 3
	tasksPerVersion = 10
	testsPerTask    = 50
	projName        = "crumpet"
)

func TestFetchFailures(t *testing.T) {
	assert := assert.New(t)
	testutil.HandleTestingErr(db.ClearCollections(model.VersionCollection, task.Collection, testresult.Collection), t, "error cleraing collections")
	generateData(assert)
	current := model.Version{
		RevisionOrderNumber: numVersions + 1,
		Identifier:          projName,
	}
	failures, err := FetchFailures(current, numVersions+1)
	assert.NoError(err)
	assert.Len(failures, numVersions*tasksPerVersion/2*testsPerTask)
	for _, failure := range failures {
		last := failure.Id.Task[len(failure.Id.Task)-1 : len(failure.Id.Task)]
		lastInt, err := strconv.Atoi(last)
		assert.NoError(err)
		assert.Equal(1, lastInt%2) //odd numbered tasks are failures
	}
}

func TestFetchRevisionOrderFailures(t *testing.T) {
	assert := assert.New(t)
	testutil.HandleTestingErr(db.ClearCollections(model.VersionCollection, task.Collection, testresult.Collection), t, "error cleraing collections")
	generateData(assert)
	current := model.Version{
		RevisionOrderNumber: numVersions + 1,
		Identifier:          projName,
	}
	failures, err := FetchRevisionOrderFailures(current, numVersions+1)
	assert.NoError(err)
	assert.Len(failures[0].Failures, numVersions*tasksPerVersion/2*testsPerTask)
	for _, failure := range failures[0].Failures {
		last := failure.TaskId[len(failure.TaskId)-1 : len(failure.TaskId)]
		lastInt, err := strconv.Atoi(last)
		assert.NoError(err)
		assert.Equal(1, lastInt%2) //odd numbered tasks are failures
	}
}

func generateData(assert *assert.Assertions) {
	for i := 0; i < numVersions; i++ {
		v := model.Version{
			Id:                  fmt.Sprintf("v%d", i),
			RevisionOrderNumber: i + 1,
			Identifier:          projName,
		}
		assert.NoError(v.Insert())
		for j := 0; j < tasksPerVersion; j++ {
			t := task.Task{
				Id:                  fmt.Sprintf("task%d%d", i, j),
				Project:             projName,
				RevisionOrderNumber: i + 1,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         fmt.Sprintf("task%d%d", i, j),
				BuildVariant:        "foo",
				Execution:           0,
			}
			if j%2 == 0 {
				t.Status = evergreen.TaskSucceeded
			} else {
				t.Status = evergreen.TaskFailed
			}
			assert.NoError(t.Insert())
			for k := 0; k < testsPerTask; k++ {
				r := testresult.TestResult{
					TaskID:    t.Id,
					TestFile:  fmt.Sprintf("test%d", k),
					Execution: 0,
				}
				if j%2 == 0 {
					r.Status = evergreen.TestSucceededStatus
				} else {
					r.Status = evergreen.TestFailedStatus
				}
				assert.NoError(r.Insert())
			}
		}
		// generate some extraneous data
		t := task.Task{
			Id:                  fmt.Sprintf("extra_task%d", i),
			Project:             projName,
			RevisionOrderNumber: i + 1,
			Requester:           evergreen.RepotrackerVersionRequester,
			DisplayName:         fmt.Sprintf("extra_task%d", i),
			BuildVariant:        "foo",
			Execution:           1,
			Status:              evergreen.TaskFailed,
		}
		assert.NoError(t.Insert())
		r := testresult.TestResult{
			TaskID:    t.Id,
			TestFile:  fmt.Sprintf("test%d", i),
			Execution: 0,
			Status:    evergreen.TestFailedStatus,
		}
		assert.NoError(r.Insert())
	}
}
