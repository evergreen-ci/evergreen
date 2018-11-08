package anser

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	grip.SetName("anser.test")
}

type EnvImplSuite struct {
	env     *envState
	q       amboy.Queue
	session db.Session
	cancel  context.CancelFunc
	suite.Suite
}

func TestEnvImplSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(EnvImplSuite))
}

func (s *EnvImplSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.q = queue.NewLocalUnordered(4)
	s.cancel = cancel
	s.NoError(s.q.Start(ctx))

	mgoses, err := mgo.DialWithTimeout("mongodb://localhost:27017/", 10*time.Millisecond)
	s.Require().NoError(err)
	s.session = db.WrapSession(mgoses)

	s.Require().Equal(globalEnv, GetEnvironment())

	s.env = &envState{
		migrations: make(map[string]db.MigrationOperation),
		processor:  make(map[string]db.Processor),
	}

	s.Nil(s.env.session)
	s.False(s.env.isSetup)
	s.NoError(s.env.Setup(s.q, s.session))
	s.True(s.env.isSetup)
	s.NotNil(s.env.session)
	s.Equal(s.env.metadataNS.DB, defaultAnserDB)
	s.Equal(s.env.metadataNS.Collection, defaultMetadataCollection)
	s.Equal(s.env.MetadataNamespace(), s.env.metadataNS)
}

func (s *EnvImplSuite) TearDownTest() {
	s.cancel()
}

func (s *EnvImplSuite) TestCallingSetupMultipleTimesErrors() {
	s.Error(s.env.Setup(s.q, s.session))
	s.True(s.env.isSetup)
}

func (s *EnvImplSuite) TestDialErrorCausesSetupError() {
	s.env.isSetup = false
	s.env.session = nil
	s.Error(s.env.Setup(s.q, nil))
	s.False(s.env.isSetup)
	s.Nil(s.env.session)
}

func (s *EnvImplSuite) TestUnstartedQueueCausesError() {
	s.env.isSetup = false
	s.env.queue = nil

	s.Error(s.env.Setup(queue.NewLocalUnordered(2), s.session))
	s.Nil(s.env.queue)
	s.False(s.env.isSetup)
}

func (s *EnvImplSuite) TestDatabaseNameOverrideFromURI() {
	s.env.isSetup = false
	mgoses, err := mgo.DialWithTimeout("mongodb://localhost:27017/mci", 10*time.Millisecond)
	s.Require().NoError(err)
	session := db.WrapSession(mgoses)
	defer session.Close()

	s.NoError(s.env.Setup(s.q, session))
	s.True(s.env.isSetup)
	s.Equal("mci", s.env.metadataNS.DB)
}

func (s *EnvImplSuite) TestSessionAccessor() {
	session, err := s.env.GetSession()
	s.NoError(err)
	s.NotNil(session)
}

func (s *EnvImplSuite) TestQueueAccessor() {
	queue, err := s.env.GetQueue()
	s.NoError(err)
	s.Equal(s.q, queue)
}

func (s *EnvImplSuite) TestDepNetworkAccessor() {
	network, err := s.env.GetDependencyNetwork()
	s.NoError(err)
	s.NotNil(network)
	s.NotEqual(globalEnv.deps, network)
	s.Equal(s.env.deps, network)
}

func (s *EnvImplSuite) TestManualMigrationOperationRegistry() {
	count := 0

	op := func(_ db.Session, _ bson.RawD) error { count++; return nil }
	s.Len(s.env.migrations, 0)
	s.NoError(s.env.RegisterManualMigrationOperation("foo", op))
	s.Len(s.env.migrations, 1)
	s.Error(s.env.RegisterManualMigrationOperation("foo", op))
	s.Len(s.env.migrations, 1)

	fun, ok := s.env.GetManualMigrationOperation("foo")
	s.True(ok)
	s.Equal(0, count)
	fun(nil, bson.RawD{})
	s.Equal(1, count)

	fun, ok = s.env.GetManualMigrationOperation("bar")
	s.False(ok)
	s.Zero(fun)
}

func (s *EnvImplSuite) TestDocumentProcessor() {
	s.Len(s.env.processor, 0)
	s.NoError(s.env.RegisterDocumentProcessor("foo", nil))
	s.Len(s.env.processor, 1)
	s.Error(s.env.RegisterDocumentProcessor("foo", nil))
	s.Len(s.env.processor, 1)

	dp, ok := s.env.GetDocumentProcessor("foo")
	s.True(ok)
	s.Nil(dp)

	dp, ok = s.env.GetDocumentProcessor("bar")
	s.False(ok)
	s.Nil(dp)
}

func (s *EnvImplSuite) TestDependencyNetworkConstructor() {
	dep := s.env.NewDependencyManager("foo")

	s.NotNil(dep)
	mdep := dep.(*migrationDependency)
	s.Equal(mdep.Env(), s.env)
	s.Equal(mdep.MigrationID, "foo")
}

func (s *EnvImplSuite) TestRegisterCloser() {
	s.Len(s.env.closers, 1)
	s.env.RegisterCloser(nil)
	s.Len(s.env.closers, 1)
	s.env.RegisterCloser(func() error { return nil })
	s.Len(s.env.closers, 2)

	s.env.RegisterCloser(func() error { return nil })
	s.Len(s.env.closers, 3)

	s.NoError(s.env.Close())

	s.env.RegisterCloser(func() error { return errors.New("foo") })
	s.Len(s.env.closers, 4)

	s.Error(s.env.Close())
}

func (s *EnvImplSuite) TestCloseEncountersError() {
	s.Len(s.env.closers, 1)
	s.env.RegisterCloser(func() error { return errors.New("foo") })
	s.Len(s.env.closers, 2)

	s.Error(s.env.Close())
}

func TestUninitializedEnvErrors(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	env := &envState{}

	session, err := env.GetSession()
	assert.Nil(session)
	assert.Error(err)

	queue, err := env.GetQueue()
	assert.Nil(queue)
	assert.Error(err)

	net, err := env.GetDependencyNetwork()
	assert.Nil(net)
	assert.Error(err)
}
