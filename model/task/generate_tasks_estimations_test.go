package task

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestGenerateTasksEstimations(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))
	bv := "bv"
	project := "proj"

	t1 := Task{
		Id:                         "t1",
		BuildVariant:               bv,
		Project:                    project,
		NumGeneratedTasks:          1,
		NumActivatedGeneratedTasks: 10,
		RevisionOrderNumber:        1,
	}
	assert.NoError(t1.Insert())
	t2 := Task{
		Id:                         "t2",
		BuildVariant:               bv,
		Project:                    project,
		NumGeneratedTasks:          2,
		NumActivatedGeneratedTasks: 20,
		RevisionOrderNumber:        2,
	}
	assert.NoError(t2.Insert())
	t3 := Task{
		Id:                         "t3",
		BuildVariant:               bv,
		Project:                    project,
		NumGeneratedTasks:          3,
		NumActivatedGeneratedTasks: 30,
		RevisionOrderNumber:        3,
	}
	assert.NoError(t3.Insert())
	t4 := Task{
		Id:                  "t4",
		BuildVariant:        bv,
		Project:             project,
		RevisionOrderNumber: 4,
	}
	assert.NoError(t4.Insert())

	err := t4.setGenerateTasksEstimations()
	assert.NoError(err)
	assert.Equal(2, *t4.EstimatedNumGeneratedTasks)
	assert.Equal(20, *t4.EstimatedNumActivatedGeneratedTasks)
}
