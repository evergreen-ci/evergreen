package test2json

import (
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	test2JSONFile                = "testdata/test2json.json"
	test2JSONPanicFile           = "testdata/test2json_panic.json"
	test2JSONBenchmarkFile       = "testdata/test2json_benchmark.json"
	test2JSONSilentBenchmarkFile = "testdata/test2json_benchmark_silent.json"
)

type testEventExpectation struct {
	StartTime string
	EndTime   string
	Status    string
	LineInLog int
}

type test2JSONSuite struct {
	suite.Suite
}

func TestGoTest2JSON(t *testing.T) {
	suite.Run(t, &test2JSONSuite{})
}

func (s *test2JSONSuite) TestExecute() {
	data, err := ioutil.ReadFile(test2JSONFile)
	s.NoError(err)

	out, err := ProcessBytes(data)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Len(out.Tests, 14)
	s.Len(out.Log, 96)
	s.saneTestResults(out)

	expectedResults := map[string]testEventExpectation{
		"TestConveyPass": {
			StartTime: "2018-06-25T12:05:04.017015269-04:00",
			EndTime:   "2018-06-25T12:05:34.035040344-04:00",
			Status:    Passed,
			LineInLog: 4,
		},
		"TestConveyFail": {
			StartTime: "2018-06-25T12:05:04.017036781-04:00",
			EndTime:   "2018-06-25T12:05:34.034868188-04:00",
			Status:    Failed,
			LineInLog: 6,
		},
		"TestFailingButInAnotherFile": {
			StartTime: "2018-06-25T12:05:24.032450248-04:00",
			EndTime:   "2018-06-25T12:05:34.034820373-04:00",
			Status:    Failed,
			LineInLog: 30,
		},
		"TestNativeTestPass": {
			StartTime: "2018-06-25T12:05:04.01707699-04:00",
			EndTime:   "2018-06-25T12:05:34.034725614-04:00",
			Status:    Passed,
			LineInLog: 8,
		},
		"TestNativeTestFail": {
			StartTime: "2018-06-25T12:05:04.017098154-04:00",
			EndTime:   "2018-06-25T12:05:34.034919477-04:00",
			Status:    Failed,
			LineInLog: 10,
		},
		"TestPassingButInAnotherFile": {
			StartTime: "2018-06-25T12:05:24.032378982-04:00",
			EndTime:   "2018-06-25T12:05:34.035090616-04:00",
			Status:    Passed,
			LineInLog: 28,
		},
		"TestSkippedTestFail": {
			StartTime: "2018-06-25T12:05:04.0171186-04:00",
			EndTime:   "2018-06-25T12:05:44.034981576-04:00",
			Status:    Skipped,
			LineInLog: 12,
		},
		"TestTestifyFail": {
			StartTime: "2018-06-25T12:05:04.016984664-04:00",
			EndTime:   "2018-06-25T12:05:44.034874417-04:00",
			Status:    Failed,
			LineInLog: 2,
		},
		"TestTestifyPass": {
			StartTime: "2018-06-25T12:05:04.016652959-04:00",
			EndTime:   "2018-06-25T12:05:34.034985982-04:00",
			Status:    Passed,
			LineInLog: 0,
		},
		"TestTestifySuite": {
			StartTime: "2018-06-25T12:05:14.024473312-04:00",
			EndTime:   "2018-06-25T12:05:24.032367066-04:00",
			Status:    Passed,
			LineInLog: 22,
		},
		"TestTestifySuiteFail": {
			StartTime: "2018-06-25T12:05:04.017169659-04:00",
			EndTime:   "2018-06-25T12:05:14.024456046-04:00",
			Status:    Failed,
			LineInLog: 14,
		},
		"TestTestifySuiteFail/TestThings": {
			StartTime: "2018-06-25T12:05:04.019154773-04:00",
			EndTime:   "2018-06-25T12:05:14.02441206-04:00",
			Status:    Failed,
			LineInLog: 15,
		},
		"TestTestifySuite/TestThings": {
			StartTime: "2018-06-25T12:05:14.027554888-04:00",
			EndTime:   "2018-06-25T12:05:24.03234939-04:00",
			Status:    Passed,
			LineInLog: 23,
		},
		"": {
			StartTime: "2018-06-25T12:05:04.006218299-04:00",
			EndTime:   "2018-06-25T12:05:44.037218299-04:00",
			Status:    Failed,
			LineInLog: 0,
		},
	}
	s.Len(expectedResults, 14)
	s.doTableTest(out, expectedResults)
}

func (s *test2JSONSuite) TestExecuteWithBenchmarks() {
	data, err := ioutil.ReadFile(test2JSONBenchmarkFile)
	s.NoError(err)

	out, err := ProcessBytes(data)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Len(out.Tests, 6)
	s.Len(out.Log, 24)
	s.saneTestResults(out)

	expectedResults := map[string]testEventExpectation{
		"BenchmarkSomethingFailing": {
			StartTime: "2018-06-26T11:39:55.144987764-04:00",
			EndTime:   "2018-06-26T11:39:55.144987764-04:00",
			Status:    Failed,
			LineInLog: 12,
		},
		"BenchmarkSomething-8": {
			StartTime: "2018-06-26T11:39:35.141056901-04:00",
			EndTime:   "2018-06-26T11:39:35.141056901-04:00",
			Status:    Passed,
			LineInLog: 7,
		},
		"BenchmarkSomethingElse-8": {
			StartTime: "2018-06-26T11:39:55.144650126-04:00",
			EndTime:   "2018-06-26T11:39:55.144650126-04:00",
			Status:    Passed,
			LineInLog: 11,
		},
		"BenchmarkSomethingSkipped": {
			StartTime: "2018-06-26T11:39:55.145230368-04:00",
			EndTime:   "2018-06-26T11:39:55.145230368-04:00",
			Status:    Skipped,
			LineInLog: 16,
		},
		"BenchmarkSomethingSkippedSilent": {
			StartTime: "2018-06-26T11:39:55.145309703-04:00",
			EndTime:   "2018-06-26T11:39:55.145309703-04:00",
			Status:    Skipped,
			LineInLog: 19,
		},
		"": {
			StartTime: "2018-06-26T11:39:15.13245153-04:00",
			EndTime:   "2018-06-26T11:39:55.14645153-04:00",
			Status:    Failed,
			LineInLog: 0,
		},
	}
	s.doTableTest(out, expectedResults)

	for _, result := range out.Tests {
		// benchmark timings should be 0, except for the package level
		// result
		if result.Name == "" {
			s.NotEqual(result.StartTime, result.EndTime)
			s.True(result.EndTime.Sub(result.StartTime) > 0)
		} else {
			s.Equal(result.StartTime, result.EndTime)
		}
	}
}

func (s *test2JSONSuite) TestExecuteWithSilentBenchmarks() {
	data, err := ioutil.ReadFile(test2JSONSilentBenchmarkFile)
	s.NoError(err)

	out, err := ProcessBytes(data)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Require().Len(out.Tests, 1)
	s.Len(out.Log, 6)
	s.saneTestResults(out)
	s.Equal(Passed, out.Tests[TestKey{
		Name:      "",
		Iteration: 0,
	}].Status)
}

func (s *test2JSONSuite) TestExecuteWithFileContainingPanic() {
	data, err := ioutil.ReadFile(test2JSONPanicFile)
	s.NoError(err)

	out, err := ProcessBytes(data)
	s.NoError(err)
	s.Require().NotNil(out)
	s.Len(out.Tests, 3)
	s.Len(out.Log, 24)
	s.saneTestResults(out)

	expectedResults := map[string]testEventExpectation{
		"TestWillPanic": {
			StartTime: "2018-06-25T14:57:29.280697961-04:00",
			EndTime:   "2018-06-25T14:57:39.287082569-04:00",
			Status:    Failed,
			LineInLog: 0,
		},
		"TestTestifySuitePanic": {
			StartTime: "2018-06-25T14:57:29.281022553-04:00",
			EndTime:   "2018-06-25T14:57:29.281073606-04:00",
			Status:    Failed,
			LineInLog: 2,
		},
		"TestTestifySuitePanic/TestWillPanic": {
			StartTime: "2018-06-25T14:57:29.28261188-04:00",
			EndTime:   "2018-06-25T14:57:29.28262775-04:00",
			Status:    Failed,
			LineInLog: 6,
		},
	}
	s.doTableTest(out, expectedResults)
}

func (s *test2JSONSuite) doTableTest(t *TestResults, expectedResults map[string]testEventExpectation) {
	usedResults := map[string]bool{}

	for key, test := range t.Tests {
		expected, ok := expectedResults[test.Name]
		if !ok {
			if test.Name == "" {
				s.T().Log("Missing expected results for package level results ('')")
			} else {
				s.T().Logf("Missing expected results for %s", test.Name)
			}
			continue
		}

		start, err := time.Parse(time.RFC3339Nano, expected.StartTime)
		s.NoError(err)
		s.True(start.Equal(test.StartTime), test.Name)

		end, err := time.Parse(time.RFC3339Nano, expected.EndTime)
		s.NoError(err)
		s.True(end.Equal(test.EndTime), test.Name)

		s.Equal(expected.Status, test.Status, test.Name)
		s.Equal(expected.LineInLog, test.FirstLogLine, test.Name)
		usedResults[test.Name] = true
		s.Equal(0, key.Iteration)
	}
	s.Len(usedResults, len(expectedResults))
}

// Assert: non-zero start/end times, starttime > endtime
func (s *test2JSONSuite) saneTestResults(t *TestResults) {
	for key, result := range t.Tests {
		s.False(result.StartTime.IsZero(), result.Name)
		s.False(result.EndTime.IsZero(), result.Name)
		s.True(result.EndTime.After(result.StartTime) || result.EndTime.Equal(result.StartTime))
		// benchmark output is weird: You'd think you would get a
		// start and an end time, or an average iteration time, but
		// you don't
		if !strings.HasPrefix(result.Name, benchmark) && !strings.Contains(result.Name, "Panic") {
			s.True(result.EndTime.Sub(result.StartTime) >= (10 * time.Second))
		}
		if len(result.Name) == 0 {
			s.Equal(0, key.Iteration)
		}
	}
}
