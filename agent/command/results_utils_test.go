package command

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	resultTestutil "github.com/evergreen-ci/evergreen/model/testresult/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)

	require.NoError(t, task.ClearTestResults(ctx, env))
	require.NoError(t, db.Clear(task.Collection))
	defer func() {
		assert.NoError(t, task.ClearTestResults(ctx, env))
		assert.NoError(t, db.Clear(task.Collection))
	}()

	results := []testresult.TestResult{
		{
			TestName:        "test",
			DisplayTestName: "display",
			Status:          "pass",
			LogInfo: &testresult.TestLogInfo{
				LogName: "name.log",
				LogsToMerge: []*string{
					utility.ToStringPtr("background.log"),
					utility.ToStringPtr("process.log"),
				},
				LineNum:       456,
				RenderingType: utility.ToStringPtr("resmoke"),
				Version:       1,
			},
			GroupID:       "group",
			LogURL:        "https://url.com",
			RawLogURL:     "https://rawurl.com",
			LineNum:       123,
			TestStartTime: time.Now().Add(-time.Hour).UTC(),
			TestEndTime:   time.Now().UTC(),
		},
	}
	conf := &internal.TaskConfig{
		Task: task.Task{
			Id:           "id",
			Secret:       "secret",
			CreateTime:   time.Now().Add(-time.Hour),
			Project:      "project",
			Version:      "version",
			BuildVariant: "build_variant",
			DisplayName:  "task_name",
			Execution:    5,
			Requester:    evergreen.GithubPRRequester,
		},
		DisplayTaskInfo: &apimodels.DisplayTaskInfo{
			ID:   "mock_display_task_id",
			Name: "display_task_name",
		},
	}
	comm := client.NewMock("url")
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, logger.Close())
	}()

	conf.Task.TaskOutputInfo = &task.TaskOutput{
		TestResults: task.TestResultOutput{
			Version: task.TestResultServiceEvergreen,
			BucketConfig: evergreen.BucketConfig{
				Type:              evergreen.BucketTypeLocal,
				TestResultsPrefix: "test-results",
				Name:              t.TempDir(),
			},
		},
	}
	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: conf.Task.TaskOutputInfo.TestResults.BucketConfig.Name})
	require.NoError(t, err)
	svc := task.NewTestResultService(env)

	saveTestResults(t, ctx, testBucket, svc, &conf.Task, 1, results)
	t.Run("ToEvergreen", func(t *testing.T) {
		for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, comm *client.Mock){
			"Succeeds": func(ctx context.Context, t *testing.T, comm *client.Mock) {
				t.Run("PassingResults", func(t *testing.T) {
					require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))
					assert.False(t, comm.ResultsFailed)
				})
				t.Run("FailingResults", func(t *testing.T) {
					results[0].Status = evergreen.TestFailedStatus
					require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))
					assert.True(t, comm.ResultsFailed)
					results[0].Status = "pass"
				})
			},
			"SucceedsNoDisplayTestName": func(ctx context.Context, t *testing.T, comm *client.Mock) {
				displayTestName := results[0].DisplayTestName
				results[0].DisplayTestName = ""
				require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))
				assert.False(t, comm.ResultsFailed)
				results[0].DisplayTestName = displayTestName
			},
		} {
			t.Run(testName, func(t *testing.T) {
				comm.ResultsFailed = false
				testCase(ctx, t, comm)
			})
		}
	})
}

func getTestResults() *testresult.DbTaskTestResults {
	info := testresult.TestResultsInfo{
		Project:   utility.RandomString(),
		Version:   utility.RandomString(),
		Variant:   utility.RandomString(),
		TaskName:  utility.RandomString(),
		TaskID:    utility.RandomString(),
		Execution: rand.Intn(5),
		Requester: utility.RandomString(),
	}
	// Optional fields, we should test that we handle them properly when
	// they are populated and when they do not.
	if sometimes.Half() {
		info.DisplayTaskName = utility.RandomString()
		info.DisplayTaskID = utility.RandomString()
	}

	return &testresult.DbTaskTestResults{
		ID:          info.ID(),
		Info:        info,
		CreatedAt:   time.Now().Add(-time.Hour).UTC().Round(time.Millisecond),
		CompletedAt: time.Now().UTC().Round(time.Millisecond),
	}
}

func saveTestResults(t *testing.T, ctx context.Context, testBucket pail.Bucket, svc task.TestResultsService, tsk *task.Task, length int, savedResults []testresult.TestResult) []testresult.TestResult {
	tr := getTestResults()
	tr.Info.TaskID = tsk.Id
	tr.Info.Execution = tsk.Execution
	savedParquet := testresult.ParquetTestResults{
		Version:   tr.Info.Version,
		Variant:   tr.Info.Variant,
		TaskName:  tr.Info.TaskName,
		TaskID:    tr.Info.TaskID,
		Execution: int32(tr.Info.Execution),
		Requester: tr.Info.Requester,
		CreatedAt: tr.CreatedAt.UTC(),
		Results:   make([]testresult.ParquetTestResult, length),
	}

	for i := 0; i < len(savedResults); i++ {
		savedParquet.Results[i] = testresult.ParquetTestResult{
			TestName:       savedResults[i].TestName,
			GroupID:        utility.ToStringPtr(savedResults[i].GroupID),
			Status:         savedResults[i].Status,
			LogInfo:        savedResults[i].LogInfo,
			TaskCreateTime: savedResults[i].TaskCreateTime.UTC(),
			TestStartTime:  savedResults[i].TestStartTime.UTC(),
			TestEndTime:    savedResults[i].TestEndTime.UTC(),
		}
		if savedResults[i].DisplayTestName != "" {
			savedParquet.Results[i].DisplayTestName = utility.ToStringPtr(savedResults[i].DisplayTestName)
			savedParquet.Results[i].GroupID = utility.ToStringPtr(savedResults[i].GroupID)
			savedParquet.Results[i].LogTestName = utility.ToStringPtr(savedResults[i].LogTestName)
			savedParquet.Results[i].LogURL = utility.ToStringPtr(savedResults[i].LogURL)
			savedParquet.Results[i].RawLogURL = utility.ToStringPtr(savedResults[i].RawLogURL)
			savedParquet.Results[i].LineNum = utility.ToInt32Ptr(int32(savedResults[i].LineNum))
		}
	}

	w, err := testBucket.Writer(ctx, fmt.Sprintf("%s/%s", tsk.TaskOutputInfo.TestResults.BucketConfig.TestResultsPrefix, testresult.PartitionKey(tr.CreatedAt, tr.Info.Project, tr.ID)))
	require.NoError(t, err)
	defer func() { assert.NoError(t, w.Close()) }()

	pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(task.ParquetTestResultsSchemaDef)))
	require.NoError(t, pw.Write(savedParquet))
	require.NoError(t, pw.Close())
	require.NoError(t, db.Insert(ctx, testresult.Collection, tr))
	require.NoError(t, svc.AppendTestResultMetadata(resultTestutil.MakeAppendTestResultMetadataReq(ctx, savedResults, tr.ID)))
	return savedResults
}
