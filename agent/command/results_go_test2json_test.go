package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

func test2JSONFile() string {
	return filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "test2json.json")
}

type test2JSONSuite struct {
	args map[string]interface{}
	c    *goTest2JSONCommand

	sender   *send.InternalSender
	comm     *client.Mock
	cedarSrv *timberutil.MockCedarServer
	conf     *internal.TaskConfig

	suite.Suite
}

func TestGoTest2JSON(t *testing.T) {
	suite.Run(t, &test2JSONSuite{})
}

func (s *test2JSONSuite) SetupTest() {
	s.c = goTest2JSONFactory().(*goTest2JSONCommand)
	s.c.Files = []string{test2JSONFile()}

	s.args = map[string]interface{}{
		"files": []string{test2JSONFile()},
	}
	s.Equal("gotest.parse_json", s.c.Name())
	s.comm = &client.Mock{
		LogID: "log0",
	}
	s.cedarSrv = setupCedarServer(context.TODO(), s.T(), s.comm)
	s.conf = &internal.TaskConfig{
		Task: &task.Task{
			Id: "task0",
		},
		ProjectRef: &model.ProjectRef{},
		Expansions: util.NewExpansions(map[string]string{}),
	}
	s.conf.Expansions.Put("expandme", test2JSONFile())
	s.sender = send.MakeInternalLogger()
}

func (s *test2JSONSuite) TestNoFiles() {
	s.c.Files = []string{}
	s.args = map[string]interface{}{}
	err := s.c.ParseParams(s.args)
	s.Require().Error(err)
	s.Contains(err.Error(), "must specify at least one file pattern to parse")

	s.c.Files = []string{}
	s.args = map[string]interface{}{
		"files": []string{},
	}
	err = s.c.ParseParams(s.args)
	s.Require().Error(err)
	s.Contains(err.Error(), "must specify at least one file pattern to parse")

	err = s.c.ParseParams(nil)
	s.Require().Error(err)
	s.Contains(err.Error(), "must specify at least one file pattern to parse")
}

func (s *test2JSONSuite) TestParseArgs() {
	s.c.Files = []string{}
	s.args = map[string]interface{}{
		"files": []string{test2JSONFile(), "some/other/file.json"},
	}
	s.NoError(s.c.ParseParams(s.args))
	s.Equal(s.args["files"], s.c.Files)
}

func (s *test2JSONSuite) TestPathExpansions() {
	s.c.Files = []string{"${expandme}"}
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))
	s.Require().Equal(test2JSONFile(), s.c.Files[0])
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

	s.Require().Len(s.cedarSrv.TestResults.Results, 1)
	s.True(s.comm.HasCedarResults)
	s.True(s.comm.CedarResultsFailed)
	s.Len(s.cedarSrv.Buildlogger.Data, 1)
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
