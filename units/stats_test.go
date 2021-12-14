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
	s.Require().NoError(s.env.Configure(ctx))
}

func (s *StatUnitsSuite) TestHostStatsCollector() {
	factory, err := registry.GetJobFactory(hostStatsCollectorJobName)
	s.NoError(err)
	s.NotNil(factory)
	s.NotNil(factory())
	s.Equal(factory().Type().Name, hostStatsCollectorJobName)

	j, ok := NewHostStatsCollector("id").(*hostStatsCollector)
	j.logger = logging.MakeGrip(s.sender)
	s.True(ok)
	s.False(s.sender.HasMessage())
	j.Run(context.Background())
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

	j, ok := NewTaskStatsCollector("id").(*taskStatsCollector)
	j.logger = logging.MakeGrip(s.sender)
	s.True(ok)
	s.False(s.sender.HasMessage())
	s.False(j.Status().Completed)
	j.Run(context.Background())
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

	j, ok := NewSysInfoStatsCollector("id").(*sysInfoStatsCollector)
	s.True(ok)
	j.logger = logging.MakeGrip(s.sender)

	s.False(s.sender.HasMessage())
	s.False(j.Status().Completed)
	j.Run(context.Background())
	s.True(j.Status().Completed)
	s.False(j.HasErrors())
	s.True(s.sender.HasMessage())

	m1, ok1 := s.sender.GetMessageSafe()
	if s.True(ok1) {
		s.True(m1.Logged)
		s.True(strings.Contains(m1.Message.String(), "cpu"), m1.Message.String())
	}

	m2, ok2 := s.sender.GetMessageSafe()
	if s.True(ok2) {
		s.True(m2.Logged)
		s.True(strings.Contains(m2.Message.String(), "cgo.calls"), m2.Message.String())
	}
}
