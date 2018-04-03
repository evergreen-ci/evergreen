package migrations

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type setDefaultBranchMigrationSuite struct {
	refs []anserdb.Document
	migrationSuite
}

func TestSetDefaultBranchMigration(t *testing.T) {
	suite.Run(t, &setDefaultBranchMigrationSuite{})
}

func (s *setDefaultBranchMigrationSuite) SetupTest() {
	const projectRefCollection = "project_ref"

	c, err := s.session.DB(s.database).C(projectRefCollection).RemoveAll(anserdb.Document{})
	s.Require().NoError(err)
	s.Require().NotNil(c)

	s.refs = []anserdb.Document{
		{
			"identifier":  "1",
			"branch_name": "",
		},
		{
			"identifier":  "2",
			"branch_name": "something",
		},
	}
	for _, e := range s.refs {
		s.NoError(db.Insert(projectRefCollection, e))
	}
}

func (s *setDefaultBranchMigrationSuite) TestMigration() {
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    "migration-" + migrationSetDefaultBranch,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gen, err := setDefaultBranchMigrationGenerator(anser.GetEnvironment(), args)
	s.Require().NoError(err)
	gen.Run(ctx)
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run(ctx)
		s.NoError(j.Error())
	}
	s.Equal(1, i)

	out := []bson.M{}
	s.Require().NoError(db.FindAllQ("project_ref", db.Q{}, &out))
	s.Len(out, 2)

	for _, e := range out {
		if e["identifier"] == "1" {
			s.Equal("master", e["branch_name"])

		} else if e["identifier"] == "2" {
			s.Equal("something", e["branch_name"])

		} else {
			s.T().Errorf("unknown project ref")
		}
	}
}
