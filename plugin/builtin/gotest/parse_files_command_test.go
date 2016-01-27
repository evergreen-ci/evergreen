package gotest_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/model"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/gotest"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
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
			taskConfig := &model.TaskConfig{Task: &model.Task{Id: "taskOne", Execution: 1}}

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
