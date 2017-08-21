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
	suite.Run(t, new(MetricsTestSuite))
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
	s.comm.Mu.Lock()
	s.Zero(len(s.comm.ProcInfo[s.id]))
	s.comm.Mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	cancel()

	s.comm.Mu.Lock()
	firstLen := len(s.comm.ProcInfo[s.id])
	s.comm.Mu.Unlock()
	s.True(firstLen >= 1)

	// after stopping it shouldn't continue to collect stats
	time.Sleep(time.Second)

	s.comm.Mu.Lock()
	s.Equal(firstLen, len(s.comm.ProcInfo[s.id]))
	s.comm.Mu.Unlock()
}

func (s *MetricsTestSuite) TestCollectSubProcesses() {
	s.comm.Mu.Lock()
	s.Zero(len(s.comm.ProcInfo[s.id]))
	s.comm.Mu.Unlock()
	cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
	s.NoError(cmd.Start())

	ctx, cancel := context.WithCancel(context.Background())

	go s.collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	cancel()

	s.NoError(cmd.Process.Kill())

	if runtime.GOOS == "windows" {
		s.comm.Mu.Lock()
		s.True(len(s.comm.ProcInfo[s.id]) >= 1)
		s.comm.Mu.Unlock()
	} else {
		s.comm.Mu.Lock()
		s.True(len(s.comm.ProcInfo[s.id]) >= 2)
		s.comm.Mu.Unlock()
	}
}

func (s *MetricsTestSuite) TestPersistSystemStats() {
	ctx, cancel := context.WithCancel(context.Background())

	s.comm.Mu.Lock()
	s.Nil(s.comm.SysInfo[s.id])
	s.comm.Mu.Unlock()

	go s.collector.sysInfoCollector(ctx, 750*time.Millisecond)
	time.Sleep(time.Second)
	cancel()

	s.comm.Mu.Lock()
	s.True(len(s.comm.SysInfo) >= 1)
	s.comm.Mu.Unlock()

	time.Sleep(time.Second)

	s.comm.Mu.Lock()
	s.True(len(s.comm.SysInfo) >= 1)
	s.comm.Mu.Unlock()
}
