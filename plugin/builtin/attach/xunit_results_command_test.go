package attach_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/attach"
	"github.com/evergreen-ci/evergreen/plugin/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestAttachXUnitResults(t *testing.T) {
	resetTasks(t)
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(evergreen.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)
		configFile := "testdata/plugin_attach_xunit.yml"
		taskConfig, err := testutil.CreateTestConfig(configFile, t)
		util.HandleTestingErr(err, t, "failed to create test config: %v")
		taskConfig.WorkDir = "."
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
					task, err := model.FindTask(httpCom.TaskId)
					util.HandleTestingErr(err, t, "Couldn't find task")
					So(task, ShouldNotBeNil)
				}
			}

			Convey("and the tests should be present in the db", func() {
				task, err := model.FindTask("mocktaskid")
				So(err, ShouldBeNil)
				So(len(task.TestResults), ShouldNotEqual, 0)

				Convey("along with the proper logs", func() {
					tl1, err := model.FindOneTestLog(
						"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
						"mocktaskid",
						0,
					)
					So(err, ShouldBeNil)
					So(tl1, ShouldNotBeNil)
					So(tl1.Lines[0], ShouldContainSubstring, "SKIPPED")
					tl2, err := model.FindOneTestLog(
						"test.test_bson.TestBSON.test_basic_encode",
						"mocktaskid",
						0,
					)
					So(err, ShouldBeNil)
					So(tl2, ShouldNotBeNil)
					So(tl2.Lines[0], ShouldContainSubstring, "AssertionError")
				})
			})
		})
	})
}
