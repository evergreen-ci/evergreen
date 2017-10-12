package agent

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
)

type MetricsSuite struct {
	suite.Suite
	comm      *client.Mock
	collector *metricsCollector
	id        string
}

func TestMetricsSuite(t *testing.T) {
	suite.Run(t, new(MetricsSuite))
}

func (s *MetricsSuite) SetupTest() {
	s.comm = client.NewMock("")
	s.id = "test_task_id"
	s.collector = &metricsCollector{
		comm:     s.comm,
		taskData: client.TaskData{ID: s.id},
	}
}

func (s *MetricsSuite) TestRunForIntervalAndSendMessages() {
	if runtime.GOOS == "windows" {
		s.T().Skip("skipping on windows")
	}
	s.Zero(s.comm.GetProcessInfoLength(s.id))
	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	cancel()

	firstLen := s.comm.GetProcessInfoLength(s.id)
	s.True(firstLen >= 1)

	// after stopping it shouldn't continue to collect stats
	time.Sleep(time.Second)

	s.Equal(firstLen, s.comm.GetProcessInfoLength(s.id))
}

func (s *MetricsSuite) TestCollectSubProcesses() {
	if runtime.GOOS == "windows" {
		s.T().Skip("skipping on windows")
	}
	s.Zero(s.comm.GetProcessInfoLength(s.id))
	cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
	s.NoError(cmd.Start())

	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	cancel()

	s.NoError(cmd.Process.Kill())

	s.True(s.comm.GetProcessInfoLength(s.id) >= 2)
}

func (s *MetricsSuite) TestPersistSystemStats() {
	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.sysInfoCollector(ctx, 750*time.Millisecond)

	if runtime.GOOS == "windows" {
		time.Sleep(5 * time.Second)
	} else {
		time.Sleep(time.Second)
	}
	cancel()

	s.True(s.comm.GetSystemInfoLength() >= 1)

	time.Sleep(time.Second)

	s.True(s.comm.GetSystemInfoLength() >= 1)
}
