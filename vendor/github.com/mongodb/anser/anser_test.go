package anser

import (
	"context"
	"testing"

	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/mock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type ApplicationSuite struct {
	app *Application
	env *mock.Environment
	suite.Suite
}

func TestApplicationSuite(t *testing.T) {
	suite.Run(t, new(ApplicationSuite))
}

func (s *ApplicationSuite) SetupSuite()    {}
func (s *ApplicationSuite) TearDownSuite() {}
func (s *ApplicationSuite) SetupTest() {
	s.app = &Application{}
	s.env = &mock.Environment{}
}
func (s *ApplicationSuite) TearDownTest() {}

func (s *ApplicationSuite) TestSetupErrorsWhenHasSetupRuns() {
	s.NoError(s.app.Setup(s.env))

	for i := 0; i < 10; i++ {
		s.Error(s.app.Setup(s.env))
	}
}

func (s *ApplicationSuite) TestApplicationSetupInternals() {
	s.app.hasSetup = true
	s.Error(s.app.Setup(s.env))
	s.app.hasSetup = false

	s.Error(s.app.Setup(nil))
}

func (s *ApplicationSuite) TestStupCanGetNetwork() {
	s.env.NetworkError = errors.New("problem")

	err := s.app.Setup(s.env)
	s.Error(s.app.Setup(s.env))

	s.Equal(errors.Cause(err), s.env.NetworkError)
}

func (s *ApplicationSuite) TestRunMethodErrorsIfQueueHasError() {
	s.env.QueueError = errors.New("problem")
	ctx := context.Background()

	s.NoError(s.app.Setup(s.env))

	err := s.app.Run(ctx)
	s.Error(err)
	s.Equal(errors.Cause(err), s.env.QueueError)
}

func (s *ApplicationSuite) TestRunErrorsWithCanceledContext() {
	s.NoError(s.app.Setup(s.env))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.app.Run(ctx)
	s.Error(err)
	s.Equal(err.Error(), "migration operation canceled")
}

func (s *ApplicationSuite) TestRunDoesNotErrorWithDryRun() {
	s.env.Queue = queue.NewLocalUnordered(2)
	s.NoError(s.app.Setup(s.env))

	ctx := context.Background()
	s.app.DryRun = true
	s.NoError(s.app.Run(ctx))
}
