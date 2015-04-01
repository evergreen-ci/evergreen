package gotest_test

import (
	"10gen.com/mci"
	"10gen.com/mci/agent"
	"10gen.com/mci/apiserver"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/plugin"
	. "10gen.com/mci/plugin/builtin/gotest"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	"github.com/10gen-labs/slogger/v1"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func reset(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(mci.TestConfig()))
	util.HandleTestingErr(
		db.ClearCollections(model.TasksCollection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestGotestPluginOnFailingTests(t *testing.T) {
	SkipConvey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)

		registry := plugin.NewSimpleRegistry()
		testPlugin := &GotestPlugin{}
		err := registry.Register(testPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin %v")

		server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			util.HandleTestingErr(err, t, "Couldn't get working directory")
			taskConfig, err := testutil.CreateTestConfig("testdata/bad.yml", t)
			// manually override working dirctory to the main repo, since this
			// is much easier than copying over the required testing dependencies
			// to a temporary directory
			util.HandleTestingErr(err, t, "Couldn't set up test config %v")
			taskConfig.WorkDir = curWD
			task, _, err := testutil.SetupAPITestData("testTask", false, t)
			util.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(plugin, ShouldNotBeNil)
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(), httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldEqual, "test failures")
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := model.FindTask(task.Id)
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
	SkipConvey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)

		registry := plugin.NewSimpleRegistry()
		testPlugin := &GotestPlugin{}
		err := registry.Register(testPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin %v")

		server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			util.HandleTestingErr(err, t, "Couldn't get working directory")
			taskConfig, err := testutil.CreateTestConfig("testdata/good.yml", t)
			// manually override working directory to the main repo, since this
			// is much easier than copying over the required testing dependencies
			// to a temporary directory
			util.HandleTestingErr(err, t, "Couldn't set up test config %v")
			taskConfig.WorkDir = curWD
			task, _, err := testutil.SetupAPITestData("testTask", false, t)
			util.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(plugin, ShouldNotBeNil)
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(), httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig, make(chan bool))

					So(err, ShouldBeNil)
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := model.FindTask(task.Id)
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

func TestGotestPluginWithEnvironmentVariables(t *testing.T) {
	Convey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)

		registry := plugin.NewSimpleRegistry()
		testPlugin := &GotestPlugin{}
		err := registry.Register(testPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin %v")

		server, err := apiserver.CreateTestServer(mci.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &mci.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestAgentLogger(sliceAppender)

		Convey("test command should get a copy of custom environment variables", func() {
			curWD, err := os.Getwd()
			util.HandleTestingErr(err, t, "Couldn't get working directory")
			taskConfig, err := testutil.CreateTestConfig("testdata/env.yml", t)
			// manually override working directory to the main repo, since this
			// is much easier than copying over the required testing dependencies
			// to a temporary directory
			util.HandleTestingErr(err, t, "Couldn't set up test config %v")
			taskConfig.WorkDir = curWD
			_, _, err = testutil.SetupAPITestData("testTask", false, t)
			util.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, task := range taskConfig.Project.Tasks {
				So(len(task.Commands), ShouldNotEqual, 0)
				for _, command := range task.Commands {
					pluginCmd, plugin, err := registry.GetCommands(command, taskConfig.Project.Functions)
					util.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(plugin, ShouldNotBeNil)
					So(pluginCmd, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{plugin.Name(), httpCom}
					err = pluginCmd.Execute(logger, pluginCom, taskConfig, make(chan bool))

					So(err, ShouldBeNil)
				}
			}
		})
	})
}
