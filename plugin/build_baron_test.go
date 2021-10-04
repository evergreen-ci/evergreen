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

func TestBuildBaronPluginConfigure(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 1)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj1": evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
			"proj2": evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 2)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject: "BFG",
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)
}

func TestBuildBaronPluginConfigureBFSuggestion(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionUsername:    "user",
				BFSuggestionPassword:    "pass",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 1)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 1)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
				BFSuggestionUsername: "user",
				BFSuggestionPassword: "pass",
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionPassword:    "pass",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 0,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: -1,
			},
		},
	}))
	assert.Len(bbPlugin.opts.Projects, 0)
}

func TestBbGetProject(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, model.ProjectRefCollection, model.ParserProjectCollection),
		"Error clearing task collections")

	myProject := model.ProjectRef{
		Id: "proj",
		BuildBaronSettings: evergreen.BuildBaronSettings{
			TicketCreateProject: "BFG",
		},
	}
	myProject2 := model.ProjectRef{
		Id: "proj2",
		BuildBaronSettings: evergreen.BuildBaronSettings{
			TicketCreateProject: "123",
		},
	}
	myProjectParser := model.ParserProject{
		Id: "proj2",
		BuildBaronSettings: &evergreen.BuildBaronSettings{
			TicketCreateProject: "ABC",
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

	env := evergreen.GetEnvironment().Settings()
	flags := evergreen.ServiceFlags{
		PluginAdminPageDisabled: true,
	}
	assert.NoError(t, evergreen.SetServiceFlags(flags))

	bbProj, ok1 := BbGetProject(env, testTask.Project, testTask.Version)
	bbProj2, ok2 := BbGetProject(env, testTask2.Project, testTask2.Version)
	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, bbProj.TicketCreateProject, "BFG")
	assert.Equal(t, bbProj2.TicketCreateProject, "ABC")
}
