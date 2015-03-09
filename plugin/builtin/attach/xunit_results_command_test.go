package attach_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	. "10gen.com/mci/plugin/builtin/attach"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestAttachXUnitResults(t *testing.T) {
	resetTasks(t)
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimplePluginRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)
		configFile := "testdata/plugin_attach_xunit.yml"
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
