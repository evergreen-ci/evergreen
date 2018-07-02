package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/stretchr/testify/assert"
)

func TestArtifactModels(t *testing.T) {
	assert := assert.New(t)
	entry := artifact.Entry{
		TaskId:          "myTask",
		TaskDisplayName: "task",
		BuildId:         "b1",
		Execution:       1,
		Files: []artifact.File{
			{
				Name: "file1",
				Link: "l1",
			},
			{
				Name: "file2",
				Link: "l2",
			},
		},
	}
	apiEntry := APIEntry{}
	err := apiEntry.BuildFromService(entry)
	assert.NoError(err)

	origEntry, err := apiEntry.ToService()
	assert.NoError(err)
	assert.EqualValues(entry, origEntry)
}
