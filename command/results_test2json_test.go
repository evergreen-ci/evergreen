package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

const (
	test2JSONFile          = "command/testdata/test2json.json"
	test2JSONWindowsFile   = "command/testdata/test2json_windows.json"
	test2JSONBenchmarkFile = "command/testdata/test2json_benchmark.json"
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
		Expansions: util.NewExpansions(map[string]string{}),
	}
	s.conf.Expansions.Put("expandme", test2JSONFile)
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
}

func (s *test2JSONSuite) TestPathExpansions() {
	s.c.Files = []string{"${expandme}"}
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))
	s.Require().Equal(test2JSONFile, s.c.Files[0])
	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)
}

func (s *test2JSONSuite) TestExecute() {
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 14)
	s.Equal(14, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 14)
	s.saneTestResults()

	for _, test := range s.comm.LocalTestResults.Results {
		s.NotEqual(test.StartTime, test.EndTime)
		s.NotEqual(test.EndTime-test.StartTime, 0)
	}
}

func (s *test2JSONSuite) TestExecuteWithBenchmarks() {
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.c.Files[0] = test2JSONBenchmarkFile
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 4)
	s.Equal(4, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 4)
	s.saneTestResults()
}

func (s *test2JSONSuite) TestExecuteWithWindowsResultsFile() {
	s.c.Files[0] = test2JSONWindowsFile
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 14)
	s.Equal(14, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 14)
	s.saneTestResults()
}

func (s *test2JSONSuite) noErrorMessages(msgs []*send.InternalMessage) {
	for i := range msgs {
		if msgs[i].Priority >= level.Warning {
			s.T().Errorf("message: '%s' had level: %s", msgs[i].Message.String(), msgs[i].Level)
		}
	}
}

// Assert: non-zero start/end times, starttime > endtime
//	   no duplicate log IDs
func (s *test2JSONSuite) saneTestResults() {
	logFiles := map[string]bool{}
	for _, result := range s.comm.LocalTestResults.Results {
		_, ok := logFiles[result.LogId]
		s.False(ok)
		logFiles[result.LogId] = true
	}

	for _, result := range s.comm.LocalTestResults.Results {
		s.False(result.StartTime == 0)
		s.False(result.EndTime == 0)
		s.True(result.EndTime >= result.StartTime)
		s.True((result.EndTime - result.StartTime) >= 10)
	}
}

func drainMessages(sender *send.InternalSender) []*send.InternalMessage {
	out := []*send.InternalMessage{}
	for msg, ok := sender.GetMessageSafe(); ok; msg, ok = sender.GetMessageSafe() {
		out = append(out, msg)
	}

	return out
}
