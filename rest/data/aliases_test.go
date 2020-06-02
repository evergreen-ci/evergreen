package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
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
	}
	for _, v := range aliases {
		a.NoError(v.Upsert())
	}
}

func (a *AliasSuite) TestFindProjectAliases() {
	found, err := a.sc.FindProjectAliases("project_id")
	a.NoError(err)
	a.Len(found, 3)

	found, err = a.sc.FindProjectAliases("non-existent")
	a.NoError(err)
	a.Len(found, 0)
}

func (a *AliasSuite) TestCopyProjectAliases() {
	res, err := a.sc.FindProjectAliases("new_project_id")
	a.NoError(err)
	a.Len(res, 0)

	a.NoError(a.sc.CopyProjectAliases("project_id", "new_project_id"))

	res, err = a.sc.FindProjectAliases("project_id")
	a.NoError(err)
	a.Len(res, 3)

	res, err = a.sc.FindProjectAliases("new_project_id")
	a.NoError(err)
	a.Len(res, 3)

}

func (a *AliasSuite) TestUpdateProjectAliases() {
	found, err := a.sc.FindProjectAliases("other_project_id")
	a.NoError(err)
	a.Require().Len(found, 2)
	toUpdate := found[0]
	toDelete := found[1]
	toUpdate.Alias = restModel.ToStringPtr("different_alias")
	toDelete.Delete = true
	aliasUpdates := []restModel.APIProjectAlias{
		toUpdate,
		toDelete,
		{
			Alias:   restModel.ToStringPtr("new_alias"),
			Task:    restModel.ToStringPtr("new_task"),
			Variant: restModel.ToStringPtr("new_variant"),
		},
	}
	a.NoError(a.sc.UpdateProjectAliases("other_project_id", aliasUpdates))
	found, err = a.sc.FindProjectAliases("other_project_id")
	a.NoError(err)
	a.Require().Len(found, 2) // added one alias, deleted another

	a.NotEqual(restModel.FromStringPtr(toDelete.ID), found[0].ID)
	a.NotEqual(restModel.FromStringPtr(toDelete.ID), found[1].ID)
	a.Equal(restModel.FromStringPtr(toUpdate.ID), restModel.FromStringPtr(found[0].ID))
	a.Equal("different_alias", restModel.FromStringPtr(found[0].Alias))

	a.NotEmpty(found[1].ID)
	a.Equal("new_alias", restModel.FromStringPtr(found[1].Alias))
	a.Equal("new_task", restModel.FromStringPtr(found[1].Task))
	a.Equal("new_variant", restModel.FromStringPtr(found[1].Variant))
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
	a.NoError(err)
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
