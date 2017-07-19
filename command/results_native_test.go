package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func resetTasks(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestAttachResults(t *testing.T) {
	resetTasks(t)
	testConfig := testutil.TestConfig()
	cwd := testutil.GetDirectoryOfFile()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := client.NewMock("http://localhost.com")

	SkipConvey("With attachResults plugin installed into plugin registry", t, func() {

		configFile := filepath.Join(cwd, "testdata", "attach", "plugin_attach_results.yml")
		resultsLoc := filepath.Join(cwd, "testdata", "attach", "plugin_attach_results.json")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configFile, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")
		So(err, ShouldBeNil)
		modelData.TaskConfig.WorkDir = "."

		Convey("all commands in test project should execute successfully", func() {
			conf := modelData.TaskConfig
			logger := comm.GetLoggerProducer(client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := Render(command, conf.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(task.ById(conf.Task.Id))
					testutil.HandleTestingErr(err, t, "Couldn't find task")
					So(testTask, ShouldNotBeNil)
					// ensure test results are exactly as expected
					// attempt to open the file
					reportFile, err := os.Open(resultsLoc)
					testutil.HandleTestingErr(err, t, "Couldn't open report file: '%v'", err)
					results := &task.TestResults{}
					err = util.ReadJSONInto(reportFile, results)
					testutil.HandleTestingErr(err, t, "Couldn't read report file: '%v'", err)
					testResults := *results
					So(testTask.TestResults, ShouldResemble, testResults.Results)
					testutil.HandleTestingErr(err, t, "Couldn't clean up test temp dir")
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
		testutil.HandleTestingErr(err, t, "failed to setup test data")

		modelData.TaskConfig.WorkDir = "."
		conf := modelData.TaskConfig
		logger := comm.GetLoggerProducer(client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

		Convey("when attaching a raw log ", func() {
			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {

					pluginCmds, err := Render(command, conf.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					// create a plugin communicator

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					Convey("when retrieving task", func() {
						// fetch the task
						testTask, err := task.FindOne(task.ById(conf.Task.Id))
						testutil.HandleTestingErr(err, t, "Couldn't find task")
						So(testTask, ShouldNotBeNil)

						Convey("test results should match and raw log should be in appropriate collection", func() {

							reportFile, err := os.Open(resultsLoc)
							testutil.HandleTestingErr(err, t, "Couldn't open report file: '%v'", err)
							results := &task.TestResults{}
							err = util.ReadJSONInto(reportFile, results)
							testutil.HandleTestingErr(err, t, "Couldn't read report file: '%v'", err)

							testResults := *results
							So(len(testResults.Results), ShouldEqual, 3)
							So(len(testTask.TestResults), ShouldEqual, 3)
							firstResult := testTask.TestResults[0]
							So(firstResult.LogRaw, ShouldEqual, "")
							So(firstResult.LogId, ShouldNotEqual, "")

							testLog, err := model.FindOneTestLogById(firstResult.LogId)
							So(err, ShouldBeNil)
							So(testLog.Lines[0], ShouldEqual, testResults.Results[0].LogRaw)

							Convey("both URL and raw log should be stored appropriately if both exist", func() {
								urlResult := testTask.TestResults[2]
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
