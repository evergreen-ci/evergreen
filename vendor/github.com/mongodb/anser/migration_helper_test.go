package anser

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgo "gopkg.in/mgo.v2"
)

type MigrationHelperSuite struct {
	env          *mock.Environment
	mh           *migrationBase
	session      db.Session
	client       client.Client
	queue        amboy.Queue
	cancel       context.CancelFunc
	preferClient bool
	suite.Suite
}

func TestLegacyMigrationHelperSuite(t *testing.T) {
	s := new(MigrationHelperSuite)
	suite.Run(t, s)
}

func TestClientMigrationHelperSuite(t *testing.T) {
	s := new(MigrationHelperSuite)
	s.preferClient = true
	suite.Run(t, s)
}

func (s *MigrationHelperSuite) SetupSuite() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.queue = queue.NewLocalLimitedSize(4, 256)
	s.NoError(s.queue.Start(ctx))

	ses, err := mgo.DialWithTimeout("mongodb://localhost:27017", 10*time.Millisecond)
	s.Require().NoError(err)
	s.session = db.WrapSession(ses)
	cl, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(100 * time.Millisecond))
	s.Require().NoError(err)
	err = cl.Connect(ctx)
	s.Require().NoError(err)
	s.client = client.WrapClient(cl)
}

func (s *MigrationHelperSuite) TearDownSuite() {
	s.cancel()
}

func (s *MigrationHelperSuite) SetupTest() {
	s.env = mock.NewEnvironment()
	s.env.MetaNS = model.Namespace{DB: "anserDB", Collection: "anserMeta"}
	s.env.Queue = s.queue
	s.env.ShouldPreferClient = s.preferClient
	s.mh = NewMigrationHelper(s.env).(*migrationBase)

	s.NoError(s.env.Setup(s.queue, s.client, s.session))
}

func (s *MigrationHelperSuite) TestEnvironmentIsConsistent() {
	s.Equal(s.mh.Env(), s.env)
	s.NotEqual(s.mh.Env(), globalEnv)
}

func (s *MigrationHelperSuite) TestSaveMigrationEvent() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.mh.SaveMigrationEvent(ctx, &model.MigrationMetadata{ID: "foo"})
	s.Error(err)
}

func (s *MigrationHelperSuite) TestFinishMigrationIsTracked() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	base := &job.Base{}

	status := base.Status()
	s.False(status.Completed)

	s.mh.FinishMigration(ctx, "foo", base)

	status = base.Status()
	s.True(status.Completed)
}

func (s *MigrationHelperSuite) TestGetMigrationEvents() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	query := map[string]interface{}{"foo": 1}

	iter := s.mh.GetMigrationEvents(ctx, query)
	s.Require().NotNil(iter)

	err := iter.Err()
	s.NoError(err)
}

func (s *MigrationHelperSuite) TestErrorCaseInMigrationFinishing() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := mock.NewEnvironment()
	ns := model.Namespace{DB: "dbname", Collection: "collname"}
	env.MetaNS = ns
	env.Session.DB(ns.DB).C(ns.Collection).(*mock.LegacyCollection).FailWrites = true

	mh := NewMigrationHelper(env).(*migrationBase)

	base := &job.Base{TaskID: "jobid"}
	s.False(base.HasErrors())
	s.False(base.Status().Completed)
	mh.FinishMigration(ctx, "foo", base)
	s.True(base.Status().Completed)
	s.True(base.HasErrors())
}

func (s *MigrationHelperSuite) TestPendingMigrationsWithoutConfiguration() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Zero(s.mh.PendingMigrationOperations(ctx, model.Namespace{DB: "dbname", Collection: "collname"}, map[string]interface{}{}))
}

func TestDefaultEnvironmentAndMigrationHelperState(t *testing.T) {
	assert := assert.New(t)
	env := &envState{}
	mh := NewMigrationHelper(env).(*migrationBase)
	assert.Equal(env, mh.Env())
	assert.Equal(env, mh.env)

	assert.Equal(globalEnv, GetEnvironment())

	mh.env = nil
	assert.Equal(globalEnv, mh.Env())
	assert.NotEqual(mh.Env(), env)
}

func TestErrorIterator(t *testing.T) {
	iter := &errorMigrationIterator{}

	assert.NoError(t, iter.Err())
	assert.NoError(t, iter.Close())
	assert.False(t, iter.Next(context.Background()))
	assert.Nil(t, iter.Item())

	iter.err = errors.New("error")

	assert.Error(t, iter.Err())
	assert.NoError(t, iter.Close())
	assert.False(t, iter.Next(context.Background()))
	assert.Nil(t, iter.Item())
}
