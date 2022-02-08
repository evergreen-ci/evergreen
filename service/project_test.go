package service

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyAliasExists(t *testing.T) {
	assert.NoError(t, db.Clear(model.ProjectAliasCollection))

	// a new definition for the github alias is added
	newDefinitions := []model.ProjectAlias{{Alias: evergreen.GithubPRAlias}}
	exists, err := verifyAliasExists(evergreen.GithubPRAlias, "evergreen", newDefinitions, []string{})
	assert.NoError(t, err)
	assert.True(t, exists)

	// a definition already exists
	alias := &model.ProjectAlias{
		Alias:     evergreen.GithubPRAlias,
		ProjectID: "evergreen",
	}
	require.NoError(t, alias.Upsert())
	aliases, err := model.FindAliasInProjectRepoOrConfig("evergreen", evergreen.GithubPRAlias)
	require.NoError(t, err)
	require.Len(t, aliases, 1)
	exists, err = verifyAliasExists(evergreen.GithubPRAlias, "evergreen", []model.ProjectAlias{}, []string{})
	assert.NoError(t, err)
	assert.True(t, exists)

	// the only existing definition is being deleted
	exists, err = verifyAliasExists(evergreen.GithubPRAlias, "evergreen", []model.ProjectAlias{}, []string{aliases[0].ID.Hex()})
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestValidateBbProject(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection))
	p := model.ProjectRef{
		Identifier: "proj1",
	}
	assert.NoError(p.Insert())
	assert.Nil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil, true))

	assert.Nil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil, true))

	assert.Nil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject: "BFG",
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil, true))
}

func TestBuildBaronPluginConfigureBFSuggestion(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection))
	p := model.ProjectRef{
		Identifier: "proj1",
	}
	assert.NoError(p.Insert())
	assert.Nil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionUsername:    "user",
		BFSuggestionPassword:    "pass",
		BFSuggestionTimeoutSecs: 10,
	}, nil, true))

	assert.Nil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: 10,
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
		BFSuggestionUsername: "user",
		BFSuggestionPassword: "pass",
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionTimeoutSecs: 10,
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: 10,
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionPassword:    "pass",
		BFSuggestionTimeoutSecs: 10,
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: 0,
	}, nil, true))

	assert.NotNil(model.ValidateBbProject("proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: -1,
	}, nil, true))
}
