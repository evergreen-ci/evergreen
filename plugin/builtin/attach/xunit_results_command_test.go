package attach_test

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apiserver"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	. "github.com/evergreen-ci/evergreen/plugin/builtin/attach"
	"github.com/evergreen-ci/evergreen/plugin/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

const (
	TotalResultCount = 676
)

func TestAttachXUnitWildcardResults(t *testing.T) {
	runTest(t, dBTestsWildcard)
	return
}

func TestAttachXUnitResults(t *testing.T) {
	runTest(t, dBTests)
	return
}

// RunTest abstracts away common tests and setup between all attach xunit tests.
// It also takes as an argument a function which runs any additional tests desired.
func runTest(t *testing.T, customTests func()) {
	resetTasks(t)
	Convey("With attachResults plugin installed into plugin registry", t, func() {
		registry := plugin.NewSimpleRegistry()
		attachPlugin := &AttachPlugin{}
		err := registry.Register(attachPlugin)
		util.HandleTestingErr(err, t, "Couldn't register plugin: %v")

		server, err := apiserver.CreateTestServer(evergreen.TestConfig(), nil, plugin.Published, true)
		util.HandleTestingErr(err, t, "Couldn't set up testing server")
		httpCom := testutil.TestAgentCommunicator("mocktaskid", "mocktasksecret", server.URL)
		configFile := "testdata/plugin_attach_xunit.yml"
		taskConfig, err := testutil.CreateTestConfig(configFile, t)
		util.HandleTestingErr(err, t, "failed to create test config: %v")
		taskConfig.WorkDir = "."
		sliceAppender := &evergreen.SliceAppender{[]*slogger.Log{}}
		logger := agent.NewTestLogger(sliceAppender)

		Convey("all commands in test project should execute successfully", func() {
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
					task, err := model.FindTask(httpCom.TaskId)
					util.HandleTestingErr(err, t, "Couldn't find task")
					So(task, ShouldNotBeNil)
				}
			}
			Convey("and the tests should be present in the db", customTests)
		})
	})
	return
}

// DBTests are the database verification tests for standard one file execution
func dBTests() {
	task, err := model.FindTask("mocktaskid")
	So(err, ShouldBeNil)
	So(len(task.TestResults), ShouldNotEqual, 0)

	Convey("along with the proper logs", func() {
		// junit_3.xml
		tl := dBFindOneTestLog(
			"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
		)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")
		tl = dBFindOneTestLog("test.test_bson.TestBSON.test_basic_encode")
		So(tl.Lines[0], ShouldContainSubstring, "AssertionError")
	})
	return
}

// DBTestsWildcard are the database verification tests for globbed file execution
func dBTestsWildcard() {
	task, err := model.FindTask("mocktaskid")
	So(err, ShouldBeNil)
	So(len(task.TestResults), ShouldEqual, TotalResultCount)

	Convey("along with the proper logs", func() {
		// junit_1.xml
		tl := dBFindOneTestLog("pkg1.test.test_things.test_params_func:2")
		So(tl.Lines[0], ShouldContainSubstring, "FAILURE")
		So(tl.Lines[6], ShouldContainSubstring, "AssertionError")
		tl = dBFindOneTestLog("pkg1.test.test_things.SomeTests.test_skippy")
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")

		// junit_2.xml
		tl = dBFindOneTestLog("tests.ATest.fail")
		So(tl.Lines[0], ShouldContainSubstring, "FAILURE")
		So(tl.Lines[1], ShouldContainSubstring, "AssertionFailedError")

		// junit_3.xml
		tl = dBFindOneTestLog(
			"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
		)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")
		tl = dBFindOneTestLog("test.test_bson.TestBSON.test_basic_encode")
		So(tl.Lines[0], ShouldContainSubstring, "AssertionError")
	})
	return
}

// DBFindOneTestLog abstracts away some of the common attributes of database
// verification tests.
func dBFindOneTestLog(name string) *model.TestLog {
	ret, err := model.FindOneTestLog(
		name,
		"mocktaskid",
		0,
	)
	So(err, ShouldBeNil)
	So(ret, ShouldNotBeNil)
	return ret
}
