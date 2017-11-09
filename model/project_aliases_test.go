package model

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type ProjectAliasSuite struct {
	suite.Suite
	aliases []ProjectAlias
}

func TestProjectAliasSuite(t *testing.T) {
	s := &ProjectAliasSuite{}
	suite.Run(t, s)
}

func (s *ProjectAliasSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

	for i := 0; i < 10; i++ {
		s.aliases = append(s.aliases, ProjectAlias{
			ProjectID: fmt.Sprintf("project-%d", i),
			Alias:     fmt.Sprintf("alias-%d", i),
			Variants:  fmt.Sprintf("variant-%d", i),
			Tasks:     fmt.Sprintf("task-%d", i),
		})
	}
}

func (s *ProjectAliasSuite) SetupTest() {
	s.Require().NoError(db.Clear(ProjectAliasCollection))
}

func (s *ProjectAliasSuite) TestInsert() {
	for _, a := range s.aliases {
		s.NoError(a.Insert())
	}

	var out ProjectAlias
	for i, a := range s.aliases {
		q := db.Query(bson.M{ProjectIDKey: fmt.Sprintf("project-%d", i)})
		s.NoError(db.FindOneQ(ProjectAliasCollection, q, &out))
		s.Equal(a.ProjectID, out.ProjectID)
		s.Equal(a.Alias, out.Alias)
		s.Equal(a.Variants, out.Variants)
		s.Equal(a.Tasks, out.Tasks)
	}
}

func (s *ProjectAliasSuite) TestRemove() {
	for _, a := range s.aliases {
		s.NoError(a.Insert())
	}
	var out []ProjectAlias
	q := db.Query(bson.M{})
	s.NoError(db.FindAllQ(ProjectAliasCollection, q, &out))
	s.Len(out, 10)

	for i, a := range s.aliases {
		s.NoError(a.Remove())
		s.NoError(db.FindAllQ(ProjectAliasCollection, q, &out))
		s.Len(out, 10-i-1)
	}
}

func (s *ProjectAliasSuite) TestFindProjectAliases() {
	for _, a := range s.aliases {
		s.NoError(a.Insert())
	}
	a1 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-1",
		Variants:  "variants-11",
		Tasks:     "variants-11",
	}
	a2 := ProjectAlias{
		ProjectID: "project-1",
		Alias:     "alias-2",
		Variants:  "variants-11",
		Tasks:     "variants-11",
	}
	a3 := ProjectAlias{
		ProjectID: "project-2",
		Alias:     "alias-1",
		Variants:  "variants-11",
		Tasks:     "variants-11",
	}
	s.NoError(a1.Insert())
	s.NoError(a2.Insert())
	s.NoError(a3.Insert())

	found, err := FindProjectAliases("project-1", "alias-1")
	s.NoError(err)
	s.Len(found, 2)
}
