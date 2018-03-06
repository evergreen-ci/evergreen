package migrations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	anserdb "github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
)

// migrationSuite is a reusable base for migration tests that performs anser
// setup. Use in place of suite.Suite.
// If your suite needs to define a SetupSuite() method, call
// s.migrationSuite.SetupSuite() from your SetupSuite() method
type migrationSuite struct {
	env      *mock.Environment
	database string
	session  anserdb.Session
	cancel   func()

	suite.Suite
}

func (s *migrationSuite) SetupSuite() {
	require := s.Require()
	var ctx context.Context
	ctx, s.cancel = context.WithCancel(context.Background())

	s.env = &mock.Environment{}
	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	var err error
	mgoSession, database, err := db.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()
	s.database = database.Name

	s.session = anserdb.WrapSession(mgoSession.Copy())

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { s.cancel(); return nil })
}

func (s *migrationSuite) TearDownSuite() {
	s.Require().NotPanics(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.session != nil {
			s.session.Close()
		}
	})
}

func (s *migrationSuite) TestMigrationSuiteHasSaneConditions() {
	const migrationSuiteNotSetupMessage = "migrationSuite has not been setup, please add a call to s.migrationSuite.SetupSuite() to your suite's SetupSuite function"

	require := s.Require()
	require.NotNil(s.env, migrationSuiteNotSetupMessage)
	require.NotEmpty(s.database, migrationSuiteNotSetupMessage)
	require.NotEmpty(s.session, migrationSuiteNotSetupMessage)
	require.NotNil(s.cancel, migrationSuiteNotSetupMessage)
}

func TestMigrationBaseSuite(t *testing.T) {
	suite.Run(t, &migrationSuite{})
}
