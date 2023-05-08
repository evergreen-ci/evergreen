package command

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserRegex(t *testing.T) {
	Convey("With our test regexes", t, func() {
		Convey("a test start should parse and match the test name", func() {
			Convey("for vanilla logs", func() {
				name, err := startInfoFromLogLine("=== RUN   TestParserFunctionality", startRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "TestParserFunctionality")
			})
			Convey("and gocheck logs", func() {
				name, err := startInfoFromLogLine("START: test_file.go:81: TestName.TearDownSuite", gocheckStartRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "TestName.TearDownSuite")
			})
		})
		Convey("a test end should parse and match the test name", func() {
			Convey("for vanilla logs", func() {
				name, status, dur, err := endInfoFromLogLine("--- FAIL: TestParserRegex (0.05s)", endRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "TestParserRegex")
				So(status, ShouldEqual, FAIL)
				So(dur, ShouldEqual, time.Duration(50)*time.Millisecond)
				name, status, dur, err = endInfoFromLogLine("--- PASS: TestParserRegex (0.00s)", endRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "TestParserRegex")
				So(status, ShouldEqual, PASS)
				So(dur, ShouldEqual, time.Duration(0))
				name, status, dur, err = endInfoFromLogLine("--- PASS: TestParserRegex (2m4.0s)", endRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "TestParserRegex")
				So(status, ShouldEqual, PASS)
				expDur, err := time.ParseDuration("2m4s")
				So(err, ShouldBeNil)
				So(dur, ShouldEqual, expDur)
			})
			Convey("and gocheck logs", func() {
				name, status, dur, err := endInfoFromLogLine(
					"FAIL: adjust_test.go:40: AdjustSuite.TestAdjust", gocheckEndRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "AdjustSuite.TestAdjust")
				So(status, ShouldEqual, FAIL)
				So(dur, ShouldEqual, time.Duration(0))
				name, status, dur, err = endInfoFromLogLine(
					"PASS: update_test.go:81: UpdateSuite.TearDownSuite	0.900s", gocheckEndRegex)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "UpdateSuite.TearDownSuite")
				So(status, ShouldEqual, PASS)
				So(dur, ShouldEqual, time.Duration(900)*time.Millisecond)
			})
			Convey("go test build lines", func() {
				path, err := pathNameFromLogLine(
					"FAIL   github.go/evergreen-ci/evergreen/model/patch.go     [build failed] ")
				So(err, ShouldBeNil)
				So(path, ShouldEqual, "github.go/evergreen-ci/evergreen/model/patch.go")

				path, err = pathNameFromLogLine(
					"FAIL github.go/evergreen-ci/evergreen/model/patch.go [build failed]")
				So(err, ShouldBeNil)
				So(path, ShouldEqual, "github.go/evergreen-ci/evergreen/model/patch.go")

				_, err = pathNameFromLogLine(
					"FAIL   github.go/evergreen-ci/evergreen/model/patch.go 2.47s")
				So(err, ShouldNotBeNil)

				_, err = pathNameFromLogLine(
					"ok     github.go/evergreen-ci/evergreen/model/patch.go 2.47s")
				So(err, ShouldNotBeNil)
			})
		})
	})

}

func TestParserFunctionality(t *testing.T) {
	cwd := testutil.GetDirectoryOfFile()

	Convey("With a simple log file and parser", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "1_simple.log"))
		require.NoError(t, err, "couldn't open log file")
		parser := &goTestParser{}

		Convey("running parse on the given log file should succeed", func() {
			err = parser.Parse(bytes.NewBuffer(logdata))
			So(err, ShouldBeNil)

			Convey("and logs should be the correct length", func() {
				logs := parser.Logs()
				So(len(logs), ShouldEqual, 18)
			})

			Convey("and there should be two test results", func() {
				results := parser.Results()
				So(len(results), ShouldEqual, 2)

				Convey("with the proper fields matching the original log file", func() {
					So(results[0].Name, ShouldEqual, "TestFailures")
					So(results[0].Status, ShouldEqual, FAIL)
					rTime, _ := time.ParseDuration("5.02s")
					So(results[0].RunTime, ShouldEqual, rTime)
					So(results[0].StartLine, ShouldEqual, 1)
					So(results[0].EndLine, ShouldEqual, 14)
					So(results[1].Name, ShouldEqual, "TestFailures2")
					So(results[1].Status, ShouldEqual, FAIL)
					rTime, _ = time.ParseDuration("2.00s")
					So(results[1].RunTime, ShouldEqual, rTime)
					So(results[1].StartLine, ShouldEqual, 15)
					So(results[1].EndLine, ShouldEqual, 15)
				})
			})
		})
	})
	Convey("With a gocheck log file and parser", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "2_simple.log"))
		require.NoError(t, err, "couldn't open log file")
		parser := &goTestParser{}

		Convey("running parse on the given log file should succeed", func() {
			err = parser.Parse(bytes.NewBuffer(logdata))
			So(err, ShouldBeNil)

			Convey("and logs should be the correct length", func() {
				logs := parser.Logs()
				So(len(logs), ShouldEqual, 15)
			})

			Convey("and there should be three test results", func() {
				results := parser.Results()
				So(len(results), ShouldEqual, 3)

				Convey("with the proper fields matching the original log file", func() {
					So(results[1].Name, ShouldEqual, "MyTestName.SetUpTest")
					So(results[1].Status, ShouldEqual, PASS)
					rTime, _ := time.ParseDuration("0.576s")
					So(results[1].RunTime, ShouldEqual, rTime)
					So(results[1].StartLine, ShouldEqual, 2)
					So(results[1].EndLine, ShouldEqual, 4)
				})
			})
		})
	})
	Convey("un-terminated tests are failures", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "3_simple.log"))
		require.NoError(t, err, "couldn't open log file")
		parser := &goTestParser{}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldBeNil)

		results := parser.Results()
		So(len(results), ShouldEqual, 1)
		So(results[0].Name, ShouldEqual, "TestFailures")
		So(results[0].Status, ShouldEqual, FAIL)
	})
	Convey("testify suites with leading spaces", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "4_simple.log"))
		So(err, ShouldBeNil)

		parser := &goTestParser{}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldBeNil)

		results := parser.Results()
		So(len(results), ShouldEqual, 19)
		So(results[18].Name, ShouldEqual, "TestClientSuite/TestURLGeneratiorWithoutDefaultPortInResult")
		So(results[18].Status, ShouldEqual, PASS)
	})
	Convey("gotest log with multiple executions of the same test", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "5_simple.log"))
		So(err, ShouldBeNil)

		parser := &goTestParser{}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldBeNil)

		results := parser.Results()
		So(len(results), ShouldEqual, 3)
		So(results[0].Name, ShouldEqual, "Test1")
		So(results[1].Name, ShouldEqual, "TestSameName")
		So(results[2].Name, ShouldEqual, "TestSameName")
		So(results[1].Status, ShouldEqual, PASS)
		So(results[2].Status, ShouldEqual, FAIL)
	})

	Convey("gotest log with negative duration", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "6_simple.log"))
		So(err, ShouldBeNil)

		parser := &goTestParser{}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldBeNil)

		results := parser.Results()
		So(len(results), ShouldEqual, 1)
		So(results[0].Status, ShouldEqual, PASS)
	})

	Convey("deeply nested Subtests", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "7_simple.log"))
		So(err, ShouldBeNil)

		parser := &goTestParser{}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldBeNil)

		results := parser.Results()
		So(len(results), ShouldEqual, 39)
		for idx, r := range results {
			if idx == 0 {
				// first result is the inclosing test,
				// and we should ignore that
				continue
			}
			outcome := strings.Contains(r.Name, "Basic") || strings.Contains(r.Name, "Complex")
			if !outcome {
				fmt.Printf("result '%s' should contain either 'Basic' or 'Complex'", r.Name)
			}
			So(outcome, ShouldBeTrue)

		}
	})

	Convey("gotest log with failed build", t, func() {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "8_simple.log"))
		So(err, ShouldBeNil)

		parser := &goTestParser{}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "github.com/evergreen-ci/evergreen/model/host")
	})

	t.Run("LargeLogLine", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "large_line.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		assert.Error(t, parser.Parse(bytes.NewBuffer(logdata)))
	})
}
