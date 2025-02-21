package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	s.Equal(s.resp.BuildRevision, evergreen.BuildRevision)
	s.Equal(s.resp.AgentVersion, evergreen.AgentVersion)
	s.Equal(s.resp.AgentPid, os.Getpid())
	s.Equal(s.resp.HostId, s.testOpts.HostID)
}

func (s *StatusSuite) TestPopulateSystemInfo() {
	s.NotNil(s.resp.SystemInfo)
}

func (s *StatusSuite) TestProcessTreeInfo() {
	s.GreaterOrEqual(len(s.resp.ProcessTree), 1)
	for _, ps := range s.resp.ProcessTree {
		s.NotNil(ps)
	}
}

func (s *StatusSuite) TestAgentStartsStatusServer() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	agt, err := newWithCommunicator(ctx, s.testOpts, client.NewMock("url"))
	s.Require().NoError(err)

	sender, err := agt.GetSender(ctx, globals.LogOutputStdout, "agent", "t", 0)
	s.Require().NoError(err)
	agt.SetDefaultLogger(sender)

	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true
	go func() {
		_ = agt.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:2286/status")
	s.Require().NoError(err)
	resp.Body.Close()
	s.Equal(200, resp.StatusCode)
}

func (s *StatusSuite) TestAgentFailsToStartTwice() {
	_, err := http.Get("http://127.0.0.1:2287/status")
	s.Require().Error(err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	s.cancel = cancel

	s.testOpts.StatusPort = 2287
	agt, err := newWithCommunicator(ctx, s.testOpts, client.NewMock("url"))
	s.Require().NoError(err)

	sender, err := agt.GetSender(ctx, globals.LogOutputStdout, "agent", "t", 0)
	s.Require().NoError(err)
	agt.SetDefaultLogger(sender)

	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true

	first := make(chan error, 1)
	go func(c chan error) {
		c <- agt.Start(ctx)
	}(first)

	resp, err := http.Get("http://127.0.0.1:2287/status")
	if err != nil {
		// the service hasn't started.

		timer := time.NewTimer(0)
		defer timer.Stop()
	retryLoop:
		for {
			select {
			case <-ctx.Done():
				break retryLoop
			case <-timer.C:
				resp, err = http.Get("http://127.0.0.1:2287/status")
				if err == nil {
					break retryLoop
				}
				timer.Reset(10 * time.Millisecond)
			}
		}
	}

	s.Require().NoError(err)
	resp.Body.Close()
	s.Equal(200, resp.StatusCode)

	second := make(chan error, 1)
	go func(c chan error) {
		secondCtx, secondCancel := context.WithCancel(context.Background())
		defer secondCancel()
		c <- agt.Start(secondCtx)
	}(second)

	select {
	case <-ctx.Done():
		s.Fail("first agent status server stopped before second status server could attempt to start")
	case err = <-second:
	}
	s.Error(err)
	s.Contains(err.Error(), "another process is running on localhost port 2287")

	cancel()
	err = <-first
	s.Require().NoError(err)
}

func (s *StatusSuite) TestCheckOOMSucceeds() {
	if runtime.GOOS == "darwin" {
		s.T().Skip("OOM tests will not work on static mac hosts because logs are never cleared and will be too long to parse")
	}
	if os.Getenv("IS_DOCKER") == "true" {
		s.T().Skip("OOM checker is not supported for docker")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	agt, err := newWithCommunicator(ctx, s.testOpts, client.NewMock("url"))
	s.Require().NoError(err)
	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true

	go func() {
		_ = agt.Start(ctx)
	}()

	resp, err := http.Get("http://127.0.0.1:2286/jasper/v1/list/oom")
	if err != nil {
		// the service hasn't started.

		timer := time.NewTimer(0)
		defer timer.Stop()
	retryLoop:
		for {
			select {
			case <-ctx.Done():
				break retryLoop
			case <-timer.C:
				resp, err = http.Get("http://127.0.0.1:2286/jasper/v1/list/oom")
				if err == nil {
					break retryLoop
				}
				timer.Reset(10 * time.Millisecond)
			}
		}
	}

	s.Require().NoError(err)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, err := io.ReadAll(resp.Body)
		grip.Error(err)
		s.FailNow(fmt.Sprintf("received status code %d from OOM endpoint with body %s", resp.StatusCode, string(b)))
	}

	tracker := jasper.NewOOMTracker()
	s.NoError(utility.ReadJSON(resp.Body, tracker))
	lines, pids := tracker.Report()
	s.Empty(lines)
	s.Empty(pids)
}

func TestUnmarshalTriggerEndTaskResp(t *testing.T) {
	body := `{"status":"failed", "type":"setup", "desc":"this should be set", "should_continue":true}`
	resp := triggerEndTaskResp{}

	assert.NoError(t, json.Unmarshal([]byte(body), &resp))
	assert.Equal(t, "failed", resp.Status)
	assert.Equal(t, "setup", resp.Type)
	assert.Equal(t, "this should be set", resp.Description)
	assert.True(t, resp.ShouldContinue)
}

func TestUnmarshalAddMetadataTagResp(t *testing.T) {
	body := `{"add_failure_metadata_tags": ["failure_tag"]}`
	resp := triggerAddMetadataTagResp{}

	assert.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.Len(t, resp.AddFailureMetadataTags, 1)
	assert.Equal(t, "failure_tag", resp.AddFailureMetadataTags[0])
}
