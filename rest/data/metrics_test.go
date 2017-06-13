package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type MetricsConnectorSuite struct {
	ctx *DBMetricsConnector
	suite.Suite
}

func TestMetricsConnectorSuite(t *testing.T) {
	suite.Run(t, new(MetricsConnectorSuite))
}

func (s *MetricsConnectorSuite) SetupSuite() {
	testutil.ConfigureIntegrationTest(s.T(), testConfig, "TestFindTaskById")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
}

func (s *MetricsConnectorSuite) SetupTest() {
	s.ctx = &DBMetricsConnector{}
	s.Require().NoError(db.Clear(event.TaskLogCollection))
}

func (s *MetricsConnectorSuite) TestSystemsResultsShouldBeEmpty() {
	sys, err := s.ctx.FindTaskSystemMetrics("foo", time.Now(), 100, -1)
	s.NoError(err)
	s.Len(sys, 0)
}

func (s *MetricsConnectorSuite) TestProcessMetricsShouldBeEmpty() {
	procs, err := s.ctx.FindTaskProcessMetrics("foo", time.Now(), 100, -1)
	s.NoError(err)
	s.Len(procs, 0, "%d", len(procs))
}

func (s *MetricsConnectorSuite) TestProcessLimitingFunctionalityConstrainsResults() {
	msgs := []*message.ProcessInfo{}
	for _, m := range message.CollectProcessInfoSelfWithChildren() {
		msgs = append(msgs, m.(*message.ProcessInfo))
	}

	event.LogTaskProcessData("foo", msgs)
	event.LogTaskProcessData("foo", msgs)
	event.LogTaskProcessData("foo", msgs)
	event.LogTaskProcessData("foo", msgs)
	event.LogTaskProcessData("foo", msgs)

	for i := 1; i < 4; i++ {
		procs, err := s.ctx.FindTaskProcessMetrics("foo", time.Now(), i, -1)
		s.NoError(err)

		s.Len(procs, i, "checking limit of %d", i)
	}

	procs, err := s.ctx.FindTaskProcessMetrics("foo", time.Now(), 900, -1)
	s.NoError(err)

	s.Len(procs, 5, "checking unlimited")
}

func (s *MetricsConnectorSuite) TestSystemMetricsLimitConstrainsResults() {
	for i := 0; i < 20; i++ {
		m := message.CollectSystemInfo()
		event.LogTaskSystemData("foo", m.(*message.SystemInfo))
	}

	info, err := s.ctx.FindTaskSystemMetrics("foo", time.Now().Add(-2*time.Hour), 100, 1)
	s.NoError(err)
	s.Len(info, 20)

	for i := 1; i < 15; i++ {
		info, err = s.ctx.FindTaskSystemMetrics("foo", time.Now().Add(-2*time.Hour), i, 1)
		s.NoError(err)
		s.Len(info, i)
	}

	// if limit is 0, then full results
	info, err = s.ctx.FindTaskSystemMetrics("foo", time.Now().Add(-2*time.Hour), 0, 1)
	s.NoError(err)
	s.Len(info, 20)

}

func (s *MetricsConnectorSuite) TestSystemMetricsReturnedInSortedOrder() {
	for i := 0; i < 20; i++ {
		m := message.CollectSystemInfo()
		event.LogTaskSystemData("foo", m.(*message.SystemInfo))
	}

	info, err := s.ctx.FindTaskSystemMetrics("foo", time.Now().Add(-2*time.Hour), 100, -1)
	s.NoError(err)
	s.Len(info, 0)

	info, err = s.ctx.FindTaskSystemMetrics("foo", time.Now().Add(-2*time.Hour), 100, 1)
	s.NoError(err)
	s.Len(info, 20)
}
