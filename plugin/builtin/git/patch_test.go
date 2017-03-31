package git

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPatchPluginAPI(t *testing.T) {
	testConfig := testutil.TestConfig()
	cwd := testutil.GetDirectoryOfFile()
	Convey("With a running api server and installed plugin", t, func() {
		registry := plugin.NewSimpleRegistry()
		gitPlugin := &GitPlugin{}
		err := registry.Register(gitPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register patch plugin")
		server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		configPath := filepath.Join(cwd, "testdata", "plugin_patch.yml")
		patchFile := filepath.Join(cwd, "testdata", "test.patch")

		testCommand := GitGetProjectCommand{Directory: "dir"}
		modelData, err := modelutil.SetupAPITestData(testConfig, "testTask", "testvar", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")
		err = plugintest.SetupPatchData(modelData, patchFile, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")
		taskConfig := modelData.TaskConfig

		logger := agentutil.NewTestLogger(slogger.StdOutAppender())

		Convey("calls to existing tasks with patches should succeed", func() {
			httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)
			pluginCom := &comm.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(pluginCom, logger)
			So(err, ShouldBeNil)
			So(patch, ShouldNotBeNil)
			testutil.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})
		Convey("calls to non-existing tasks should fail", func() {
			v := version.Version{Id: ""}
			testutil.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			modelData.Task = &task.Task{
				Id: "BAD_TASK_ID",
			}
			httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)
			pluginCom := &comm.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(pluginCom, logger)
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
			modelData.Task = &noPatchTask
			httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)
			pluginCom := &comm.TaskJSONCommunicator{gitPlugin.Name(), httpCom}
			patch, err := testCommand.GetPatch(pluginCom, logger)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no patch found for task")
			So(patch, ShouldBeNil)
			testutil.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})

	})
}

func TestPatchPlugin(t *testing.T) {
	cwd := testutil.GetDirectoryOfFile()
	testConfig := testutil.TestConfig()
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
		server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		defer server.Close()

		patchFile := filepath.Join(cwd, "testdata", "testmodule.patch")
		configPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "plugin_patch.yml")
		modelData, err := modelutil.SetupAPITestData(testConfig, "testTask", "testvar", configPath, modelutil.InlinePatch)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

		err = plugintest.SetupPatchData(modelData, patchFile, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up patch documents")

		taskConfig := modelData.TaskConfig
		httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)

		logger := agentutil.NewTestLogger(slogger.StdOutAppender())

		Convey("all commands in test project should execute successfully", func() {
			taskConfig.Task.Requester = evergreen.PatchVersionRequester

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &comm.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
				}
			}
		})
	})
}

func TestGetPatchCommands(t *testing.T) {
	Convey("With a patch that has modules", t, func() {
		testPatch := patch.Patch{
			Patches: []patch.ModulePatch{
				patch.ModulePatch{
					ModuleName: "",
					PatchSet: patch.PatchSet{
						Patch: "",
					},
				},
				patch.ModulePatch{
					ModuleName: "anotherOne",
					PatchSet: patch.PatchSet{
						Patch: "these are words",
					},
				},
			},
		}

		Convey("on an empty patch module, a set of commands that does not apply the patch should be returned", func() {
			commands := GetPatchCommands(testPatch.Patches[0], "", "")
			So(len(commands), ShouldEqual, 5)
		})
		Convey("on a patch with content, the set of commands should apply the patch", func() {
			commands := GetPatchCommands(testPatch.Patches[1], "", "")
			So(len(commands), ShouldEqual, 8)
		})
	})
}
