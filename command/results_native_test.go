package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func resetTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, model.TestLogCollection),
		"error clearing test collections")
}

func TestAttachResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := evergreen.GetEnvironment()
	testConfig := env.Settings()

	cwd := testutil.GetDirectoryOfFile()
	comm := client.NewMock("http://localhost.com")

	resetTasks(t)
	SkipConvey("With attachResults plugin installed into plugin registry", t, func() {

		configFile := filepath.Join(cwd, "testdata", "attach", "plugin_attach_results.yml")
		resultsLoc := filepath.Join(cwd, "testdata", "attach", "plugin_attach_results.json")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configFile, modelutil.NoPatch)
		require.NoError(t, err, "failed to setup test data")
		So(err, ShouldBeNil)
		modelData.TaskConfig.WorkDir = "."

		Convey("all commands in test project should execute successfully", func() {
			conf := modelData.TaskConfig
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
			So(err, ShouldBeNil)

			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := Render(command, conf.Project.Functions)
					require.NoError(t, err, "Couldn't get plugin command: %s", command.Command)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(task.ById(conf.Task.Id))
					require.NoError(t, err, "Couldn't find task")
					So(testTask, ShouldNotBeNil)
					// ensure test results are exactly as expected
					// attempt to open the file
					reportFile, err := os.Open(resultsLoc)
					require.NoError(t, err, "Couldn't open report file: '%v'", err)
					results := &task.LocalTestResults{}
					err = util.ReadJSONInto(reportFile, results)
					require.NoError(t, err, "Couldn't read report file: '%v'", err)
					testResults := *results
					So(testTask.LocalTestResults, ShouldResemble, testResults.Results)
					require.NoError(t, err, "Couldn't clean up test temp dir")
				}
			}
		})
	})
}
func TestAttachRawResults(t *testing.T) {
	resetTasks(t)
	testConfig := testutil.TestConfig()
	cwd := testutil.GetDirectoryOfFile()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")

	SkipConvey("With attachResults plugin installed into plugin registry", t, func() {
		configFile := filepath.Join(cwd, "testdata", "attach", "plugin_attach_results_raw.yml")
		resultsLoc := filepath.Join(cwd, "testdata", "attach", "plugin_attach_results_raw.json")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configFile, modelutil.NoPatch)
		require.NoError(t, err, "failed to setup test data")

		modelData.TaskConfig.WorkDir = "."
		conf := modelData.TaskConfig
		logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
		So(err, ShouldBeNil)

		Convey("when attaching a raw log ", func() {
			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {

					pluginCmds, err := Render(command, conf.Project.Functions)
					require.NoError(t, err, "Couldn't get plugin command: %s", command.Command)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					// create a plugin communicator

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					Convey("when retrieving task", func() {
						// fetch the task
						testTask, err := task.FindOne(task.ById(conf.Task.Id))
						require.NoError(t, err, "Couldn't find task")
						So(testTask, ShouldNotBeNil)

						Convey("test results should match and raw log should be in appropriate collection", func() {

							reportFile, err := os.Open(resultsLoc)
							require.NoError(t, err, "Couldn't open report file: '%v'", err)
							results := &task.LocalTestResults{}
							err = util.ReadJSONInto(reportFile, results)
							require.NoError(t, err, "Couldn't read report file: '%v'", err)

							testResults := *results
							So(len(testResults.Results), ShouldEqual, 3)
							So(len(testTask.LocalTestResults), ShouldEqual, 3)
							firstResult := testTask.LocalTestResults[0]
							So(firstResult.LogRaw, ShouldEqual, "")
							So(firstResult.LogId, ShouldNotEqual, "")

							testLog, err := model.FindOneTestLogById(firstResult.LogId)
							So(err, ShouldBeNil)
							So(testLog.Lines[0], ShouldEqual, testResults.Results[0].LogRaw)

							Convey("both URL and raw log should be stored appropriately if both exist", func() {
								urlResult := testTask.LocalTestResults[2]
								So(urlResult.LogRaw, ShouldEqual, "")
								So(urlResult.URL, ShouldNotEqual, "")
								So(urlResult.LogId, ShouldNotEqual, "")

								testLog, err := model.FindOneTestLogById(urlResult.LogId)
								So(err, ShouldBeNil)
								So(testLog.Lines[0], ShouldEqual, testResults.Results[2].LogRaw)
							})
						})
					})

				}
			}
		})
	})
}
