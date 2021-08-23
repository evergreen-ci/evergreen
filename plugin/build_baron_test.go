package plugin

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
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

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject: "BFG",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))
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

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
				BFSuggestionUsername: "user",
				BFSuggestionPassword: "pass",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
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

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 0,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronSettings{
			"proj": evergreen.BuildBaronSettings{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: -1,
			},
		},
	}))
}
