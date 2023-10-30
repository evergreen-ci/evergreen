package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/utility"
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
	apiEntry.BuildFromService(entry)

	origEntry := apiEntry.ToService()
	assert.EqualValues(entry, origEntry)
}

func TestAPIFileBuildFromService(t *testing.T) {
	assert := assert.New(t)
	file := artifact.File{
		Name:           "file1",
		Link:           "l1",
		Visibility:     "public",
		IgnoreForFetch: true,
		ContentType:    "text/plain",
	}
	apiFile := APIFile{}
	apiFile.BuildFromService(file)

	origFile := apiFile.ToService()
	assert.EqualValues(file, origFile)

	env := evergreen.GetEnvironment()
	apiFile.GetLogURL(env, "t1", 1)
	assert.Equal("https://localhost:4173/taskFile/t1/1/file1", utility.FromStringPtr(apiFile.URLParsley))

	file = artifact.File{
		Name:           "some complex/file name",
		Link:           "l1",
		Visibility:     "public",
		IgnoreForFetch: true,
		ContentType:    "text/plain",
	}
	apiFile = APIFile{}
	apiFile.BuildFromService(file)
	apiFile.GetLogURL(env, "t1", 1)
	assert.Equal("https://localhost:4173/taskFile/t1/1/some%20complex%2Ffile%20name", utility.FromStringPtr(apiFile.URLParsley))

}
