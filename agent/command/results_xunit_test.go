package command

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/internal/testutil"
	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const TotalResultCount = 677

var (
	workingDirectory = testutil.GetDirectoryOfFile()
	SingleFileConfig = filepath.Join(workingDirectory, "testdata", "attach", "plugin_attach_xunit.yml")
	WildcardConfig   = filepath.Join(workingDirectory, "testdata", "attach", "plugin_attach_xunit_wildcard.yml")
)

// runTest abstracts away common tests and setup between all attach xunit tests.
// It also takes as an argument a function which runs any additional tests desired.
func runTest(t *testing.T, configPath string, customTests func(context.Context, string)) {
	resetTasks(t)
	testConfig := testutil.TestConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")

	SkipConvey("With attachResults plugin installed into plugin registry", t, func() {
		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "rhel55", configPath, modelutil.NoPatch)
		require.NoError(t, err)

		conf, err := agentutil.MakeTaskConfigFromModelData(ctx, testConfig, modelData)
		require.NoError(t, err)
		conf.WorkDir = "."
		logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
		require.NoError(t, err)

		Convey("all commands in test project should execute successfully", func() {
			for _, projTask := range conf.Project.Tasks {
				So(len(projTask.Commands), ShouldNotEqual, 0)
				for _, command := range projTask.Commands {
					pluginCmds, err := Render(command, &conf.Project, BlockInfo{})
					require.NoError(t, err)
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)

					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
					testTask, err := task.FindOne(ctx, db.Query(task.ById(conf.Task.Id)))
					require.NoError(t, err)
					So(testTask, ShouldNotBeNil)
				}
			}

			Convey("and the tests should be present in the db", func() {
				customTests(t.Context(), modelData.Task.Id)
			})
		})
	})
}

// dBTests are the database verification tests for standard one file execution
func dBTests(ctx context.Context, taskId string) {
	t, err := task.FindOne(context.Background(), db.Query(task.ById(taskId)))
	So(err, ShouldBeNil)
	So(t, ShouldNotBeNil)
	So(len(t.LocalTestResults), ShouldNotEqual, 0)

	Convey("along with the proper logs", func() {
		// junit_3.xml
		tl := dBFindOneTestLog(ctx,
			"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
			taskId,
		)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")
		tl = dBFindOneTestLog(ctx, "test.test_bson.TestBSON.test_basic_encode", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "AssertionError")
	})
}

// dBTestsWildcard are the database verification tests for globbed file execution
func dBTestsWildcard(ctx context.Context, taskId string) {
	t, err := task.FindOne(context.Background(), db.Query(task.ById(taskId)))
	So(err, ShouldBeNil)
	So(len(t.LocalTestResults), ShouldEqual, TotalResultCount)

	Convey("along with the proper logs", func() {
		// junit_1.xml
		tl := dBFindOneTestLog(ctx, "pkg1.test.test_things.test_params_func_2", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "FAILURE")
		So(tl.Lines[6], ShouldContainSubstring, "AssertionError")
		tl = dBFindOneTestLog(ctx, "pkg1.test.test_things.SomeTests.test_skippy", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")

		// junit_2.xml
		tl = dBFindOneTestLog(ctx, "tests.ATest.fail", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "FAILURE")
		So(tl.Lines[1], ShouldContainSubstring, "AssertionFailedError")

		// junit_3.xml
		tl = dBFindOneTestLog(ctx,
			"test.test_threads_replica_set_client.TestThreadsReplicaSet.test_safe_update",
			taskId,
		)
		So(tl.Lines[0], ShouldContainSubstring, "SKIPPED")
		tl = dBFindOneTestLog(ctx, "test.test_bson.TestBSON.test_basic_encode", taskId)
		So(tl.Lines[0], ShouldContainSubstring, "AssertionError")
	})
}

// dBFindOneTestLog abstracts away some of the common attributes of database
// verification tests.
func dBFindOneTestLog(ctx context.Context, name, taskId string) *testlog.TestLog {
	ret, err := testlog.FindOneTestLog(
		ctx,
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

func TestXUnitParseAndUpload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := client.NewMock("url")
	conf := &internal.TaskConfig{
		Task: task.Task{
			Id:             "id",
			Secret:         "secret",
			Project:        "project",
			Version:        "version",
			BuildVariant:   "build_variant",
			DisplayName:    "task_name",
			Execution:      5,
			Requester:      evergreen.GithubPRRequester,
			TaskOutputInfo: agentutil.InitializeTaskOutput(t),
		},
		DisplayTaskInfo: &apimodels.DisplayTaskInfo{},
		WorkDir:         filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "xunit"),
		NewExpansions:   &util.DynamicExpansions{},
	}
	conf.Task.TaskOutputInfo.TestResults.Version = task.TestResultServiceEvergreen

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer){
		"GlobMatchesAsteriskAndSendsTestResults": func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer) {
			xr := xunitResults{
				Files: []string{"*"},
			}
			assert.NoError(t, xr.parseAndUploadResults(ctx, conf, logger, comm))
			assert.NoError(t, logger.Close())
			assert.Len(t, comm.FailedTestSample, 12)
			assert.Equal(t, comm.TestResultStats.FailedCount, 12)
			assert.Equal(t, comm.TestResultStats.TotalCount, 1114)
		},
		"GlobMatchesAbsolutePathContainingWorkDirPrefixAndSendsTestResults": func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer) {
			xr := xunitResults{
				Files: []string{filepath.Join(conf.WorkDir, "junit*.xml")},
			}
			assert.NoError(t, xr.parseAndUploadResults(ctx, conf, logger, comm))
			assert.NoError(t, logger.Close())

			assert.Len(t, comm.FailedTestSample, 10)
			assert.Equal(t, comm.TestResultStats.FailedCount, 10)
			assert.Equal(t, comm.TestResultStats.TotalCount, 683)
		},
		"GlobMatchesRelativePathAndSendsTestResults": func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer) {
			xr := xunitResults{
				Files: []string{filepath.Join(conf.WorkDir, "*")},
			}
			assert.NoError(t, xr.parseAndUploadResults(ctx, conf, logger, comm))
			assert.NoError(t, logger.Close())

			assert.Len(t, comm.FailedTestSample, 12)
			assert.Equal(t, comm.TestResultStats.FailedCount, 12)
			assert.Equal(t, comm.TestResultStats.TotalCount, 1114)
		},
		"EmptyTestsForValidPathCauseNoError": func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer) {
			xr := xunitResults{
				Files: []string{filepath.Join(conf.WorkDir, "empty.xml")},
			}
			assert.NoError(t, xr.parseAndUploadResults(ctx, conf, logger, comm))
			assert.NoError(t, logger.Close())
		},
		"EmptyTestsForInvalidPathErrors": func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer) {
			xr := xunitResults{
				Files: []string{filepath.Join(conf.WorkDir, "nonexistent.xml")},
			}
			assert.Error(t, xr.parseAndUploadResults(ctx, conf, logger, comm))
			assert.NoError(t, logger.Close())
		},
		"DirectoryErrors": func(ctx context.Context, t *testing.T, conf *internal.TaskConfig, logger client.LoggerProducer) {
			xr := xunitResults{
				Files: []string{conf.WorkDir},
			}
			assert.Error(t, xr.parseAndUploadResults(ctx, conf, logger, comm))
			assert.NoError(t, logger.Close())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()

			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			tCase(tctx, t, conf, logger)
		})
	}
}
