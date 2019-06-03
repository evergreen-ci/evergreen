package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
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
	}
	for _, v := range aliases {
		a.NoError(v.Upsert())
	}
}

func (a *AliasSuite) TestFindProjectAliases() {
	found, err := a.sc.FindProjectAliases("project_id")
	a.Nil(err)
	a.Len(found, 3)

	found, err = a.sc.FindProjectAliases("non-existent")
	a.Nil(err)
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
