package git_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/version"
	"10gen.com/mci/plugin"
	. "10gen.com/mci/plugin/builtin/git"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestPatchPluginAPI(t *testing.T) {
	testConfig := mci.TestConfig()
	Convey("With a running api server and installed plugin", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		util.HandleTestingErr(err, t, "Couldn't register patch plugin")
		server, err := apiserver.CreateTestServer(testConfig, nil, plugin.Published, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		taskConfig, _ := testutil.CreateTestConfig("testdata/plugin_patch.yml", t)
		testCommand := GitApplyPatchCommand{"dir"}
		_, _, err = testutil.SetupAPITestData("testTask", true, t)
		util.HandleTestingErr(err, t, "Couldn't set up test documents")
		testTask, err := model.FindTask("testTaskId")
		util.HandleTestingErr(err, t, "Couldn't set up test patch task")

		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("calls to existing tasks with patches should succeed", func() {
			httpCom := testutil.TestAgentCommunicator(testTask.Id, testTask.Secret, server.URL)
			pluginCom := &agent.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(taskConfig, pluginCom, logger)
			So(err, ShouldBeNil)
			So(patch, ShouldNotBeNil)
			util.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})
		Convey("calls to non-existing tasks should fail", func() {
			v := version.Version{Id: ""}
			util.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			httpCom := testutil.TestAgentCommunicator("BAD_TASK_ID", "", server.URL)
			pluginCom := &agent.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(taskConfig, pluginCom, logger)
			So(err.Error(), ShouldContainSubstring, "not found")
			So(err, ShouldNotBeNil)
			So(patch, ShouldBeNil)
			util.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})
		Convey("calls to existing tasks without patches should fail", func() {
			noPatchTask := model.Task{Id: "noPatchTask", BuildId: "a"}
			util.HandleTestingErr(noPatchTask.Insert(), t, "Couldn't insert patch task")
			noPatchVersion := version.Version{Id: "noPatchVersion", BuildIds: []string{"a"}}
			util.HandleTestingErr(noPatchVersion.Insert(), t, "Couldn't insert patch version")
			v := version.Version{Id: ""}
			util.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			httpCom := testutil.TestAgentCommunicator(noPatchTask.Id, "", server.URL)
			pluginCom := &agent.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(taskConfig, pluginCom, logger)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no patch found for task")
			So(patch, ShouldBeNil)
			util.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})

	})
}

func TestPatchPlugin(t *testing.T) {
	testConfig := mci.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	Convey("With patch plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin %v")
		util.HandleTestingErr(db.Clear(version.Collection), t,
			"unable to clear versions collection")
		version := &version.Version{
			Id: "",
		}
		So(version.Insert(), ShouldBeNil)
		server, err := apiserver.CreateTestServer(testConfig, nil, plugin.Published, false)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			taskConfig, _ := testutil.CreateTestConfig("testdata/plugin_patch.yml", t)
			taskConfig.Task.Requester = mci.PatchVersionRequester
			_, _, err = testutil.SetupAPITestData("testTask", true, t)
			util.HandleTestingErr(err, t, "Couldn't set up test documents")

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
				}
			}
		})
		Convey("broken test project should fail during execution", func() {
			// this config tries to patch on an empty repo
			taskConfig, _ := testutil.CreateTestConfig("testdata/plugin_broken_patch.yml", t)
			taskConfig.Task.Requester = mci.PatchVersionRequester
			_, _, err = testutil.SetupAPITestData("testTask", true, t)
			util.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
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
