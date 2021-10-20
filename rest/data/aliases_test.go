package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type AliasSuite struct {
	sc *DBConnector
	suite.Suite
}

func TestAliasSuite(t *testing.T) {
	suite.Run(t, new(AliasSuite))
}

func (a *AliasSuite) SetupTest() {
	a.sc = &DBConnector{}
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	a.Require().NoError(session.DB(testConfig.Database.DB).DropDatabase(), "Error dropping database")

	aliases := []model.ProjectAlias{
		{
			ProjectID: "project_id",
			Alias:     "foo",
			Variant:   "variant",
			Task:      "task",
		},
		{
			ProjectID: "project_id",
			Alias:     "bar",
			Variant:   "not_this_variant",
			Task:      "not_this_task",
		},
		{
			ProjectID: "project_id",
			Alias:     "foo",
			Variant:   "another_variant",
			Task:      "another_task",
		},
		{
			ProjectID: "other_project_id",
			Alias:     "baz",
			Variant:   "variant",
			Task:      "task",
		},
		{
			ProjectID: "other_project_id",
			Alias:     "delete_me",
			Variant:   "variant",
			Task:      "task",
		},
		{
			ProjectID: "repo_id",
			Alias:     "from_repo",
			Variant:   "repo_variant",
			Task:      "repo_task",
		},
	}
	for _, v := range aliases {
		a.NoError(v.Upsert())
	}
}

func (a *AliasSuite) TestFindProjectAliases() {
	found, err := a.sc.FindProjectAliases("project_id", "", nil)
	a.NoError(err)
	a.Len(found, 3)

	found, err = a.sc.FindProjectAliases("project_id", "repo_id", nil)
	a.NoError(err)
	a.Len(found, 3) // ignore repo

	found, err = a.sc.FindProjectAliases("non-existent", "", nil)
	a.NoError(err)
	a.Len(found, 0)

	found, err = a.sc.FindProjectAliases("non-existent", "repo_id", nil)
	a.NoError(err)
	a.Len(found, 1) // from repo

	found, err = a.sc.FindProjectAliases("", "repo_id", nil)
	a.NoError(err)
	a.Len(found, 1)

	// TODO: add test
}

func (a *AliasSuite) TestCopyProjectAliases() {
	res, err := a.sc.FindProjectAliases("new_project_id", "", nil)
	a.NoError(err)
	a.Len(res, 0)

	a.NoError(a.sc.CopyProjectAliases("project_id", "new_project_id"))

	res, err = a.sc.FindProjectAliases("project_id", "", nil)
	a.NoError(err)
	a.Len(res, 3)

	res, err = a.sc.FindProjectAliases("new_project_id", "", nil)
	a.NoError(err)
	a.Len(res, 3)

}

func (a *AliasSuite) TestUpdateProjectAliases() {
	found, err := a.sc.FindProjectAliases("other_project_id", "", nil)
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
	a.NoError(a.sc.UpdateProjectAliases("other_project_id", aliasUpdates))
	found, err = a.sc.FindProjectAliases("other_project_id", "", nil)
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

func (a *AliasSuite) TestHasMatchingGitTagAliasAndRemotePath() {
	newAlias := model.ProjectAlias{
		ProjectID: "project_id",
		Alias:     evergreen.GitTagAlias,
		GitTag:    "release",
		Variant:   "variant",
		Task:      "task",
	}
	a.NoError(newAlias.Upsert())
	newAlias2 := model.ProjectAlias{
		ProjectID:  "project_id",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "file.yml",
	}
	a.NoError(newAlias2.Upsert())
	hasAliases, path, err := a.sc.HasMatchingGitTagAliasAndRemotePath("project_id", "release")
	a.Error(err)
	a.False(hasAliases)
	a.Empty(path)

	newAlias2.RemotePath = ""
	a.NoError(newAlias2.Upsert())
	hasAliases, path, err = a.sc.HasMatchingGitTagAliasAndRemotePath("project_id", "release")
	a.NoError(err)
	a.True(hasAliases)
	a.Empty(path)

	hasAliases, path, err = a.sc.HasMatchingGitTagAliasAndRemotePath("project_id2", "release")
	a.Error(err)
	a.False(hasAliases)
	a.Empty(path)

	newAlias3 := model.ProjectAlias{
		ProjectID:  "project_id2",
		Alias:      evergreen.GitTagAlias,
		GitTag:     "release",
		RemotePath: "file.yml",
	}
	a.NoError(newAlias3.Upsert())
	hasAliases, path, err = a.sc.HasMatchingGitTagAliasAndRemotePath("project_id2", "release")
	a.NoError(err)
	a.True(hasAliases)
	a.Equal("file.yml", path)
}

func (a *AliasSuite) TestUpdateAliasesForSection() {
	originalAliases, err := model.FindAliasesForProject("project_id")
	a.NoError(err)
	a.Len(originalAliases, 3)

	// delete one alias, add one alias, modify one alias
	aliasToKeep := restModel.APIProjectAlias{}
	a.NoError(aliasToKeep.BuildFromService(originalAliases[0]))
	aliasToModify := restModel.APIProjectAlias{}
	a.NoError(aliasToModify.BuildFromService(originalAliases[1]))
	aliasToModify.Alias = utility.ToStringPtr("this is a new alias")

	newAlias := restModel.APIProjectAlias{
		ID:      utility.ToStringPtr(mgobson.NewObjectId().Hex()),
		Alias:   utility.ToStringPtr("patchAlias"),
		Variant: utility.ToStringPtr("var"),
		Task:    utility.ToStringPtr("task"),
	}
	newInternalAlias := restModel.APIProjectAlias{
		ID:      utility.ToStringPtr(mgobson.NewObjectId().Hex()),
		Alias:   utility.ToStringPtr(evergreen.CommitQueueAlias), //internal alias shouldn't be added
		Variant: utility.ToStringPtr("var"),
		Task:    utility.ToStringPtr("task"),
	}

	updatedAliases := []restModel.APIProjectAlias{aliasToKeep, aliasToModify, newAlias, newInternalAlias}
	modified, err := a.sc.UpdateAliasesForSection("project_id", updatedAliases, originalAliases, model.ProjectPagePatchAliasSection)
	a.NoError(err)
	a.True(modified)

	aliasesFromDb, err := model.FindAliasesForProject("project_id")
	a.NoError(err)
	a.Len(aliasesFromDb, 3)
	for _, alias := range aliasesFromDb {
		a.NotEqual(alias.ID, originalAliases[2].ID)         // removed the alias that we didn't add to the new alias list
		a.NotEqual(alias.Alias, evergreen.CommitQueueAlias) // didn't add the internal alias on the patch alias section
		if alias.ID == originalAliases[1].ID {              // verify we modified the second alias
			a.Equal(alias.Alias, "this is a new alias")
		}
	}

	modified, err = a.sc.UpdateAliasesForSection("project_id", updatedAliases, originalAliases, model.ProjectPageGithubAndCQSection)
	a.NoError(err)
	a.True(modified)
	aliasesFromDb, err = model.FindAliasesForProject("project_id")
	a.NoError(err)
	a.Len(aliasesFromDb, 4) // adds internal alias
}
