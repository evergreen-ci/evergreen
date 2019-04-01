package migrations

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

type preferenceMigrationSuite struct {
	env       *mock.Environment
	session   db.Session
	database  string
	now       time.Time
	generator anser.Generator
	suite.Suite
}

func TestSpawnhostPreferenceMigration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)

	session, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer session.Close()

	s := &preferenceMigrationSuite{
		env:      &mock.Environment{},
		session:  session,
		database: database.Name(),
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *preferenceMigrationSuite) SetupSuite() {
	var err error
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 10,
		id:    "foo",
	}

	s.generator, err = setSpawnhostPreferenceGenerator(anser.GetEnvironment(), args)
	s.NoError(err)
}

func (s *preferenceMigrationSuite) SetupTest() {
	s.NoError(evgdb.ClearCollections(userCollection, subscriptionCollection))
	s.now = time.Now().Round(time.Millisecond).Truncate(time.Millisecond)

	user1 := db.Document{
		"_id":   "user1",
		"email": "user1@something.com",
	}
	s.NoError(evgdb.Insert(userCollection, user1))
	user2 := db.Document{
		"_id":   "user2",
		"email": "user2@something.com",
	}
	s.NoError(evgdb.Insert(userCollection, user2))
	user3 := db.Document{
		"_id": "user3",
	}
	s.NoError(evgdb.Insert(userCollection, user3))
}

func (s *preferenceMigrationSuite) TestMigration() {
	s.generator.Run(context.Background())
	s.NoError(s.generator.Error())

	for j := range s.generator.Jobs() {
		j.Run(context.Background())
		s.NoError(j.Error())
	}

	var users []user
	s.NoError(evgdb.FindAllQ(userCollection, evgdb.Q{}, &users))
	s.Len(users, 3)
	for _, u := range users {
		s.Equal("email", u.Settings.Notifications.SpawnHostExpiration)

		var sub emailSubscription
		subscriptionID := u.Settings.Notifications.SpawnHostExpirationID
		query := evgdb.Query(db.Document{
			"_id": subscriptionID,
		})
		s.NoError(evgdb.FindOneQ(subscriptionCollection, query, &sub))
		s.Equal(u.ID, sub.Owner)
		s.Equal("HOST", sub.Type)
		s.Equal("email", sub.Subscriber.Type)
		s.Equal(u.Email, sub.Subscriber.Target)
		s.Equal("owner", sub.Selectors[0].Type)
		s.Equal(u.ID, sub.Selectors[0].Data)
	}
}

func (s *preferenceMigrationSuite) TestNoChangeExistingData() {
	id := mgobson.NewObjectId()
	user4 := db.Document{
		"_id": "user4",
		"settings": db.Document{
			"notifications": db.Document{
				"spawn_host_expiration_id": id,
			},
		},
	}
	s.NoError(evgdb.Insert(userCollection, user4))

	s.generator.Run(context.Background())
	s.NoError(s.generator.Error())

	for j := range s.generator.Jobs() {
		j.Run(context.Background())
		s.NoError(j.Error())
	}

	var u user
	query := evgdb.Query(db.Document{
		"_id": "user4",
	})
	s.NoError(evgdb.FindOneQ(userCollection, query, &u))
	s.Equal(id, u.Settings.Notifications.SpawnHostExpirationID)
}
