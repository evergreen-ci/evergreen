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
	if runtime.GOOS == "windows" {
		t.Skip("skipping metrics tests on windows")
	}

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
	s.Zero(s.comm.GetProcessInfoLength(s.id))
	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond)
	time.Sleep(time.Second)
	cancel()

	// since the process metrics collector takes some time to collect metrics, only fail if
	// the assert doesn't pass for 20 secs
	pass := false
	var firstLen int
	for i := 0; i < 20; i++ {
		firstLen = s.comm.GetProcessInfoLength(s.id)
		if firstLen >= 1 {
			pass = true
			break
		}
		time.Sleep(time.Second)
	}
	if !pass {
		s.Fail("TestRunForIntervalAndSendMessages: incorrect initial process info length")
	}

	// after stopping it shouldn't continue to collect stats
	time.Sleep(time.Second)

	pass = false
	for i := 0; i < 20; i++ {
		temp := s.comm.GetProcessInfoLength(s.id)
		if temp == firstLen {
			pass = true
			break
		}
		time.Sleep(time.Second)
	}
	if !pass {
		s.Fail("TestRunForIntervalAndSendMessages: incorrect process info length after stopping")
	}
}

func (s *MetricsSuite) TestCollectSubProcesses() {
	s.Zero(s.comm.GetProcessInfoLength(s.id))
	cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
	s.NoError(cmd.Start())

	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond)
	time.Sleep(time.Second)
	cancel()

	s.NoError(cmd.Process.Kill())

	pass := false
	for i := 0; i < 20; i++ {
		temp := s.comm.GetProcessInfoLength(s.id)
		if temp >= 2 {
			pass = true
			break
		}
		time.Sleep(time.Second)
	}
	if !pass {
		s.Fail("TestCollectSubProcesses: incorrect process info length")
	}
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
