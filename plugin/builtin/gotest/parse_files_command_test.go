package gotest_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/gotest"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestAllOutputFiles(t *testing.T) {

	Convey("When determining the name of all files to be parsed", t, func() {

		Convey("All specified files should be included", func() {

			pfCmd := &ParseFilesCommand{
				Files: []string{"testdata/monitor.suite", "testdata/util.suite"},
			}
			files, err := pfCmd.AllOutputFiles()
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 2)

		})

		Convey("File patterns should be expanded correctly", func() {

			pfCmd := &ParseFilesCommand{
				Files: []string{"testdata/monitor.suite", "testdata/test_output_dir/*"},
			}
			files, err := pfCmd.AllOutputFiles()
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 4)

		})

		Convey("Duplicates should be removed", func() {

			pfCmd := &ParseFilesCommand{
				Files: []string{"testdata/monitor.suite", "testdata/*.suite"},
			}
			files, err := pfCmd.AllOutputFiles()
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 2)

		})

	})

}

func TestParseOutputFiles(t *testing.T) {

	Convey("When parsing files containing go test output", t, func() {

		Convey("The output in all of the specified files should be parsed correctly", func() {

			// mock up a logger
			sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
			logger := agent.NewTestLogger(sliceAppender)

			// mock up a task config
			taskConfig := &model.TaskConfig{Task: &task.Task{Id: "taskOne", Execution: 1}}

			// the files we want to parse
			files := []string{
				"testdata/monitor.suite",
				"testdata/util.suite",
				"testdata/test_output_dir/monitor_fail.suite",
				"testdata/test_output_dir/evergreen.suite",
			}

			logs, results, err := ParseTestOutputFiles(files, nil, logger, taskConfig)
			So(err, ShouldBeNil)
			So(logs, ShouldNotBeNil)
			So(results, ShouldNotBeNil)
			So(len(results), ShouldEqual, 4)

		})

	})

}

func TestResultConversion(t *testing.T) {
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
			fakeTask := &task.Task{Id: "taskID"}
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
				So(newRes.Results[1].EndTime-newRes.Results[1].StartTime, ShouldBeBetween, 4.9, 5.1)
			})
		})
	})
}
