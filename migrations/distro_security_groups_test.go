package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type securityGroupSuite struct {
	env       *mock.Environment
	session   db.Session
	database  string
	generator anser.Generator
	suite.Suite
}

type distro struct {
	ID       string           `bson:"_id"`
	Settings providerSettings `bson:"settings"`
}

func TestSecurityGroupMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := db.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &securityGroupSuite{
		env:      &mock.Environment{},
		session:  session,
		database: database.Name,
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *securityGroupSuite) SetupSuite() {
	var err error
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 10,
		id:    "foo",
	}

	s.generator, err = distroSecurityGroupsGenerator(anser.GetEnvironment(), args)
	s.NoError(err)
}

func (s *securityGroupSuite) SetupTest() {
	s.NoError(evgdb.ClearCollections(distroCollection))

	d1 := db.Document{
		"_id": "d1",
		settingsKey: db.Document{
			securityGroupKey: "sg-1",
		},
	}
	s.NoError(evgdb.Insert(distroCollection, d1))
	d2 := db.Document{
		"_id": "d2",
		settingsKey: db.Document{
			securityGroupsKey: []string{"sg-2"},
		},
	}
	s.NoError(evgdb.Insert(distroCollection, d2))
}

func (s *securityGroupSuite) TestMigration() {
	s.generator.Run(context.Background())
	s.NoError(s.generator.Error())

	for j := range s.generator.Jobs() {
		j.Run(context.Background())
		s.NoError(j.Error())
	}

	var distroVal distro
	migratedQuery := evgdb.Query(db.Document{
		"_id": "d1",
	})
	s.NoError(evgdb.FindOneQ(distroCollection, migratedQuery, &distroVal))
	s.Equal("", distroVal.Settings.SecurityGroup)
	s.Equal([]string{"sg-1"}, distroVal.Settings.SecurityGroups)

	noopQuery := evgdb.Query(db.Document{
		"_id": "d2",
	})
	s.NoError(evgdb.FindOneQ(distroCollection, noopQuery, &distroVal))
	s.Equal("", distroVal.Settings.SecurityGroup)
	s.Equal([]string{"sg-2"}, distroVal.Settings.SecurityGroups)
}
