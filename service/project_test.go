package service

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/stretchr/testify/assert"
)

func TestValidateBbProject(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection))
	p := model.ProjectRef{
		Identifier: "proj1",
	}
	assert.NoError(p.Insert())
	assert.NoError(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil))

	assert.NoError(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil))

	assert.NoError(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject: "BFG",
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketSearchProjects: []string{"BF", "BFG"},
	}, nil))
}

func TestBuildBaronPluginConfigureBFSuggestion(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(model.ProjectRefCollection))
	p := model.ProjectRef{
		Identifier: "proj1",
	}
	assert.NoError(p.Insert())
	assert.NoError(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionUsername:    "user",
		BFSuggestionPassword:    "pass",
		BFSuggestionTimeoutSecs: 10,
	}, nil))

	assert.NoError(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: 10,
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:  "BFG",
		TicketSearchProjects: []string{"BF", "BFG"},
		BFSuggestionUsername: "user",
		BFSuggestionPassword: "pass",
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionTimeoutSecs: 10,
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: 10,
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionPassword:    "pass",
		BFSuggestionTimeoutSecs: 10,
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: 0,
	}, nil))

	assert.Error(model.ValidateBbProject(t.Context(), "proj1", evergreen.BuildBaronSettings{
		TicketCreateProject:     "BFG",
		TicketSearchProjects:    []string{"BF", "BFG"},
		BFSuggestionServer:      "https://evergreen.mongodb.com",
		BFSuggestionTimeoutSecs: -1,
	}, nil))
}
