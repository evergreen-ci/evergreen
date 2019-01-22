package agent

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/suite"
)

type StatusSuite struct {
	suite.Suite
	testOpts Options
	resp     statusResponse
	cancel   context.CancelFunc
}

func TestStatusSuite(t *testing.T) {
	suite.Run(t, new(StatusSuite))
}

func (s *StatusSuite) SetupTest() {
	s.testOpts = Options{
		HostID:     "none",
		StatusPort: 2286,
	}
	s.resp = buildResponse(s.testOpts)
}

func (s *StatusSuite) TearDownSuite() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *StatusSuite) TestBasicAssumptions() {
	s.Equal(s.resp.BuildId, evergreen.BuildRevision)
	s.Equal(s.resp.AgentPid, os.Getpid())
	s.Equal(s.resp.HostId, s.testOpts.HostID)
}

func (s *StatusSuite) TestPopulateSystemInfo() {
	grip.Alert(strings.Join(s.resp.SystemInfo.Errors, ";\n"))
	grip.Info(s.resp.SystemInfo)
	s.NotNil(s.resp.SystemInfo)
}

func (s *StatusSuite) TestProcessTreeInfo() {
	s.True(len(s.resp.ProcessTree) >= 1)
	for _, ps := range s.resp.ProcessTree {
		s.NotNil(ps)
	}
}

func (s *StatusSuite) TestAgentStartsStatusServer() {
	agt := New(s.testOpts, client.NewMock("url"))

	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		_ = agt.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:2286/status")
	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)
}

func (s *StatusSuite) TestAgentFailsToStartTwice() {
	resp, err := http.Get("http://127.0.0.1:2287/status")
	s.Error(err)

	s.testOpts.StatusPort = 2287
	agt := New(s.testOpts, client.NewMock("url"))
	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go func() {
		err = agt.Start(ctx)
		s.NoError(err)
	}()
	time.Sleep(100 * time.Millisecond)
	resp, err = http.Get("http://127.0.0.1:2287/status")
	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)

	go func() {
		err = agt.Start(ctx)
		s.Error(err)
		s.Contains(err.Error(), "another agent is running on 2287")
	}()
}
