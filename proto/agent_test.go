package proto

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type AgentTestSuite struct {
	suite.Suite
	a                Agent
	mockCommunicator *client.Mock
	tc               *taskContext
}

func TestAgentTestSuite(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

func (s *AgentTestSuite) SetupTest() {
	s.a = Agent{
		opts: Options{
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogPrefix:  "prefix",
		},
		comm: client.NewMock("url"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)

	s.tc = &taskContext{}
	s.tc.task.ID = "task_id"
	s.tc.task.Secret = "task_secret"
	s.tc.logger = s.a.comm.GetLoggerProducer(s.tc.task)
}

func (s *AgentTestSuite) TestNextTaskResponseShouldExit() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: true}
	err := s.a.loop(context.Background())
	s.Error(err)
}

func (s *AgentTestSuite) TestTaskWithoutSecret() {
	s.mockCommunicator.NextTaskResponse = &apimodels.NextTaskResponse{
		TaskId:     "mocktaskid",
		TaskSecret: "",
		ShouldExit: false}
	err := s.a.loop(context.Background())
	s.Error(err)
}

func (s *AgentTestSuite) TestErrorGettingNextTask() {
	s.mockCommunicator.NextTaskShouldFail = true
	err := s.a.loop(context.Background())
	s.Error(err)
}

func (s *AgentTestSuite) TestCanceledContext() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.a.loop(ctx)
	s.NoError(err)
}

func (s *AgentTestSuite) TestAgentEndTaskShouldExit() {
	s.mockCommunicator.EndTaskResponse = &apimodels.EndTaskResponse{ShouldExit: true}
	err := s.a.loop(context.Background())
	s.Error(err)
}

func (s *AgentTestSuite) TestFinishTaskReturnsEndTaskResponse() {
	endTaskResponse := &apimodels.EndTaskResponse{Message: "end task response"}
	s.mockCommunicator.EndTaskResponse = endTaskResponse
	resp, err := s.a.finishTask(context.Background(), s.tc, evergreen.TaskSucceeded, true)
	s.Equal(endTaskResponse, resp)
	s.NoError(err)
}

func (s *AgentTestSuite) TestFinishTaskEndTaskError() {
	s.mockCommunicator.EndTaskShouldFail = true
	resp, err := s.a.finishTask(context.Background(), s.tc, evergreen.TaskSucceeded, true)
	s.Nil(resp)
	s.Error(err)
}

func (s *AgentTestSuite) TestEndTaskResponse() {
	detail := s.a.endTaskResponse(s.tc, evergreen.TaskSucceeded, true)
	s.True(detail.TimedOut)
	s.Equal(evergreen.TaskSucceeded, detail.Status)

	detail = s.a.endTaskResponse(s.tc, evergreen.TaskSucceeded, false)
	s.False(detail.TimedOut)
	s.Equal(evergreen.TaskSucceeded, detail.Status)

	detail = s.a.endTaskResponse(s.tc, evergreen.TaskFailed, true)
	s.True(detail.TimedOut)
	s.Equal(evergreen.TaskFailed, detail.Status)

	detail = s.a.endTaskResponse(s.tc, evergreen.TaskFailed, false)
	s.False(detail.TimedOut)
	s.Equal(evergreen.TaskFailed, detail.Status)
}

func (s *AgentTestSuite) TestAbort() {
	s.mockCommunicator.HeartbeatShouldAbort = true
	err := s.a.startNextTask(context.Background(), s.tc)
	s.NoError(err)
	s.Equal(evergreen.TaskFailed, s.mockCommunicator.EndTaskResult.Detail.Status)
	s.Equal("initial task setup", s.mockCommunicator.EndTaskResult.Detail.Description)
}

func (s *AgentTestSuite) TestAgentConstructorSetsHostData() {
	agent := New(Options{HostID: "host_id", HostSecret: "host_secret"}, client.NewMock("url"))
	s.Equal("host_id", agent.comm.GetHostID())
	s.Equal("host_secret", agent.comm.GetHostSecret())
}
