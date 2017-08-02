package proto

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type MetricsTestSuite struct {
	suite.Suite
	comm      *client.Mock
	collector metricsCollector
	id        string
}

func TestMetricsTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

func (s *MetricsTestSuite) SetupTest() {
	s.comm = client.NewMock("")
	s.id = "test_task_id"
	s.collector = metricsCollector{
		comm:     s.comm,
		taskData: client.TaskData{ID: s.id},
	}
}

func (s *MetricsTestSuite) TestRunForIntervalAndSendMessages() {
	s.Zero(len(s.comm.ProcInfo[s.id]))
	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	cancel()

	firstLen := len(s.comm.ProcInfo[s.id])
	if runtime.GOOS == "windows" {
		s.True(firstLen >= 1)
	} else {
		s.True(firstLen >= 2)
	}
	// after stopping it shouldn't continue to collect stats
	time.Sleep(time.Second)
	s.Equal(firstLen, len(s.comm.ProcInfo[s.id]))

	for _, post := range s.comm.ProcInfo[s.id] {
		s.Len(post, 1)
	}
}

func (s *MetricsTestSuite) TestCollectSubProcesses() {
	s.Zero(len(s.comm.ProcInfo[s.id]))
	cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
	s.NoError(cmd.Start())

	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	cancel()

	s.NoError(cmd.Process.Kill())

	if runtime.GOOS == "windows" {
		s.Equal(len(s.comm.ProcInfo[s.id]), 1)
	} else {
		s.Equal(len(s.comm.ProcInfo[s.id]), 2)
	}
}

func (s *MetricsTestSuite) TestPersistSystemStats() {
	ctx, cancel := context.WithCancel(context.Background())

	s.Nil(s.comm.SysInfo[s.id])

	go s.collector.sysInfoCollector(ctx, 750*time.Millisecond)
	time.Sleep(time.Second)
	cancel()

	s.True(len(s.comm.SysInfo) >= 1)
	time.Sleep(time.Second)
	s.True(len(s.comm.SysInfo) >= 1)
}
