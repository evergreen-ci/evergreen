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
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/pail"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	"github.com/mongodb/grip/sometimes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	comm := client.NewMock("url")
	displayTaskInfo, err := comm.GetDisplayTaskInfoFromExecution(ctx, td)
	require.NoError(t, err)
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, logger.Close())
	}()

	t.Run("ToCedar", func(t *testing.T) {
		checkRecord := func(t *testing.T, srv *timberutil.MockTestResultsServer) {
			require.NotZero(t, srv.Create)
			assert.Equal(t, conf.Task.Id, srv.Create.TaskId)
			assert.Equal(t, conf.Task.Project, srv.Create.Project)
			assert.Equal(t, conf.Task.BuildVariant, srv.Create.Variant)
			assert.Equal(t, conf.Task.Version, srv.Create.Version)
			assert.EqualValues(t, conf.Task.Execution, srv.Create.Execution)
			assert.Equal(t, conf.Task.Requester, srv.Create.RequestType)
			assert.Equal(t, conf.Task.DisplayName, srv.Create.TaskName)
			assert.Equal(t, displayTaskInfo.ID, srv.Create.DisplayTaskId)
			assert.Equal(t, displayTaskInfo.Name, srv.Create.DisplayTaskName)
			assert.False(t, srv.Create.Mainline)
		}
		checkResults := func(t *testing.T, srv *timberutil.MockTestResultsServer) {
			require.Len(t, srv.Results, 1)
			for id, res := range srv.Results {
				assert.NotEmpty(t, id)
				require.Len(t, res, 1)
				require.Len(t, res[0].Results, 1)
				assert.NotEmpty(t, res[0].Results[0].TestName)
				assert.NotEqual(t, results[0].TestName, res[0].Results[0].TestName)
				if results[0].DisplayTestName != "" {
					assert.Equal(t, results[0].DisplayTestName, res[0].Results[0].DisplayTestName)
				} else {
					assert.Equal(t, results[0].TestName, res[0].Results[0].DisplayTestName)
				}
				assert.Equal(t, results[0].Status, res[0].Results[0].Status)
				assert.Equal(t, results[0].LogInfo.LogName, res[0].Results[0].LogInfo.LogName)
				require.Len(t, res[0].Results[0].LogInfo.LogsToMerge, len(results[0].LogInfo.LogsToMerge))
				for i, logName := range results[0].LogInfo.LogsToMerge {
					assert.Equal(t, *logName, res[0].Results[0].LogInfo.LogsToMerge[i])
				}
				assert.Equal(t, results[0].LogInfo.LineNum, res[0].Results[0].LogInfo.LineNum)
				assert.Equal(t, results[0].LogInfo.RenderingType, res[0].Results[0].LogInfo.RenderingType)
				assert.Equal(t, results[0].LogInfo.Version, res[0].Results[0].LogInfo.Version)
				assert.Equal(t, results[0].TestStartTime, res[0].Results[0].TestStartTime.AsTime())
				assert.Equal(t, results[0].TestEndTime, res[0].Results[0].TestEndTime.AsTime())

				// Legacy test log fields.
				assert.Equal(t, results[0].GroupID, res[0].Results[0].GroupId)
				assert.Empty(t, res[0].Results[0].LogTestName)
				assert.Equal(t, results[0].LogURL, res[0].Results[0].LogUrl)
				assert.Equal(t, results[0].RawLogURL, res[0].Results[0].RawLogUrl)
				assert.EqualValues(t, results[0].LineNum, res[0].Results[0].LineNum)
			}
		}

		for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock){
			"Succeeds": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				t.Run("PassingResults", func(t *testing.T) {
					require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

					assert.Equal(t, srv.Close.TestResultsRecordId, conf.CedarTestResultsID)
					checkRecord(t, srv)
					checkResults(t, srv)
					assert.NotZero(t, srv.Close.TestResultsRecordId)
					assert.False(t, comm.ResultsFailed)
				})
				t.Run("FailingResults", func(t *testing.T) {
					results[0].Status = evergreen.TestFailedStatus
					require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

					assert.True(t, comm.ResultsFailed)
					results[0].Status = "pass"
				})
			},
			"SucceedsNoDisplayTestName": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				displayTestName := results[0].DisplayTestName
				results[0].DisplayTestName = ""
				require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

				assert.Equal(t, srv.Close.TestResultsRecordId, conf.CedarTestResultsID)
				checkRecord(t, srv)
				checkResults(t, srv)
				assert.NotZero(t, srv.Close.TestResultsRecordId)
				assert.False(t, comm.ResultsFailed)
				results[0].DisplayTestName = displayTestName
			},
			"FailsIfCreatingRecordFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				srv.CreateErr = true

				require.Error(t, sendTestResults(ctx, comm, logger, conf, results))
				assert.Empty(t, srv.Results)
				assert.Zero(t, srv.Close)
			},
			"FailsIfAddingResultsFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				srv.AddErr = true

				require.Error(t, sendTestResults(ctx, comm, logger, conf, results))
				checkRecord(t, srv)
				assert.Empty(t, srv.Results)
				assert.Zero(t, srv.Close)
			},
			"FailsIfClosingRecordFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				srv.CloseErr = true

				require.Error(t, sendTestResults(ctx, comm, logger, conf, results))
				checkRecord(t, srv)
				checkResults(t, srv)
				assert.Zero(t, srv.Close)
			},
		} {
			t.Run(testName, func(t *testing.T) {
				conf.CedarTestResultsID = ""
				srv := setupCedarServer(ctx, t, comm)
				comm.ResultsFailed = false
				testCase(ctx, t, srv.TestResults, comm)
			})
		}
	})

	conf.Task.TaskOutputInfo = &task.TaskOutput{
		TestResults: task.TestResultOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Type:              evergreen.BucketTypeLocal,
				TestResultsPrefix: "test-results",
				Name:              t.TempDir(),
			},
		},
	}
	testBucket, err := pail.NewLocalBucket(pail.LocalOptions{Path: conf.Task.TaskOutputInfo.TestResults.BucketConfig.Name})
	require.NoError(t, err)
	svc := task.NewEvergreenService(env)

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

func setupCedarServer(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockCedarServer {
	srv, err := timberutil.NewMockCedarServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
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
