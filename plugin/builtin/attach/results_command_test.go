package attach_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	. "10gen.com/mci/plugin/builtin/attach"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func resetTasks(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mci.TestConfig()))
	util.HandleTestingErr(
		db.ClearCollections(model.TasksCollection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestAttachResults(t *testing.T) {
	resetTasks(t)
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimplePluginRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)
		configFile := "testdata/plugin_attach_results.yml"
		resultsLoc := "testdata/plugin_attach_results.json"
		taskConfig, err := testutil.CreateTestConfig(configFile, t)
		util.HandleTestingErr(err, t, "failed to create test config: %v")
		taskConfig.WorkDir = "."
		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommand(command,
						taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(plugin, ShouldNotBeNil)
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(), httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
					task, err := model.FindTask(httpCom.TaskId)
					util.HandleTestingErr(err, t, "Couldn't find task")
					So(task, ShouldNotBeNil)
					// ensure test results are exactly as expected
					// attempt to open the file
					reportFile, err := os.Open(resultsLoc)
					util.HandleTestingErr(err, t, "Couldn't open report file: '%v'", err)
					results := &model.TestResults{}
					err = util.ReadJSONInto(reportFile, results)
					util.HandleTestingErr(err, t, "Couldn't read report file: '%v'", err)
					testResults := *results
					So(task.TestResults, ShouldResemble, testResults.Results)
					util.HandleTestingErr(err, t, "Couldn't clean up test temp dir")
				}
			}
		})
	})
}
