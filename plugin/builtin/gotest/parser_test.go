package gotest

import (
	"bytes"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"
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
		})
	})

}

func TestParserFunctionality(t *testing.T) {
	var parser Parser
	Convey("With a simple log file and parser", t, func() {
		logdata, err := ioutil.ReadFile("testdata/1_simple.log")
		testutil.HandleTestingErr(err, t, "couldn't open log file")
		parser = &VanillaParser{Suite: "test"}

		Convey("running parse on the given log file should succeed", func() {
			err = parser.Parse(bytes.NewBuffer(logdata))
			So(err, ShouldBeNil)

			Convey("and logs should be the correct length", func() {
				logs := parser.Logs()
				So(len(logs), ShouldEqual, 17)
			})

			Convey("and there should be one test result", func() {
				results := parser.Results()
				So(len(results), ShouldEqual, 1)

				Convey("with the proper fields matching the original log file", func() {
					So(results[0].Name, ShouldEqual, "TestFailures")
					So(results[0].Status, ShouldEqual, FAIL)
					rTime, _ := time.ParseDuration("5.02s")
					So(results[0].RunTime, ShouldEqual, rTime)
					So(results[0].StartLine, ShouldEqual, 1)
					So(results[0].EndLine, ShouldEqual, 14)
					So(results[0].SuiteName, ShouldEqual, "test")
				})
			})
		})
	})
	Convey("With a gocheck log file and parser", t, func() {
		logdata, err := ioutil.ReadFile("testdata/2_simple.log")
		testutil.HandleTestingErr(err, t, "couldn't open log file")
		parser = &VanillaParser{Suite: "gocheck_test"}

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
					So(results[0].Name, ShouldEqual, "MyTestName.SetUpTest")
					So(results[0].Status, ShouldEqual, PASS)
					rTime, _ := time.ParseDuration("0.576s")
					So(results[0].RunTime, ShouldEqual, rTime)
					So(results[0].StartLine, ShouldEqual, 2)
					So(results[0].EndLine, ShouldEqual, 4)
					So(results[0].SuiteName, ShouldEqual, "gocheck_test")
				})
			})
		})
	})

}

func matchResultWithLog(tr TestResult, logs []string) {
	startLine := logs[tr.StartLine-1]
	endLine := logs[tr.EndLine-1]
	So(startLine, ShouldContainSubstring, tr.Name)
	So(endLine, ShouldContainSubstring, tr.Name)
	So(endLine, ShouldContainSubstring, tr.Status)
}

func TestParserOnRealTests(t *testing.T) {
	var parser Parser
	startDir, err := os.Getwd()
	testutil.HandleTestingErr(err, t, "error getting current directory")

	Convey("With a parser", t, func() {
		parser = &VanillaParser{}
		Convey("and some real test output", func() {
			// This test runs the parser on real test output from the
			// "github.com/evergreen-ci/evergreen/plugin" package.
			// It has to change the working directory of the test process
			// so that it can call that package instead of "plugin/gotest".
			// This is admittedly pretty hacky, but the "go test" paradigm was not
			// designed with running go test recursively in mind.
			//
			// For a good time, remove the line below and have
			// this test run itself forever and ever.
			testutil.HandleTestingErr(os.Chdir("../.."), t, "error changing directories %v")
			Reset(func() {
				// return to original working directory at the end of the test
				testutil.HandleTestingErr(os.Chdir(startDir), t, "error changing directories %v")
			})

			cmd := exec.Command("go", "test", "-v")
			stdout, err := cmd.StdoutPipe()
			testutil.HandleTestingErr(err, t, "error getting stdout pipe %v")
			testutil.HandleTestingErr(cmd.Start(), t, "couldn't run tests %v")
			err = parser.Parse(stdout)
			testutil.HandleTestingErr(cmd.Wait(), t, "error waiting on test %v")

			Convey("the parser should run successfully", func() {
				So(err, ShouldBeNil)

				Convey("and all results should line up with the logs", func() {
					for _, result := range parser.Results() {
						matchResultWithLog(result, parser.Logs())
					}
				})
			})
		})
	})
}
