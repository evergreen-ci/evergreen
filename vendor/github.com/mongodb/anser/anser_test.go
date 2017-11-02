package anser

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/mock"
	"github.com/mongodb/anser/model"
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

func (s *ApplicationSuite) TestLimitIsRespected() {
	opts := model.GeneratorOptions{}
	job := NewManualMigrationGenerator(s.env, opts, "").(*manualMigrationGenerator)

	job.NS = model.Namespace{DB: "foo", Collection: "bar"}
	job.Migrations = []*manualMigrationJob{
		NewManualMigration(s.env, model.Manual{}).(*manualMigrationJob),
		NewManualMigration(s.env, model.Manual{}).(*manualMigrationJob),
		NewManualMigration(s.env, model.Manual{}).(*manualMigrationJob),
	}

	for idx, j := range job.Migrations {
		j.SetID(fmt.Sprintf("job-test-%d", idx))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.env.Queue = queue.NewLocalUnordered(2)
	s.env.Network = mock.NewDependencyNetwork()

	s.NoError(s.env.Queue.Start(ctx))
	s.NoError(s.env.Queue.Put(job))
	s.Equal(1, s.env.Queue.Stats().Total)
	amboy.WaitCtxInterval(ctx, s.env.Queue, 100*time.Millisecond)

	num, err := addMigrationJobs(ctx, s.env.Queue, false, 2)
	s.NoError(err)
	amboy.WaitCtxInterval(ctx, s.env.Queue, 100*time.Millisecond)

	// two is the limit:
	s.Equal(2, num)
	// one generator plus two jobs:
	s.Equal(3, s.env.Queue.Stats().Total)

}
