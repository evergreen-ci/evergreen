package git_test

import (
	"testing"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/git"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPatchPluginAPI(t *testing.T) {
	testConfig := evergreen.TestConfig()
	Convey("With a running api server and installed plugin", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register patch plugin")
		server, err := apiserver.CreateTestServer(testConfig, nil, plugin.APIPlugins, false)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		taskConfig, _ := plugintest.CreateTestConfig("testdata/plugin_patch.yml", t)
		testCommand := GitApplyPatchCommand{"dir"}
		_, _, err = plugintest.SetupAPITestData("testTask", true, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")
		testTask, err := task.FindOne(task.ById("testTaskId"))
		testutil.HandleTestingErr(err, t, "Couldn't set up test patch task")

		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("calls to existing tasks with patches should succeed", func() {
			httpCom := plugintest.TestAgentCommunicator(testTask.Id, testTask.Secret, server.URL)
			pluginCom := &agent.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(taskConfig, pluginCom, logger)
			So(err, ShouldBeNil)
			So(patch, ShouldNotBeNil)
			testutil.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})
		Convey("calls to non-existing tasks should fail", func() {
			v := version.Version{Id: ""}
			testutil.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			httpCom := plugintest.TestAgentCommunicator("BAD_TASK_ID", "", server.URL)
			pluginCom := &agent.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(taskConfig, pluginCom, logger)
			So(err.Error(), ShouldContainSubstring, "not found")
			So(err, ShouldNotBeNil)
			So(patch, ShouldBeNil)
			testutil.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})
		Convey("calls to existing tasks without patches should fail", func() {
			noPatchTask := task.Task{Id: "noPatchTask", BuildId: "a"}
			testutil.HandleTestingErr(noPatchTask.Insert(), t, "Couldn't insert patch task")
			noPatchVersion := version.Version{Id: "noPatchVersion", BuildIds: []string{"a"}}
			testutil.HandleTestingErr(noPatchVersion.Insert(), t, "Couldn't insert patch version")
			v := version.Version{Id: ""}
			testutil.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			httpCom := plugintest.TestAgentCommunicator(noPatchTask.Id, "", server.URL)
			pluginCom := &agent.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(taskConfig, pluginCom, logger)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no patch found for task")
			So(patch, ShouldBeNil)
			testutil.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})

	})
}

func TestPatchPlugin(t *testing.T) {
	testConfig := evergreen.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	Convey("With patch plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin %v")
		testutil.HandleTestingErr(db.Clear(version.Collection), t,
			"unable to clear versions collection")
		version := &version.Version{
			Id: "",
		}
		So(version.Insert(), ShouldBeNil)
		server, err := apiserver.CreateTestServer(testConfig, nil, plugin.APIPlugins, false)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		//sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		sliceAppender := slogger.StdOutAppender()
		logger := agent.NewTestLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			taskConfig, _ := plugintest.CreateTestConfig("testdata/plugin_patch.yml", t)
			taskConfig.Task.Requester = evergreen.PatchVersionRequester
			_, _, err = plugintest.SetupAPITestData("testTask", true, t)
			testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

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
				}
			}
		})
		Convey("broken test project should fail during execution", func() {
			// this config tries to patch on an empty repo
			taskConfig, _ := plugintest.CreateTestConfig("testdata/plugin_broken_patch.yml", t)
			taskConfig.Task.Requester = evergreen.PatchVersionRequester
			_, _, err = plugintest.SetupAPITestData("testTask", true, t)
			testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldNotBeNil)
				}
			}
		})
	})
}
