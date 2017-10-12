package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

const TotalResultCount = 677

var (
	workingDirectory = testutil.GetDirectoryOfFile()
	SingleFileConfig = filepath.Join(workingDirectory, "testdata", "attach", "plugin_attach_xunit.yml")
	WildcardConfig   = filepath.Join(workingDirectory, "testdata", "attach", "plugin_attach_xunit_wildcard.yml")
)

// runTest abstracts away common tests and setup between all attach xunit tests.
// It also takes as an argument a function which runs any additional tests desired.
func runTest(t *testing.T, configPath string, customTests func(string)) {
	resetTasks(t)
	testConfig := testutil.TestConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")

	SkipConvey("With attachResults plugin installed into plugin registry", t, func() {
		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")

		conf := modelData.TaskConfig
		conf.WorkDir = "."
		logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

		Convey("all commands in test project should execute successfully", func() {
			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := Render(command, conf.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(task.ById(conf.Task.Id))
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
	t, err := task.FindOne(task.ById(taskId))
	So(err, ShouldBeNil)
	So(t, ShouldNotBeNil)
	So(len(t.TestResults), ShouldNotEqual, 0)

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
	t, err := task.FindOne(task.ById(taskId))
	So(err, ShouldBeNil)
	So(len(t.TestResults), ShouldEqual, TotalResultCount)

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

func TestParseAndUpload(t *testing.T) {
	assert := assert.New(t)
	xr := xunitResults{
		Files: []string{"*"},
	}
	testConfig := testutil.TestConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("/dev/null")
	modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", WildcardConfig, modelutil.NoPatch)
	testutil.HandleTestingErr(err, t, "failed to setup test data")

	conf := modelData.TaskConfig
	conf.WorkDir = "command/testdata/xunit/"
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	err = xr.parseAndUploadResults(ctx, conf, logger, comm)
	assert.NoError(err)
	messages := comm.GetMockMessages()[conf.Task.Id]

	// spot check messages logged from sending logs to the mock communicator
	messagesToCheck := []string{
		"Attaching test logs for pkg1.test.test_things.SomeTests.test_params_method_2",  // junit_1.xml
		"Attaching test logs for tests.ATest.error",                                     // junit_2.xml
		"Attaching test logs for test.test_bson.TestBSON.test_basic_encode",             // junit_3.xml
		"Attaching test logs for unittest.loader.ModuleImportFailure.tests.test_binder", // results.xml
	}
	count := 0
	for _, message := range messages {
		if util.SliceContains(messagesToCheck, message.Message) {
			count++
		}
	}
	assert.Equal(len(messagesToCheck), count)
}
