package command

import (
	"bytes"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
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
		})
	})

}

func TestParserFunctionality(t *testing.T) {
	cwd := testutil.GetDirectoryOfFile()

	Convey("With a simple log file and parser", t, func() {
		logdata, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "gotest", "1_simple.log"))
		testutil.HandleTestingErr(err, t, "couldn't open log file")
		parser := &goTestParser{Suite: "test"}

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
					So(results[0].SuiteName, ShouldEqual, "test")
					So(results[1].Name, ShouldEqual, "TestFailures2")
					So(results[1].Status, ShouldEqual, FAIL)
					rTime, _ = time.ParseDuration("2.00s")
					So(results[1].RunTime, ShouldEqual, rTime)
					So(results[1].StartLine, ShouldEqual, 15)
					So(results[1].EndLine, ShouldEqual, 15)
					So(results[1].SuiteName, ShouldEqual, "test")
				})
			})
		})
	})
	Convey("With a gocheck log file and parser", t, func() {
		logdata, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "gotest", "2_simple.log"))
		testutil.HandleTestingErr(err, t, "couldn't open log file")
		parser := &goTestParser{Suite: "gocheck_test"}

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
					So(results[1].SuiteName, ShouldEqual, "gocheck_test")
				})
			})
		})
	})
	Convey("un-terminated tests are failures", t, func() {
		logdata, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "gotest", "3_simple.log"))
		testutil.HandleTestingErr(err, t, "couldn't open log file")
		parser := &goTestParser{Suite: "gocheck_test"}
		err = parser.Parse(bytes.NewBuffer(logdata))
		So(err, ShouldBeNil)

		results := parser.Results()
		So(len(results), ShouldEqual, 1)
		So(results[0].Name, ShouldEqual, "TestFailures")
		So(results[0].Status, ShouldEqual, FAIL)
	})

}

func matchResultWithLog(tr *goTestResult, logs []string) {
	startLine := logs[tr.StartLine-1]
	endLine := logs[tr.EndLine-1]
	So(startLine, ShouldContainSubstring, tr.Name)
	So(endLine, ShouldContainSubstring, tr.Name)
	So(endLine, ShouldContainSubstring, tr.Status)
}

func TestParserOnRealTests(t *testing.T) {
	// there are some issues with gccgo:
	testutil.SkipTestUnlessAll(t, "TestParserOnRealTests")

	Convey("With a parser", t, func() {
		parser := &goTestParser{}
		Convey("and some real test output", func() {
			cmd := exec.Command("go", "test", "-v", "./.")
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
