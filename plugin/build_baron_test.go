package plugin

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func TestBuildBaronPluginConfigure(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj1": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
			"proj2": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject: "BFG",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketSearchProjects: []string{"BF", "BFG"},
			},
		},
	}))
}

func TestBuildBaronPluginConfigureBFSuggestion(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
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
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:  "BFG",
				TicketSearchProjects: []string{"BF", "BFG"},
				BFSuggestionUsername: "user",
				BFSuggestionPassword: "pass",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
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
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: 0,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:     "BFG",
				TicketSearchProjects:    []string{"BF", "BFG"},
				BFSuggestionServer:      "https://evergreen.mongodb.com",
				BFSuggestionTimeoutSecs: -1,
			},
		},
	}))
}
