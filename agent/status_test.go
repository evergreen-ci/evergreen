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
	defer cancel()
	go func() {
		_ = agt.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:2286/status")
	s.NoError(err)
	s.Equal(200, resp.StatusCode)
}
