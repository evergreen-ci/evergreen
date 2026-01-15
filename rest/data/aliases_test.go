package data

import (
	"sort"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AliasSuite struct {
	suite.Suite
}

func TestAliasSuite(t *testing.T) {
	suite.Run(t, new(AliasSuite))
}

func (a *AliasSuite) SetupTest() {
	session, _, _ := db.GetGlobalSessionFactory().GetSession(a.T().Context())
	a.Require().NoError(session.DB(testConfig.Database.DB).DropDatabase())

	aliases := []model.ProjectAlias{
		{
			ProjectID: "project_id",
			Alias:     "foo",
			Variant:   "variant",
			Task:      "project_ref_task",
		},
		{
			ProjectID: "project_id",
			Alias:     "bar",
			Variant:   "not_this_variant",
			Task:      "project_ref_task",
		},
		{
			ProjectID: "project_id",
			Alias:     "foo",
			Variant:   "another_variant",
			Task:      "project_ref_task",
		},
		{
			ProjectID: "project_id",
			Alias:     evergreen.CommitQueueAlias,
			Variant:   "commit_queue_variant",
			Task:      "project_ref_task",
		},
		{
			ProjectID: "other_project_id",
			Alias:     "baz",
			Variant:   "variant",
			Task:      "project_ref_task",
		},
		{
			ProjectID: "other_project_id",
			Alias:     "delete_me",
			Variant:   "variant",
			Task:      "project_ref_task",
		},
		{
			ProjectID: "repo_id",
			Alias:     "from_repo",
			Variant:   "repo_variant",
			Task:      "repo_task",
		},
		{
			ProjectID: "repo_id",
			Alias:     evergreen.CommitQueueAlias,
			Variant:   "repo_variant",
			Task:      "repo_task",
		},
		{
			ProjectID: "repo_id",
			Alias:     evergreen.GithubPRAlias,
			Variant:   "repo_variant",
			Task:      "repo_task",
		},
	}
	projectRef := model.ProjectRef{
		Identifier:            "project_id",
		Id:                    "project_id",
		VersionControlEnabled: utility.TruePtr(),
	}
	newProjectRef := model.ProjectRef{
		Identifier: "new_project_id",
		Id:         "new_project_id",
	}
	otherProjectRef := model.ProjectRef{
		Identifier: "other_project_id",
		Id:         "other_project_id",
	}
	projectConfig := &model.ProjectConfig{
		Id:      "project_id",
		Project: "project_id",
		ProjectConfigFields: model.ProjectConfigFields{
			PatchAliases: []model.ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     "alias-2",
					Task:      "project_config_task",
				},
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     "alias-1",
					Task:      "project_config_task",
				},
			},
			CommitQueueAliases: []model.ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     evergreen.CommitQueueAlias,
					Task:      "project_config_task",
				},
			},
			GitHubPRAliases: []model.ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     evergreen.GithubPRAlias,
					Task:      "project_config_task",
				},
			},
			GitHubChecksAliases: []model.ProjectAlias{
				{
					ID:        mgobson.NewObjectId(),
					ProjectID: "project-1",
					Alias:     evergreen.GithubChecksAlias,
					Task:      "project_config_task",
				},
			},
		}}
	a.NoError(otherProjectRef.Insert(a.T().Context()))
	a.NoError(projectRef.Insert(a.T().Context()))
	a.NoError(newProjectRef.Insert(a.T().Context()))
	a.NoError(projectConfig.Insert(a.T().Context()))
	for _, v := range aliases {
		a.NoError(v.Upsert(a.T().Context()))
	}
}

func (a *AliasSuite) TestFindProjectAliasesMergedWithProjectConfig() {
	found, err := FindMergedProjectAliases(a.T().Context(), "project_id", "", nil, true)
	a.Require().NoError(err)
	a.Require().Len(found, 6)
	sort.Slice(found, func(i, j int) bool {
		return utility.FromStringPtr(found[i].Alias) < utility.FromStringPtr(found[j].Alias)
	})
	a.Equal(evergreen.CommitQueueAlias, utility.FromStringPtr(found[0].Alias))
	a.Equal(evergreen.GithubPRAlias, utility.FromStringPtr(found[1].Alias))
	a.Equal(evergreen.GithubChecksAlias, utility.FromStringPtr(found[2].Alias))
	a.Equal("bar", utility.FromStringPtr(found[3].Alias))
	a.Equal("foo", utility.FromStringPtr(found[4].Alias))
	a.Equal("foo", utility.FromStringPtr(found[5].Alias))
}

func (a *AliasSuite) TestFindMergedProjectAliases() {
	// project ref only
	found, err := FindMergedProjectAliases(a.T().Context(), "project_id", "", nil, false)
	a.NoError(err)
	a.Len(found, 4)

	// project ref merged with repo
	found, err = FindMergedProjectAliases(a.T().Context(), "project_id", "repo_id", nil, false)
	a.NoError(err)
	a.Len(found, 5)

	// all non-existent
	found, err = FindMergedProjectAliases(a.T().Context(), "non-existent", "non-existent", nil, false)
	a.NoError(err)
	a.Empty(found)

	// repo only
	found, err = FindMergedProjectAliases(a.T().Context(), "non-existent", "repo_id", nil, false)
	a.NoError(err)
	a.Len(found, 3)

	// project ref, repo, project config and added aliases
	aliasesToAdd := []restModel.APIProjectAlias{
		{Alias: utility.ToStringPtr(evergreen.GitTagAlias), Task: utility.ToStringPtr("added_task")},
	}
	found, err = FindMergedProjectAliases(a.T().Context(), "project_id", "repo_id", aliasesToAdd, true)
	a.NoError(err)
	a.Len(found, 7)
	for _, alias := range found {
		switch utility.FromStringPtr(alias.Alias) {
		case evergreen.CommitQueueAlias:
			a.Equal("project_ref_task", utility.FromStringPtr(alias.Task))
		case evergreen.GithubPRAlias:
			a.Equal("repo_task", utility.FromStringPtr(alias.Task))
		case evergreen.GithubChecksAlias:
			a.Equal("project_config_task", utility.FromStringPtr(alias.Task))
		case evergreen.GitTagAlias:
			a.Equal("added_task", utility.FromStringPtr(alias.Task))
		default:
			a.Equal("project_ref_task", utility.FromStringPtr(alias.Task))
		}
	}
}

func (a *AliasSuite) TestCopyProjectAliases() {
	res, err := FindMergedProjectAliases(a.T().Context(), "new_project_id", "", nil, false)
	a.NoError(err)
	a.Empty(res)

	a.NoError(model.CopyProjectAliases(a.T().Context(), "project_id", "new_project_id"))

	res, err = FindMergedProjectAliases(a.T().Context(), "project_id", "", nil, false)
	a.NoError(err)
	a.Len(res, 4)

	res, err = FindMergedProjectAliases(a.T().Context(), "new_project_id", "", nil, false)
	a.NoError(err)
	a.Len(res, 4)

}

func (a *AliasSuite) TestUpdateProjectAliases() {
	found, err := FindMergedProjectAliases(a.T().Context(), "other_project_id", "", nil, false)
	a.NoError(err)
	a.Require().Len(found, 2)
	toUpdate := found[0]
	toDelete := found[1]
	toUpdate.Alias = utility.ToStringPtr("different_alias")
	toDelete.Delete = true
	aliasUpdates := []restModel.APIProjectAlias{
		toUpdate,
		toDelete,
		{
			Alias:   utility.ToStringPtr("new_alias"),
			Task:    utility.ToStringPtr("new_task"),
			Variant: utility.ToStringPtr("new_variant"),
		},
	}
	a.NoError(UpdateProjectAliases(a.T().Context(), "other_project_id", aliasUpdates))
	found, err = FindMergedProjectAliases(a.T().Context(), "other_project_id", "", nil, false)
	a.NoError(err)
	a.Require().Len(found, 2) // added one alias, deleted another

	a.NotEqual(utility.FromStringPtr(toDelete.ID), found[0].ID)
	a.NotEqual(utility.FromStringPtr(toDelete.ID), found[1].ID)
	a.Equal(utility.FromStringPtr(toUpdate.ID), utility.FromStringPtr(found[0].ID))
	a.Equal("different_alias", utility.FromStringPtr(found[0].Alias))

	a.NotEmpty(found[1].ID)
	a.Equal("new_alias", utility.FromStringPtr(found[1].Alias))
	a.Equal("new_task", utility.FromStringPtr(found[1].Task))
	a.Equal("new_variant", utility.FromStringPtr(found[1].Variant))
}

func (a *AliasSuite) TestUpdateAliasesForSection() {
	originalAliases, err := model.FindAliasesForProjectFromDb(a.T().Context(), "project_id")
	a.NoError(err)
	a.Len(originalAliases, 4)

	// delete one alias, add one alias, modify one alias
	aliasToKeep := restModel.APIProjectAlias{}
	aliasToKeep.BuildFromService(originalAliases[0])
	aliasToModify := restModel.APIProjectAlias{}
	aliasToModify.BuildFromService(originalAliases[1])
	aliasToModify.Alias = utility.ToStringPtr("this is a new alias")

	newAlias := restModel.APIProjectAlias{
		ID:      utility.ToStringPtr(mgobson.NewObjectId().Hex()),
		Alias:   utility.ToStringPtr("patchAlias"),
		Variant: utility.ToStringPtr("var"),
		Task:    utility.ToStringPtr("task"),
	}
	newInternalAlias := restModel.APIProjectAlias{
		ID:      utility.ToStringPtr(mgobson.NewObjectId().Hex()),
		Alias:   utility.ToStringPtr(evergreen.GithubChecksAlias), //internal alias shouldn't be added
		Variant: utility.ToStringPtr("var"),
		Task:    utility.ToStringPtr("task"),
	}

	updatedAliases := []restModel.APIProjectAlias{aliasToKeep, aliasToModify, newAlias, newInternalAlias}
	modified, err := updateAliasesForSection(a.T().Context(), "project_id", updatedAliases, originalAliases, model.ProjectPagePatchAliasSection)
	a.NoError(err)
	a.True(modified)

	aliasesFromDb, err := model.FindAliasesForProjectFromDb(a.T().Context(), "project_id")
	a.NoError(err)
	a.Len(aliasesFromDb, 4)
	for _, alias := range aliasesFromDb {
		a.NotEqual(alias.ID, originalAliases[2].ID)          // removed the alias that we didn't add to the new alias list
		a.NotEqual(evergreen.GithubChecksAlias, alias.Alias) // didn't add the internal alias on the patch alias section
		if alias.ID == originalAliases[1].ID {               // verify we modified the second alias
			a.Equal("this is a new alias", alias.Alias)
		}
	}

	modified, err = updateAliasesForSection(a.T().Context(), "project_id", updatedAliases, originalAliases, model.ProjectPageGithubAndCQSection)
	a.NoError(err)
	a.True(modified)
	aliasesFromDb, err = model.FindAliasesForProjectFromDb(a.T().Context(), "project_id")
	a.NoError(err)
	a.Len(aliasesFromDb, 4) // adds internal alias
}

func TestValidateFeaturesHaveAliases(t *testing.T) {
	assert.NoError(t, db.ClearCollections(model.ProjectAliasCollection))

	oldPRef := &model.ProjectRef{
		VersionControlEnabled: utility.FalsePtr(),
	}

	pRef := &model.ProjectRef{
		Id:                  "p1",
		PRTestingEnabled:    utility.TruePtr(),
		GithubChecksEnabled: utility.TruePtr(),
	}

	aliases := []restModel.APIProjectAlias{
		{
			Alias: utility.ToStringPtr(evergreen.GithubPRAlias),
		},
	}

	// Errors when there aren't aliases for all enabled features.
	err := validateFeaturesHaveAliases(t.Context(), oldPRef, pRef, aliases)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GitHub checks")

	pRef.RepoRefId = "r1"
	repoAlias1 := model.ProjectAlias{
		ProjectID: pRef.RepoRefId,
		Alias:     evergreen.GithubChecksAlias,
	}
	assert.NoError(t, repoAlias1.Upsert(t.Context()))
	// No error when there are aliases in the repo.
	assert.NoError(t, validateFeaturesHaveAliases(t.Context(), oldPRef, pRef, aliases))

	pRef.GitTagVersionsEnabled = utility.TruePtr()
	pRef.CommitQueue.Enabled = utility.TruePtr()
	err = validateFeaturesHaveAliases(t.Context(), oldPRef, pRef, aliases)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Git tag")
	assert.Contains(t, err.Error(), "Commit queue")

	// No error when version control is enabled.
	oldPRef.VersionControlEnabled = utility.TruePtr()
	assert.NoError(t, validateFeaturesHaveAliases(t.Context(), oldPRef, pRef, aliases))
}
