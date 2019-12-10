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
	newDefinitions := []model.ProjectAlias{{Alias: evergreen.GithubAlias}}
	exists, err := verifyAliasExists(evergreen.GithubAlias, "evergreen", newDefinitions, []string{})
	assert.NoError(t, err)
	assert.True(t, exists)

	// a definition already exists
	alias := &model.ProjectAlias{
		Alias:     evergreen.GithubAlias,
		ProjectID: "evergreen",
	}
	require.NoError(t, alias.Upsert())
	aliases, err := model.FindAliasInProject("evergreen", evergreen.GithubAlias)
	require.NoError(t, err)
	require.Len(t, aliases, 1)
	exists, err = verifyAliasExists(evergreen.GithubAlias, "evergreen", []model.ProjectAlias{}, []string{})
	assert.NoError(t, err)
	assert.True(t, exists)

	// the only existing definition is being deleted
	exists, err = verifyAliasExists(evergreen.GithubAlias, "evergreen", []model.ProjectAlias{}, []string{aliases[0].ID.Hex()})
	assert.NoError(t, err)
	assert.False(t, exists)
}
