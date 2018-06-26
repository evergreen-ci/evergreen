package command

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/k0kubun/pp"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

const (
	test2JSONFile                = "command/testdata/test2json.json"
	test2JSONPanicFile           = "command/testdata/test2json_panic.json"
	test2JSONWindowsFile         = "command/testdata/test2json_windows.json"
	test2JSONBenchmarkFile       = "command/testdata/test2json_benchmark.json"
	test2JSONSilentBenchmarkFile = "command/testdata/test2json_benchmark_silent.json"
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

	s.EqualError(s.c.ParseParams(nil), "error validating params: must specify at least one file pattern to parse: 'map[]'")
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

	s.Len(s.comm.LocalTestResults.Results, 13)
	s.Equal(1, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 1)
	s.saneTestResults()

	expectedResults := map[string]testEventExpectation{
		"TestConveyPass": {
			StartTime: "2018-06-25T12:05:04.017015269-04:00",
			EndTime:   "2018-06-25T12:05:34.035040344-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 4,
		},
		"TestConveyFail": {
			StartTime: "2018-06-25T12:05:04.017036781-04:00",
			EndTime:   "2018-06-25T12:05:34.034868188-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 6,
		},
		"TestFailingButInAnotherFile": {
			StartTime: "2018-06-25T12:05:24.032450248-04:00",
			EndTime:   "2018-06-25T12:05:34.034820373-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 30,
		},
		"TestNativeTestPass": {
			StartTime: "2018-06-25T12:05:04.01707699-04:00",
			EndTime:   "2018-06-25T12:05:34.034725614-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 8,
		},
		"TestNativeTestFail": {
			StartTime: "2018-06-25T12:05:04.017098154-04:00",
			EndTime:   "2018-06-25T12:05:34.034919477-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 10,
		},
		"TestPassingButInAnotherFile": {
			StartTime: "2018-06-25T12:05:24.032378982-04:00",
			EndTime:   "2018-06-25T12:05:34.035090616-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 28,
		},
		"TestSkippedTestFail": {
			StartTime: "2018-06-25T12:05:04.0171186-04:00",
			EndTime:   "2018-06-25T12:05:44.034981576-04:00",
			Status:    evergreen.TestSkippedStatus,
			LineInLog: 12,
		},
		"TestTestifyFail": {
			StartTime: "2018-06-25T12:05:04.016984664-04:00",
			EndTime:   "2018-06-25T12:05:44.034874417-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 2,
		},
		"TestTestifyPass": {
			StartTime: "2018-06-25T12:05:04.016652959-04:00",
			EndTime:   "2018-06-25T12:05:34.034985982-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 0,
		},
		"TestTestifySuite": {
			StartTime: "2018-06-25T12:05:14.024473312-04:00",
			EndTime:   "2018-06-25T12:05:24.032367066-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 22,
		},
		"TestTestifySuiteFail": {
			StartTime: "2018-06-25T12:05:04.017169659-04:00",
			EndTime:   "2018-06-25T12:05:14.024456046-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 14,
		},
		"TestTestifySuiteFail/TestThings": {
			StartTime: "2018-06-25T12:05:04.019154773-04:00",
			EndTime:   "2018-06-25T12:05:14.02441206-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 15,
		},
		"TestTestifySuite/TestThings": {
			StartTime: "2018-06-25T12:05:14.027554888-04:00",
			EndTime:   "2018-06-25T12:05:24.03234939-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 23,
		},
	}
	s.Len(expectedResults, 13)
	s.doTableTest(expectedResults)
}

func (s *test2JSONSuite) TestExecuteWithBenchmarks() {
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.c.Files[0] = test2JSONBenchmarkFile
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 5)
	s.Equal(1, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 1)
	s.saneTestResults()

	expectedResults := map[string]testEventExpectation{
		"BenchmarkSomethingFailing": {
			StartTime: "2018-06-26T11:39:55.144987764-04:00",
			EndTime:   "2018-06-26T11:39:55.144987764-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 12,
		},
		"BenchmarkSomething-8": {
			StartTime: "2018-06-26T11:39:35.141056901-04:00",
			EndTime:   "2018-06-26T11:39:35.141056901-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 0, // TODO
		},
		"BenchmarkSomethingElse-8": {
			StartTime: "2018-06-26T11:39:55.144650126-04:00",
			EndTime:   "2018-06-26T11:39:55.144650126-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 0, // TODO
		},
		"BenchmarkSomethingSilent-8": {
			StartTime: "2018-06-25T14:31:30.819257417-04:00",
			EndTime:   "2018-06-25T14:31:30.819257417-04:00",
			Status:    evergreen.TestSucceededStatus,
			LineInLog: 15,
		},
		"BenchmarkSomethingSkipped": {
			StartTime: "2018-06-26T11:39:55.145230368-04:00",
			EndTime:   "2018-06-26T11:39:55.145230368-04:00",
			Status:    evergreen.TestSkippedStatus,
			LineInLog: 16,
		},
		"BenchmarkSomethingSkippedSilent": {
			StartTime: "2018-06-26T11:39:55.145309703-04:00",
			EndTime:   "2018-06-26T11:39:55.145309703-04:00",
			Status:    evergreen.TestSkippedStatus,
			LineInLog: 19,
		},
	}
	s.doTableTest(expectedResults)

	for _, result := range s.comm.LocalTestResults.Results {
		// benchmark timings should be 0, except for the package level
		// result
		if strings.HasPrefix(result.TestFile, "package-") {
			s.NotEqual(result.StartTime, result.EndTime)
			s.True((result.EndTime - result.StartTime) > 0)
		} else {
			s.Equal(result.StartTime, result.EndTime)
		}
	}
}

func (s *test2JSONSuite) TestExecuteWithSilentBenchmarks() {
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.c.Files[0] = test2JSONSilentBenchmarkFile
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	pp.Println(msgs)
	s.Len(msgs, 3)

	s.Nil(s.comm.LocalTestResults)
	s.Equal(1, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 1)
}

func (s *test2JSONSuite) TestExecuteWithWindowsResultsFile() {
	s.c.Files[0] = test2JSONWindowsFile
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 13)
	s.Equal(1, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 1)
	s.saneTestResults()
}

type testEventExpectation struct {
	StartTime string
	EndTime   string
	Status    string
	LineInLog int
}

func (s *test2JSONSuite) TestExecuteWithFileContainingPanic() {
	s.c.Files[0] = test2JSONPanicFile
	logger := client.NewSingleChannelLogHarness("test", s.sender)
	s.Require().NoError(s.c.Execute(context.Background(), s.comm, logger, s.conf))

	msgs := drainMessages(s.sender)
	s.Len(msgs, 5)
	s.noErrorMessages(msgs)

	s.Len(s.comm.LocalTestResults.Results, 3)
	s.Equal(1, s.comm.TestLogCount)
	s.Len(s.comm.TestLogs, 1)
	s.saneTestResults()

	expectedResults := map[string]testEventExpectation{
		"TestWillPanic": {
			StartTime: "2018-06-25T14:57:29.280697961-04:00",
			EndTime:   "2018-06-25T14:57:39.287082569-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 0,
		},
		"TestTestifySuitePanic": {
			StartTime: "2018-06-25T14:57:29.281022553-04:00",
			EndTime:   "2018-06-25T14:57:29.281073606-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 2,
		},
		"TestTestifySuitePanic/TestWillPanic": {
			StartTime: "2018-06-25T14:57:29.281033889-04:00",
			EndTime:   "2018-06-25T14:57:29.28262775-04:00",
			Status:    evergreen.TestFailedStatus,
			LineInLog: 6,
		},
	}
	s.doTableTest(expectedResults)
}

func (s *test2JSONSuite) doTableTest(expectedResults map[string]testEventExpectation) {
	for _, test := range s.comm.LocalTestResults.Results {
		expected, ok := expectedResults[test.TestFile]
		if !ok {
			s.T().Logf("Missing expected results for %s", test.TestFile)
			continue
		}

		start, err := time.Parse(time.RFC3339Nano, expected.StartTime)
		s.NoError(err)
		s.Equal(float64(start.Unix()), test.StartTime, test.TestFile)

		end, err := time.Parse(time.RFC3339Nano, expected.EndTime)
		s.NoError(err)
		s.Equal(float64(end.Unix()), test.EndTime, test.TestFile)

		s.Equal(expected.Status, test.Status)
		s.Equal(expected.LineInLog, test.LineNum, test.TestFile)
	}
}

func (s *test2JSONSuite) noErrorMessages(msgs []*send.InternalMessage) {
	for i := range msgs {
		if msgs[i].Priority >= level.Warning {
			s.T().Errorf("message: '%s' had level: %s", msgs[i].Message.String(), msgs[i].Level)
		}
	}
}

// Assert: non-zero start/end times, starttime > endtime
func (s *test2JSONSuite) saneTestResults() {
	s.Require().NotNil(s.comm.LocalTestResults)
	for _, result := range s.comm.LocalTestResults.Results {
		s.False(result.StartTime == 0)
		s.False(result.EndTime == 0)
		s.True(result.EndTime >= result.StartTime)
		// benchmark output is weird: You'd think you would get a
		// start and an end time, or an average iteration time, but
		// you don't
		if !strings.HasPrefix(result.TestFile, "Benchmark") && !strings.Contains(result.TestFile, "Panic") {
			s.True((result.EndTime - result.StartTime) >= 10)
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
