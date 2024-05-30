package task

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestGenerateTasksEstimations(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	bv := "bv"
	project := "proj"
	displayName := "display_name"

	t1 := Task{
		Id:                         "t1",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Requester:                  evergreen.RepotrackerVersionRequester,
		NumGeneratedTasks:          1,
		NumActivatedGeneratedTasks: 10,
		RevisionOrderNumber:        1,
	}
	assert.NoError(t1.Insert())
	t2 := Task{
		Id:                         "t2",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Requester:                  evergreen.RepotrackerVersionRequester,
		NumGeneratedTasks:          2,
		NumActivatedGeneratedTasks: 20,
		RevisionOrderNumber:        2,
	}
	assert.NoError(t2.Insert())
	t3 := Task{
		Id:                         "t3",
		DisplayName:                displayName,
		BuildVariant:               bv,
		Project:                    project,
		Requester:                  evergreen.RepotrackerVersionRequester,
		NumGeneratedTasks:          3,
		NumActivatedGeneratedTasks: 30,
		RevisionOrderNumber:        3,
	}
	assert.NoError(t3.Insert())
	t4 := Task{
		GenerateTask:        true,
		Id:                  "t4",
		DisplayName:         displayName,
		Requester:           evergreen.RepotrackerVersionRequester,
		BuildVariant:        bv,
		Project:             project,
		RevisionOrderNumber: 4,
	}
	assert.NoError(t4.Insert())

	err := t4.setGenerateTasksEstimations()
	assert.NoError(err)
	assert.Equal(2, utility.FromIntPtr(t4.EstimatedNumGeneratedTasks))
	assert.Equal(20, utility.FromIntPtr(t4.EstimatedNumActivatedGeneratedTasks))
}
