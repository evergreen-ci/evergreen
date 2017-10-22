package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func reset(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestGotestPluginOnFailingTests(t *testing.T) {
	currentDirectory := testutil.GetDirectoryOfFile()
	testConfig := testutil.TestConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")

	SkipConvey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)

		configPath := filepath.Join(currentDirectory, "testdata", "gotest", "bad.yml")
		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")
		conf := modelData.TaskConfig
		logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
			conf.WorkDir = curWD

			for _, testTask := range conf.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
					pluginCmds, err := Render(command, conf.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldEqual, "test failures")
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := task.FindOne(task.ById(modelData.Task.Id))
				So(err, ShouldBeNil)
				So(updatedTask, ShouldNotBeNil)
				So(len(updatedTask.TestResults), ShouldEqual, 5)
				So(updatedTask.TestResults[0].Status, ShouldEqual, "fail")
				So(updatedTask.TestResults[1].Status, ShouldEqual, "fail")
				So(updatedTask.TestResults[2].Status, ShouldEqual, "skip")
				So(updatedTask.TestResults[3].Status, ShouldEqual, "pass")
				So(updatedTask.TestResults[4].Status, ShouldEqual, "fail")

				Convey("with relevant logs present in the DB as well", func() {
					log, err := model.FindOneTestLog("0_badpkg", "testTaskId", 0)
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
		testutil.ConfigureIntegrationTest(t, testConfig, "TestGotestPluginOnPassingTests")

		configPath := filepath.Join(currentDirectory, "testdata", "bad.yml")

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")

		conf := modelData.TaskConfig
		comm := client.NewMock("http://localhost.com")
		logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
			conf.WorkDir = curWD

			for _, testTask := range conf.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
					pluginCmds, err := Render(command, conf.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)

					So(err, ShouldBeNil)
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := task.FindOne(task.ById(modelData.Task.Id))
				So(err, ShouldBeNil)
				So(updatedTask, ShouldNotBeNil)
				So(len(updatedTask.TestResults), ShouldEqual, 2)
				So(updatedTask.TestResults[0].Status, ShouldEqual, "pass")
				So(updatedTask.TestResults[1].Status, ShouldEqual, "pass")
				So(updatedTask.TestResults[0].TestFile, ShouldEqual, "TestPass01")
				So(updatedTask.TestResults[1].TestFile, ShouldEqual, "TestPass02")
				So(updatedTask.TestResults[0].StartTime, ShouldBeLessThan,
					updatedTask.TestResults[0].EndTime)
				So(updatedTask.TestResults[1].StartTime, ShouldBeLessThan,
					updatedTask.TestResults[1].EndTime)

				Convey("with relevant logs present in the DB as well", func() {
					log, err := model.FindOneTestLog("0_goodpkg", "testTaskId", 0)
					So(log, ShouldNotBeNil)
					So(err, ShouldBeNil)
					So(log.Lines[0], ShouldContainSubstring, "TestPass01")
				})

			})
		})
	})
}
