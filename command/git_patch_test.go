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
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestPatchPluginAPI(t *testing.T) {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestPatchPluginAPI")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)

	cwd := testutil.GetDirectoryOfFile()

	// skipping because, to work, this test requires that the
	// command work against a running service.
	// it really ought to be rewritten to test the mock rather
	// than the service playing the roll of a mock.
	SkipConvey("With a running api server and installed plugin", t, func() {
		configPath := filepath.Join(cwd, "testdata", "git", "plugin_patch.yml")
		patchFile := filepath.Join(cwd, "testdata", "git", "test.patch")

		testCommand := &gitFetchProject{Directory: "dir"}
		modelData, err := modelutil.SetupAPITestData(settings, "testTask", "testvar", configPath, modelutil.NoPatch)
		modelData.TaskConfig.Expansions = util.NewExpansions(settings.Credentials)

		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")
		err = setupTestPatchData(modelData, patchFile, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

		comm.PatchFiles[""] = patchFile

		patch := &patch.Patch{}

		Convey("calls to existing tasks with patches should succeed", func() {
			err = testCommand.getPatchContents(ctx, comm, logger, conf, patch)
			So(err, ShouldBeNil)
			So(patch, ShouldNotBeNil)
			testutil.HandleTestingErr(db.Clear(model.VersionCollection), t,
				"unable to clear versions collection")
		})
		Convey("calls to non-existing tasks should fail", func() {
			v := model.Version{Id: ""}
			testutil.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			modelData.Task = &task.Task{
				Id: "BAD_TASK_ID",
			}
			err := testCommand.getPatchContents(ctx, comm, logger, conf, patch)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
			So(patch, ShouldBeNil)
			testutil.HandleTestingErr(db.Clear(model.VersionCollection), t,
				"unable to clear versions collection")
		})
		Convey("calls to existing tasks without patches should fail", func() {
			noPatchTask := task.Task{Id: "noPatchTask", BuildId: "a"}
			testutil.HandleTestingErr(noPatchTask.Insert(), t, "Couldn't insert patch task")
			noPatchVersion := model.Version{Id: "noPatchVersion", BuildIds: []string{"a"}}
			testutil.HandleTestingErr(noPatchVersion.Insert(), t, "Couldn't insert patch version")
			v := model.Version{Id: ""}
			testutil.HandleTestingErr(v.Insert(), t, "Couldn't insert dummy version")
			modelData.Task = &noPatchTask

			err := testCommand.getPatchContents(ctx, comm, logger, conf, patch)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no patch found for task")
			So(patch, ShouldBeNil)
			testutil.HandleTestingErr(db.Clear(model.VersionCollection), t,
				"unable to clear versions collection")
		})

	})
}

func TestPatchPlugin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	settings := env.Settings()

	testutil.ConfigureIntegrationTest(t, settings, "TestPatchPlugin")
	cwd := testutil.GetDirectoryOfFile()
	jpm := env.JasperManager()

	Convey("With patch plugin installed into plugin registry", t, func() {
		testutil.HandleTestingErr(db.Clear(model.VersionCollection), t,
			"unable to clear versions collection")
		version := &model.Version{
			Id: "",
		}
		So(version.Insert(), ShouldBeNil)

		patchFile := filepath.Join(cwd, "testdata", "git", "testmodule.patch")
		configPath := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "git", "plugin_patch.yml")
		modelData, err := modelutil.SetupAPITestData(settings, "testtask1", "testvar", configPath, modelutil.InlinePatch)
		modelData.TaskConfig.Expansions = util.NewExpansions(settings.Credentials)
		testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

		err = setupTestPatchData(modelData, patchFile, t)
		testutil.HandleTestingErr(err, t, "Couldn't set up patch documents")

		taskConfig := modelData.TaskConfig

		comm := client.NewMock("http://localhost.com")
		logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: taskConfig.Task.Id, Secret: taskConfig.Task.Secret}, nil)
		So(err, ShouldBeNil)

		Convey("all commands in test project should execute successfully", func() {
			taskConfig.Task.Requester = evergreen.PatchVersionRequester

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmds, err := Render(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %s", command.Command)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)

					pluginCmds[0].SetJasperManager(jpm)
					err = pluginCmds[0].Execute(ctx, comm, logger, taskConfig)
					So(err, ShouldBeNil)
				}
			}
		})
	})
}

func TestGetPatchCommands(t *testing.T) {
	assert := assert.New(t)

	modulePatch := patch.ModulePatch{
		Githash: "a4aa03d0472d8503380479b76aef96c044182822",
		PatchSet: patch.PatchSet{
			Patch: "",
		},
	}

	cmds := getPatchCommands(modulePatch, "/teapot", "/tmp/bestest.patch")

	assert.Len(cmds, 5)
	assert.Equal("cd '/teapot'", cmds[3])
	assert.Equal("git reset --hard 'a4aa03d0472d8503380479b76aef96c044182822'", cmds[4])

	modulePatch.PatchSet.Patch = "bestest code"
	cmds = getPatchCommands(modulePatch, "/teapot", "/tmp/bestest.patch")
	assert.Len(cmds, 6)
	assert.Equal("git apply --stat '/tmp/bestest.patch' || true", cmds[5])
}
