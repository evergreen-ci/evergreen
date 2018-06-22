package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

const (
	test2JSONFile = "command/testdata/test2json.json"
)

type test2JSONSuite struct {
	args map[string]interface{}
	c    *goTest2JSONCommand

	sender *send.InternalSender
	comm   *client.Mock
	conf   *model.TaskConfig

	suite.Suite
}

func TestGoTest2JSON(t *testing.T) {
	suite.Run(t, &test2JSONSuite{})
}

func (s *test2JSONSuite) SetupTest() {
	s.c = goTest2JSONFactory().(*goTest2JSONCommand)
	s.c.Files = []string{test2JSONFile}

	s.args = map[string]interface{}{
		"files": []string{test2JSONFile},
	}
	s.Equal("gotest.parse_json", s.c.Name())

	s.comm = &client.Mock{
		LogID: "log0",
	}
	s.conf = &model.TaskConfig{
		Task: &task.Task{
			Id: "task0",
		},
	}
	s.sender = send.MakeInternalLogger()
}

func (s *test2JSONSuite) TestNoFiles() {
	s.c.Files = []string{}
	s.args = map[string]interface{}{}
	s.EqualError(s.c.ParseParams(s.args), "error validating params: must specify at least one file pattern to parse: 'map[]'")

	s.c.Files = []string{}
	s.args = map[string]interface{}{
		"files": []string{},
	}
	s.EqualError(s.c.ParseParams(s.args), "error validating params: must specify at least one file pattern to parse: 'map[files:[]]'")
}

func (s *test2JSONSuite) TestParseArgs() {
	s.c.Files = []string{}
	s.args = map[string]interface{}{
		"files": []string{test2JSONFile, "some/other/file.json"},
	}
	s.NoError(s.c.ParseParams(s.args))
	s.Equal(s.args["files"], s.c.Files)

	s.args = map[string]interface{}{
		"files": []string{"some/other/*.json"},
	}

	s.c.Files = []string{}
	s.NoError(s.c.ParseParams(s.args))
	// TODO TEST EXPANSIONS
}

func (s *test2JSONSuite) TestExecute() {
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.c.Execute(context.Background(), s.comm, logger, s.conf)

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 13)
	s.Equal(13, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 13)

	logFiles := map[string]bool{}
	for _, result := range s.comm.LocalTestResults.Results {
		_, ok := logFiles[result.LogId]
		s.False(ok)
		logFiles[result.LogId] = true
	}
}

func (s *test2JSONSuite) noErrorMessages(msgs []*send.InternalMessage) {
	for i := range msgs {
		if msgs[i].Priority >= level.Warning {
			s.T().Errorf("message: '%s' had level: %s", msgs[i].Message.String(), msgs[i].Level)
		}
	}
}

func drainMessages(sender *send.InternalSender) []*send.InternalMessage {
	out := []*send.InternalMessage{}
	for msg, ok := sender.GetMessageSafe(); ok; msg, ok = sender.GetMessageSafe() {
		out = append(out, msg)
	}

	return out
}
