package attach_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/attach"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func resetTasks(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestAttachResults(t *testing.T) {
	resetTasks(t)
	testConfig := evergreen.TestConfig()
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)
		configFile := "testdata/plugin_attach_results.yml"
		resultsLoc := "testdata/plugin_attach_results.json"
		taskConfig, err := plugintest.CreateTestConfig(configFile, t)
		testutil.HandleTestingErr(err, t, "failed to create test config: %v")
		taskConfig.WorkDir = "."
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			for _, projTask := range taskConfig.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(task.ById(httpCom.TaskId))
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
	testConfig := evergreen.TestConfig()
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)
		configFile := "testdata/plugin_attach_results_raw.yml"
		resultsLoc := "testdata/plugin_attach_results_raw.json"
		taskConfig, err := plugintest.CreateTestConfig(configFile, t)
		testutil.HandleTestingErr(err, t, "failed to create test config: %v")
		taskConfig.WorkDir = "."
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("when attaching a raw log ", func() {
			for _, projTask := range taskConfig.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {

					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					// create a plugin communicator
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
					Convey("when retrieving task", func() {
						// fetch the task
						testTask, err := task.FindOne(task.ById(httpCom.TaskId))
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
