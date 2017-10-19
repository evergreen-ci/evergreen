package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPatchPluginAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	testConfig := testutil.TestConfig()
	cwd := testutil.GetDirectoryOfFile()

	// skipping because, to work, this test requires that the
	// command work against a running service.
	// it really ought to be rewritten to test the mock rather
	// than the service playing the roll of a mock.
	SkipConvey("With a running api server and installed plugin", t, func() {
		configPath := filepath.Join(cwd, "testdata", "git", "plugin_patch.yml")
		patchFile := filepath.Join(cwd, "testdata", "git", "test.patch")

		testCommand := &gitFetchProject{Directory: "dir"}
		modelData, err := modelutil.SetupAPITestData(testConfig, "testTask", "testvar", configPath, modelutil.NoPatch)

		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")
		err = plugintest.SetupPatchData(modelData, patchFile, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

		comm.PatchFiles[""] = patchFile

		patch := &patch.Patch{}

		Convey("calls to existing tasks with patches should succeed", func() {
			err = testCommand.getPatchContents(ctx, comm, logger, conf, patch)
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
			err := testCommand.getPatchContents(ctx, comm, logger, conf, patch)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
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

			err := testCommand.getPatchContents(ctx, comm, logger, conf, patch)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no patch found for task")
			So(patch, ShouldBeNil)
			testutil.HandleTestingErr(db.Clear(version.Collection), t,
				"unable to clear versions collection")
		})

	})
}

func TestPatchPlugin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cwd := testutil.GetDirectoryOfFile()
	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	Convey("With patch plugin installed into plugin registry", t, func() {
		testutil.HandleTestingErr(db.Clear(version.Collection), t,
			"unable to clear versions collection")
		version := &version.Version{
			Id: "",
		}
		So(version.Insert(), ShouldBeNil)

		patchFile := filepath.Join(cwd, "testdata", "git", "testmodule.patch")
		configPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_patch.yml")
		modelData, err := modelutil.SetupAPITestData(testConfig, "testTask", "testvar", configPath, modelutil.InlinePatch)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

		err = plugintest.SetupPatchData(modelData, patchFile, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up patch documents")

		taskConfig := modelData.TaskConfig

		comm := client.NewMock("http://localhost.com")
		logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: taskConfig.Task.Id, Secret: taskConfig.Task.Secret})

		Convey("all commands in test project should execute successfully", func() {
			taskConfig.Task.Requester = evergreen.PatchVersionRequester

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := Render(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)

					err = pluginCmds[0].Execute(ctx, comm, logger, taskConfig)
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
			commands := getPatchCommands(testPatch.Patches[0], "", "")
			So(len(commands), ShouldEqual, 5)
		})
		Convey("on a patch with content, the set of commands should apply the patch", func() {
			commands := getPatchCommands(testPatch.Patches[1], "", "")
			So(len(commands), ShouldEqual, 8)
		})
	})
}
