package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/internal/testutil"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func reset(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, testlog.TestLogCollection))
}

func TestGotestPluginOnFailingTests(t *testing.T) {
	currentDirectory := testutil.GetDirectoryOfFile()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	testConfig := env.Settings()

	comm := client.NewMock("http://localhost.com")

	SkipConvey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)

		configPath := filepath.Join(currentDirectory, "testdata", "gotest", "bad.yml")
		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		require.NoError(t, err)
		conf, err := agentutil.MakeTaskConfigFromModelData(ctx, testConfig, modelData)
		require.NoError(t, err)
		logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
		So(err, ShouldBeNil)

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			require.NoError(t, err)
			conf.WorkDir = curWD

			for _, testTask := range conf.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
					pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
					require.NoError(t, err)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldEqual, "test failures")
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := task.FindOne(db.Query(task.ById(modelData.Task.Id)))
				So(err, ShouldBeNil)
				So(updatedTask, ShouldNotBeNil)
				So(len(updatedTask.LocalTestResults), ShouldEqual, 5)
				So(updatedTask.LocalTestResults[0].Status, ShouldEqual, "fail")
				So(updatedTask.LocalTestResults[1].Status, ShouldEqual, "fail")
				So(updatedTask.LocalTestResults[2].Status, ShouldEqual, "skip")
				So(updatedTask.LocalTestResults[3].Status, ShouldEqual, "pass")
				So(updatedTask.LocalTestResults[4].Status, ShouldEqual, "fail")

				Convey("with relevant logs present in the DB as well", func() {
					log, err := testlog.FindOneTestLog("0_badpkg", "testTaskId", 0)
					So(log, ShouldNotBeNil)
					So(err, ShouldBeNil)
					So(log.Lines[0], ShouldContainSubstring, "TestFail01")
				})
			})

		})
	})
}

func TestGotestPluginOnPassingTests(t *testing.T) {
	currentDirectory := testutil.GetDirectoryOfFile()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	SkipConvey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)
		testConfig := testutil.TestConfig()

		configPath := filepath.Join(currentDirectory, "testdata", "bad.yml")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		require.NoError(t, err)

		conf, err := agentutil.MakeTaskConfigFromModelData(ctx, testConfig, modelData)
		require.NoError(t, err)
		comm := client.NewMock("http://localhost.com")

		logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
		So(err, ShouldBeNil)

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			require.NoError(t, err)
			conf.WorkDir = curWD

			for _, testTask := range conf.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
					pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
					require.NoError(t, err)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)

					So(err, ShouldBeNil)
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := task.FindOne(db.Query(task.ById(modelData.Task.Id)))
				So(err, ShouldBeNil)
				So(updatedTask, ShouldNotBeNil)
				So(len(updatedTask.LocalTestResults), ShouldEqual, 2)
				So(updatedTask.LocalTestResults[0].Status, ShouldEqual, "pass")
				So(updatedTask.LocalTestResults[1].Status, ShouldEqual, "pass")
				So(updatedTask.LocalTestResults[0].TestName, ShouldEqual, "TestPass01")
				So(updatedTask.LocalTestResults[1].TestName, ShouldEqual, "TestPass02")
				So(updatedTask.LocalTestResults[0].TestStartTime, ShouldBeLessThan,
					updatedTask.LocalTestResults[0].TestEndTime)
				So(updatedTask.LocalTestResults[1].TestStartTime, ShouldBeLessThan,
					updatedTask.LocalTestResults[1].TestEndTime)

				Convey("with relevant logs present in the DB as well", func() {
					log, err := testlog.FindOneTestLog("0_goodpkg", "testTaskId", 0)
					So(log, ShouldNotBeNil)
					So(err, ShouldBeNil)
					So(log.Lines[0], ShouldContainSubstring, "TestPass01")
				})

			})
		})
	})
}
