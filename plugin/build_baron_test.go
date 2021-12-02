package plugin

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBbGetProject(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, model.ParserProjectCollection),
		"Error clearing task collections")

	myProject := model.ProjectRef{
		Id: "proj",
		BuildBaronSettings: evergreen.BuildBaronSettings{
			TicketCreateProject:  "BFG",
			TicketSearchProjects: []string{"EVG"},
		},
	}
	myProject2 := model.ProjectRef{
		Id: "proj2",
		BuildBaronSettings: evergreen.BuildBaronSettings{
			TicketCreateProject:  "123",
			TicketSearchProjects: []string{"EVG"},
		},
	}
	myProjectParser := model.ParserProject{
		Id: "proj2",
		BuildBaronSettings: &evergreen.BuildBaronSettings{
			TicketCreateProject:  "ABC",
			TicketSearchProjects: []string{"EVG"},
		},
	}
	testTask := task.Task{
		Id:        "testone",
		Activated: true,
		Project:   "proj",
		Version:   "v1",
	}
	testTask2 := task.Task{
		Id:        "testtwo",
		Activated: true,
		Project:   "proj2",
		Version:   "proj2",
	}

	assert.NoError(t, testTask.Insert())
	assert.NoError(t, myProject.Insert())
	assert.NoError(t, myProject2.Insert())
	assert.NoError(t, myProjectParser.Insert())

	bbProj, ok1 := BbGetProject(testTask.Project, testTask.Version)
	bbProj2, ok2 := BbGetProject(testTask2.Project, testTask2.Version)
	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, bbProj.TicketCreateProject, "BFG")
	assert.Equal(t, bbProj2.TicketCreateProject, "123")
}
