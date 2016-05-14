package gotest_test

import (
	"os"
	"testing"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/gotest"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func reset(t *testing.T) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
	testutil.HandleTestingErr(
		db.ClearCollections(task.Collection, model.TestLogCollection), t,
		"error clearing test collections")
}

func TestGotestPluginOnFailingTests(t *testing.T) {
	testConfig := evergreen.TestConfig()
	SkipConvey("With gotest plugin installed into plugin registry", t, func() {
		reset(t)

		registry := plugin.NewSimpleRegistry()
		testPlugin := &GotestPlugin{}
		err := registry.Register(testPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin %v")

		server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
			taskConfig, err := plugintest.CreateTestConfig("testdata/bad.yml", t)
			// manually override working dirctory to the main repo, since this
			// is much easier than copying over the required testing dependencies
			// to a temporary directory
			testutil.HandleTestingErr(err, t, "Couldn't set up test config %v")
			taskConfig.WorkDir = curWD
			pluginTask, _, err := plugintest.SetupAPITestData("testTask", false, t)
			testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, testTask := range taskConfig.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldEqual, "test failures")
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := task.FindOne(task.ById(pluginTask.Id))
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
		testConfig := evergreen.TestConfig()
		testutil.ConfigureIntegrationTest(t, testConfig, "TestGotestPluginOnPassingTests")
		registry := plugin.NewSimpleRegistry()
		testPlugin := &GotestPlugin{}
		err := registry.Register(testPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin %v")

		server, err := service.CreateTestServer(evergreen.TestConfig(), nil, plugin.APIPlugins, true)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
			curWD, err := os.Getwd()
			testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
			taskConfig, err := plugintest.CreateTestConfig("testdata/good.yml", t)
			// manually override working directory to the main repo, since this
			// is much easier than copying over the required testing dependencies
			// to a temporary directory
			testutil.HandleTestingErr(err, t, "Couldn't set up test config %v")
			taskConfig.WorkDir = curWD
			pluginTask, _, err := plugintest.SetupAPITestData("testTask", false, t)
			testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, testTask := range taskConfig.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &agent.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))

					So(err, ShouldBeNil)
				}
			}

			Convey("and the tests in the task should be updated", func() {
				updatedTask, err := task.FindOne(task.ById(pluginTask.Id))
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
		testConfig := evergreen.TestConfig()
		testutil.ConfigureIntegrationTest(t, testConfig, "TestGotestPluginWithEnvironmentVariables")
		registry := plugin.NewSimpleRegistry()
		testPlugin := &GotestPlugin{}
		err := registry.Register(testPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin %v")

		server, err := service.CreateTestServer(testConfig, nil, plugin.APIPlugins, true)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := plugintest.TestAgentCommunicator("testTaskId", "testTaskSecret", server.URL)

		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("test command should get a copy of custom environment variables", func() {
			curWD, err := os.Getwd()
			testutil.HandleTestingErr(err, t, "Couldn't get working directory: %v")
			taskConfig, err := plugintest.CreateTestConfig("testdata/env.yml", t)
			// manually override working directory to the main repo, since this
			// is much easier than copying over the required testing dependencies
			// to a temporary directory
			testutil.HandleTestingErr(err, t, "Couldn't set up test config %v")
			taskConfig.WorkDir = curWD
			_, _, err = plugintest.SetupAPITestData("testTask", false, t)
			testutil.HandleTestingErr(err, t, "Couldn't set up test documents")

			for _, testTask := range taskConfig.Project.Tasks {
				So(len(testTask.Commands), ShouldNotEqual, 0)
				for _, command := range testTask.Commands {
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
	})
}
