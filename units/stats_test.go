package units

import (
	"context"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type StatUnitsSuite struct {
	sender *send.InternalSender
	env    *mock.Environment
	cancel context.CancelFunc
	suite.Suite
}

func TestStatUnitsSuite(t *testing.T) {
	suite.Run(t, new(StatUnitsSuite))
}

func (s *StatUnitsSuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.env = &mock.Environment{}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Require().NoError(s.env.Configure(ctx, ""))
}

func (s *StatUnitsSuite) TestAmboyStatsCollector() {
	factory, err := registry.GetJobFactory(amboyStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, amboyStatsCollectorJobName)

	// if the env isn't established it should run and not panic,
	// but return an error.
	j := makeAmboyStatsCollector()
	j.env = nil
	j.logger = logging.MakeGrip(s.sender)
	s.False(j.Status().Completed)
	s.NotPanics(func() { j.Run() })
	s.True(j.Status().Completed)
	s.True(j.HasErrors())

	// if the env is set, but the queues aren't logged it should
	// run and complete but report an error.
	j = makeAmboyStatsCollector()
	s.False(j.Status().Completed)
	j.env = s.env
	s.Equal(j, NewAmboyStatsCollector(s.env)) // validate the public constructor
	j.logger = logging.MakeGrip(s.sender)
	s.False(s.sender.HasMessage())

	j.Run()
	s.False(s.sender.HasMessage())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())

	// When we run with started queues, it should log both
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.NoError(s.env.Local.Start(ctx))
	s.NoError(s.env.Remote.Start(ctx))

	j = makeAmboyStatsCollector()
	s.False(j.Status().Completed)
	j.env = s.env
	j.logger = logging.MakeGrip(s.sender)
	s.False(s.sender.HasMessage())

	j.Run()
	s.True(s.sender.HasMessage())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.True(m1.Logged)
		s.True(strings.Contains(m1.Message.String(), "local queue stats"), m1.Message)
	}

	m2, ok2 := s.sender.GetMessageSafe()
	if s.True(ok2) {
		s.True(m2.Logged)
		s.True(strings.Contains(m2.Message.String(), "remote queue stats"), m2.Message)
	}
}

func (s *StatUnitsSuite) TestHostStatsCollector() {
	factory, err := registry.GetJobFactory(hostStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, hostStatsCollectorJobName)

	j, ok := NewHostStatsCollector().(*hostStatsCollector)
	s.True(ok)
	j.logger = logging.MakeGrip(s.sender)
	s.False(s.sender.HasMessage())
	j.Run()
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.True(m1.Logged)
		s.True(strings.Contains(m1.Message.String(), "host stats by distro"), m1.Message)
	}

	// NOTE: you can't trigger the error case given that the db
	// method that the job calls use the global session factory
}

func (s *StatUnitsSuite) TestTaskStatsCollector() {
	factory, err := registry.GetJobFactory(taskStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, taskStatsCollectorJobName)

	j, ok := NewTaskStatsCollector().(*taskStatsCollector)
	s.True(ok)
	j.logger = logging.MakeGrip(s.sender)
	s.False(s.sender.HasMessage())
	s.False(j.Status().Completed)
	j.Run()
	s.True(j.Status().Completed)
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.False(m1.Logged, "%+v", m1)
		s.Equal(0, m1.Message.(*task.ResultCounts).Total)
	}

	// NOTE: you can't trigger the error case given that the db
	// method that the job calls use the global session factory
}

func (s *StatUnitsSuite) TestSysInfoCollector() {
	factory, err := registry.GetJobFactory(sysInfoStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, sysInfoStatsCollectorJobName)

	j, ok := NewSysInfoStatsCollector().(*sysInfoStatsCollector)
	s.True(ok)
	j.logger = logging.MakeGrip(s.sender)

	s.False(s.sender.HasMessage())
	s.False(j.Status().Completed)
	j.Run()
	s.True(j.Status().Completed)
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.True(m1.Logged)
		s.True(strings.Contains(m1.Message.String(), "cpu-total"), m1.Message.String())
	}

	m2, ok2 := s.sender.GetMessageSafe()
	if s.True(ok2) {
		s.True(m2.Logged)
		s.True(strings.Contains(m2.Message.String(), "cgo.calls"), m2.Message.String())
	}
}
