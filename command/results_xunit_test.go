package command

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/comm"
	agentutil "github.com/evergreen-ci/evergreen/agent/testutil"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/plugin/plugintest"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/slogger"
	. "github.com/smartystreets/goconvey/convey"
)

const TotalResultCount = 677

var (
	workingDirectory = testutil.GetDirectoryOfFile()
	SingleFileConfig = filepath.Join(workingDirectory, "testdata", "plugin_attach_xunit.yml")
	WildcardConfig   = filepath.Join(workingDirectory, "testdata", "plugin_attach_xunit_wildcard.yml")
)

// runTest abstracts away common tests and setup between all attach xunit tests.
// It also takes as an argument a function which runs any additional tests desired.
func runTest(t *testing.T, configPath string, customTests func(string)) {
	resetTasks(t)
	testConfig := testutil.TestConfig()
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		testutil.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := service.CreateTestServer(testConfig, nil)
		testutil.HandleTestingErr(err, t, "Couldn't set up testing server")
		defer server.Close()

		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")

		httpCom := plugintest.TestAgentCommunicator(modelData, server.URL)

		taskConfig := modelData.TaskConfig
		taskConfig.WorkDir = "."
		logger := agentutil.NewTestLogger(slogger.StdOutAppender())

		Convey("all commands in test project should execute successfully", func() {
			for _, projTask := range taskConfig.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := registry.GetCommands(command, taskConfig.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					pluginCom := &comm.TaskJSONCommunicator{pluginCmds[0].Plugin(), httpCom}
					err = pluginCmds[0].Execute(logger, pluginCom, taskConfig, make(chan bool))
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(task.ById(httpCom.TaskId))
					testutil.HandleTestingErr(err, t, "Couldn't find task")
					So(testTask, ShouldNotBeNil)
				}
			}

			Convey("and the tests should be present in the db", func() {
				customTests(modelData.Task.Id)
			})
		})
	})
}

// dBTests are the database verification tests for standard one file execution
func dBTests(taskId string) {
	task, err := task.FindOne(task.ById(taskId))
	So(err, ShouldBeNil)
	So(task, ShouldNotBeNil)
	So(len(task.TestResults), ShouldNotEqual, 0)

	Convey("along with the proper logs", func() {
		// junit_3.xml
		tl := dBFindOneTestLog(
			"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
			taskId,
		)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")
		tl = dBFindOneTestLog("test.test_bson.TestBSON.test_basic_encode", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "AssertionError")
	})
}

// dBTestsWildcard are the database verification tests for globbed file execution
func dBTestsWildcard(taskId string) {
	task, err := task.FindOne(task.ById(taskId))
	So(err, ShouldBeNil)
	So(len(task.TestResults), ShouldEqual, TotalResultCount)

	Convey("along with the proper logs", func() {
		// junit_1.xml
		tl := dBFindOneTestLog("pkg1.test.test_things.test_params_func_2", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "FAILURE")
		So(tl.Lines[6], ShouldContainSubstring, "AssertionError")
		tl = dBFindOneTestLog("pkg1.test.test_things.SomeTests.test_skippy", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")

		// junit_2.xml
		tl = dBFindOneTestLog("tests.ATest.fail", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "FAILURE")
		So(tl.Lines[1], ShouldContainSubstring, "AssertionFailedError")

		// junit_3.xml
		tl = dBFindOneTestLog(
			"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
			taskId,
		)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")
		tl = dBFindOneTestLog("test.test_bson.TestBSON.test_basic_encode", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "AssertionError")
	})
}

// dBFindOneTestLog abstracts away some of the common attributes of database
// verification tests.
func dBFindOneTestLog(name, taskId string) *model.TestLog {
	ret, err := model.FindOneTestLog(
		name,
		taskId,
		0,
	)
	So(err, ShouldBeNil)
	So(ret, ShouldNotBeNil)
	return ret
}

func TestAttachXUnitResults(t *testing.T) {
	runTest(t, SingleFileConfig, dBTests)
}

func TestAttachXUnitWildcardResults(t *testing.T) {
	runTest(t, WildcardConfig, dBTestsWildcard)
}
