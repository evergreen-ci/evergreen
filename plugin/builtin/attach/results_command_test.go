package attach_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
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
		db.ClearCollections(model.TasksCollection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestAttachResults(t *testing.T) {
	resetTasks(t)
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(evergreen.TestConfig(), nil, plugin.Published, true)
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
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
					task, err := model.FindTask(httpCom.TaskId)
					testutil.HandleTestingErr(err, t, "Couldn't find task")
					So(task, ShouldNotBeNil)
					// ensure test results are exactly as expected
					// attempt to open the file
					reportFile, err := os.Open(resultsLoc)
					testutil.HandleTestingErr(err, t, "Couldn't open report file: '%v'", err)
					results := &model.TestResults{}
					err = util.ReadJSONInto(reportFile, results)
					testutil.HandleTestingErr(err, t, "Couldn't read report file: '%v'", err)
					testResults := *results
					So(task.TestResults, ShouldResemble, testResults.Results)
					testutil.HandleTestingErr(err, t, "Couldn't clean up test temp dir")
				}
			}
		})
	})
}
