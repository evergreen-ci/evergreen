package gotest_test

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/model"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/gotest"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
	"time"
)

func TestRunAndParseTests(t *testing.T) {
	var parser Parser
	if testing.Verbose() {
		fmt.Print(
			"\nNOTE: This test will fail if the tests for the " +
				"mci/plugin/builtin/archivePlugin package fail\n")
	}

	SkipConvey("With a parser", t, func() {
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)
		parser = &VanillaParser{}

		Convey("and a valid test config", func() {
			config := TestConfig{Dir: "../archivePlugin", Args: ""}

			Convey("execution should run correctly", func() {
				originalDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")

				passed, err := RunAndParseTests(config, parser, logger, make(chan bool))
				So(passed, ShouldEqual, true)
				So(err, ShouldBeNil)
				So(len(parser.Results()), ShouldBeGreaterThan, 0)

				curDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
				So(originalDir, ShouldEqual, curDir)
			})
		})

		Convey("with environment variables", func() {
			config := TestConfig{
				Dir:                  "testdata/envpkg",
				Args:                 "",
				EnvironmentVariables: []string{"PATH=$PATH:bacon"},
			}

			Convey("execution should run correctly", func() {
				originalDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")

				passed, err := RunAndParseTests(config, parser, logger, make(chan bool))
				So(passed, ShouldEqual, true)
				So(err, ShouldBeNil)
				So(len(parser.Results()), ShouldBeGreaterThan, 0)

				curDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
				So(originalDir, ShouldEqual, curDir)
			})
		})

		Convey("and an invalid test directory", func() {
			config := TestConfig{Dir: "directory/doesntexist", Args: ""}

			Convey("execution should fail", func() {
				originalDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")

				passed, err := RunAndParseTests(config, parser, logger, make(chan bool))
				So(passed, ShouldEqual, false)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "directory")
				So(len(parser.Results()), ShouldEqual, 0)

				curDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
				So(originalDir, ShouldEqual, curDir)
			})
		})

		Convey("and an invalid test flag", func() {
			config := TestConfig{Dir: "../archivePlugin", Args: "-aquaman"}

			Convey("execution should fail", func() {
				originalDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")

				passed, err := RunAndParseTests(config, parser, logger, make(chan bool))
				So(passed, ShouldEqual, false)
				So(err, ShouldBeNil)
				So(len(parser.Results()), ShouldEqual, 0)

				curDir, err := os.Getwd()
				testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
				So(originalDir, ShouldEqual, curDir)
			})
		})

	})
}

func TestResultsConversion(t *testing.T) {
	Convey("With a set of results", t, func() {
		results := []TestResult{
			{
				Name:    "TestNothing",
				RunTime: 244 * time.Millisecond,
				Status:  PASS,
			},
			{
				Name:    "TestTwo",
				RunTime: 5000 * time.Millisecond,
				Status:  SKIP,
			},
		}

		Convey("and their converted form", func() {
			fakeTask := &model.Task{Id: "taskID"}
			newRes := ToModelTestResults(fakeTask, results)
			So(len(newRes.Results), ShouldEqual, len(results))

			Convey("fields should be transformed correctly", func() {
				So(newRes.Results[0].TestFile, ShouldEqual, results[0].Name)
				So(newRes.Results[0].Status, ShouldEqual, evergreen.TestSucceededStatus)
				So(newRes.Results[0].StartTime, ShouldBeLessThan, newRes.Results[0].EndTime)
				So(newRes.Results[0].EndTime-newRes.Results[0].StartTime,
					ShouldBeBetween, .243, .245) //floating point weirdness
				So(newRes.Results[1].TestFile, ShouldEqual, results[1].Name)
				So(newRes.Results[1].Status, ShouldEqual, evergreen.TestSkippedStatus)
				So(newRes.Results[1].StartTime, ShouldBeLessThan, newRes.Results[1].EndTime)
				So(newRes.Results[1].EndTime-newRes.Results[1].StartTime,
					ShouldBeBetween, 4.9, 5.1)
			})
		})
	})
}
