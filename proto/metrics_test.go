package proto

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type MetricsTestSuite struct {
	suite.Suite
	stopper   chan bool
	comm      *comm.MockCommunicator
	collector metricsCollector
}

func TestMetricsTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

func (s *MetricsTestSuite) SetupTest() {
	s.stopper = make(chan bool)
	s.comm = &comm.MockCommunicator{
		LogChan: make(chan []apimodels.LogMessage, 100),
		Posts:   map[string][]interface{}{},
	}
	s.collector = metricsCollector{
		comm: s.comm,
		stop: s.stopper,
	}
}

func (s *MetricsTestSuite) TestRunForIntervalAndSendMessages() {
	s.Zero(len(s.comm.Posts["process_info"]))

	go s.collector.processInfoCollector(750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	s.stopper <- true
	firstLen := len(s.comm.Posts["process_info"])
	if runtime.GOOS == "windows" {
		s.True(firstLen >= 1)
	} else {
		s.True(firstLen >= 2)
	}
	// after stopping it shouldn't continue to collect stats
	time.Sleep(time.Second)
	s.Equal(firstLen, len(s.comm.Posts["process_info"]))

	for _, post := range s.comm.Posts["process_info"] {
		out, ok := post.([]message.Composer)
		s.True(ok)
		s.Len(out, 1)
	}
}

func (s *MetricsTestSuite) TestCollectSubProcesses() {
	s.Zero(len(s.comm.Posts["process_info"]))
	cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
	s.NoError(cmd.Start())
	go s.collector.processInfoCollector(750*time.Millisecond, time.Second, 2)
	time.Sleep(time.Second)
	s.stopper <- true
	s.NoError(cmd.Process.Kill())

	if runtime.GOOS == "windows" {
		s.Equal(len(s.comm.Posts["process_info"]), 1)
	} else {
		s.Equal(len(s.comm.Posts["process_info"]), 2)
	}
	for _, post := range s.comm.Posts["process_info"] {
		out, ok := post.([]message.Composer)
		s.True(ok)

		// the number of posts is different on windows,
		if runtime.GOOS == "windows" {
			s.True(len(out) >= 1)
		} else {
			s.True(len(out) >= 2)
		}

	}
}

func (s *MetricsTestSuite) TestPersistSystemStats() {
	s.Zero(len(s.comm.Posts["process_info"]))

	go s.collector.sysInfoCollector(750 * time.Millisecond)
	time.Sleep(time.Second)
	s.stopper <- true

	s.True(len(s.comm.Posts["system_info"]) >= 1)
	time.Sleep(time.Second)
	s.True(len(s.comm.Posts["system_info"]) >= 1)
}
