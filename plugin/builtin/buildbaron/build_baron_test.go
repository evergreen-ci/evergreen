package buildbaron

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
		"Projects": map[string]evergreen.BuildBaronProject{},
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

func TestBuildBaronPluginConfigureAltEndpoint(t *testing.T) {
	assert := assert.New(t)

	bbPlugin := BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointUsername:    "user",
				AlternativeEndpointPassword:    "pass",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Nil(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:         "BFG",
				TicketSearchProjects:        []string{"BF", "BFG"},
				AlternativeEndpointUsername: "user",
				AlternativeEndpointPassword: "pass",
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointPassword:    "pass",
				AlternativeEndpointTimeoutSecs: 10,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: 0,
			},
		},
	}))

	bbPlugin = BuildBaronPlugin{}
	assert.Error(bbPlugin.Configure(map[string]interface{}{
		"Projects": map[string]evergreen.BuildBaronProject{
			"proj": evergreen.BuildBaronProject{
				TicketCreateProject:            "BFG",
				TicketSearchProjects:           []string{"BF", "BFG"},
				AlternativeEndpointURL:         "https://evergreen.mongodb.com",
				AlternativeEndpointTimeoutSecs: -1,
			},
		},
	}))
}
