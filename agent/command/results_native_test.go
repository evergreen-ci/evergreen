package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/internal/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func resetTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, testlog.TestLogCollection))
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
		require.NoError(t, err)
		So(err, ShouldBeNil)

		conf, err := agentutil.MakeTaskConfigFromModelData(ctx, testConfig, modelData)
		require.NoError(t, err)
		conf.WorkDir = "."

		Convey("all commands in test project should execute successfully", func() {
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
			So(err, ShouldBeNil)

			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
					require.NoError(t, err)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(db.Query(task.ById(conf.Task.Id)))
					require.NoError(t, err)
					So(testTask, ShouldNotBeNil)

					// ensure test results are exactly as expected
					// attempt to open the file
					reportFile, err := os.Open(resultsLoc)
					require.NoError(t, err)
					var nativeResults nativeTestResults
					require.NoError(t, utility.ReadJSON(reportFile, &nativeResults))
					results := make([]testresult.TestResult, len(nativeResults.Results))
					for i, nativeResult := range nativeResults.Results {
						results[i] = nativeResult.convertToService()
					}
					So(testTask.LocalTestResults, ShouldResemble, results)
					require.NoError(t, err)
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
		require.NoError(t, err)

		conf, err := agentutil.MakeTaskConfigFromModelData(ctx, testConfig, modelData)
		require.NoError(t, err)
		conf.WorkDir = "."
		logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
		So(err, ShouldBeNil)

		Convey("when attaching a raw log ", func() {
			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {

					pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
					require.NoError(t, err)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					// create a plugin communicator

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					Convey("when retrieving task", func() {
						// fetch the task
						testTask, err := task.FindOne(db.Query(task.ById(conf.Task.Id)))
						require.NoError(t, err)
						So(testTask, ShouldNotBeNil)

						Convey("test results should match and raw log should be in appropriate collection", func() {

							reportFile, err := os.Open(resultsLoc)
							require.NoError(t, err)
							var nativeResults nativeTestResults
							require.NoError(t, utility.ReadJSON(reportFile, &nativeResults))
							results := make([]testresult.TestResult, len(nativeResults.Results))
							for i, nativeResult := range nativeResults.Results {
								results[i] = nativeResult.convertToService()
							}

							So(len(results), ShouldEqual, 3)
							So(len(testTask.LocalTestResults), ShouldEqual, 3)
							firstResult := testTask.LocalTestResults[0]
							So(firstResult.RawLogURL, ShouldEqual, "")

							Convey("both URL and raw log should be stored appropriately if both exist", func() {
								urlResult := testTask.LocalTestResults[2]
								So(urlResult.RawLogURL, ShouldEqual, "")
								So(urlResult.LogURL, ShouldNotEqual, "")
							})
						})
					})

				}
			}
		})
	})
}
