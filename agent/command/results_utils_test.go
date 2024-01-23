package command

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestSendTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := []testresult.TestResult{
		{
			TestName:        "test",
			DisplayTestName: "display",
			GroupID:         "group",
			Status:          "pass",
			LogURL:          "https://url.com",
			RawLogURL:       "https://rawurl.com",
			LogTestName:     "log_test_name",
			LineNum:         123,
			TestStartTime:   time.Now().Add(-time.Hour).UTC(),
			TestEndTime:     time.Now().UTC(),
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
				assert.Equal(t, results[0].GroupID, res[0].Results[0].GroupId)
				if results[0].LogTestName != "" {
					assert.Equal(t, results[0].LogTestName, res[0].Results[0].LogTestName)
				} else {
					assert.Equal(t, results[0].TestName, res[0].Results[0].LogTestName)
				}
				assert.Equal(t, results[0].LogURL, res[0].Results[0].LogUrl)
				assert.Equal(t, results[0].RawLogURL, res[0].Results[0].RawLogUrl)
				assert.EqualValues(t, results[0].LineNum, res[0].Results[0].LineNum)
				assert.Equal(t, results[0].TestStartTime, res[0].Results[0].TestStartTime.AsTime())
				assert.Equal(t, results[0].TestEndTime, res[0].Results[0].TestEndTime.AsTime())
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
					assert.Equal(t, testresult.TestResultsServiceCedar, comm.ResultsService)
					assert.False(t, comm.ResultsFailed)
				})
				t.Run("FailingResults", func(t *testing.T) {
					results[0].Status = evergreen.TestFailedStatus
					require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

					assert.Equal(t, testresult.TestResultsServiceCedar, comm.ResultsService)
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
				assert.Equal(t, testresult.TestResultsServiceCedar, comm.ResultsService)
				assert.False(t, comm.ResultsFailed)
				results[0].DisplayTestName = displayTestName
			},
			"SucceedsNoLogTestName": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				logTestName := results[0].LogTestName
				results[0].LogTestName = ""
				require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

				assert.Equal(t, srv.Close.TestResultsRecordId, conf.CedarTestResultsID)
				checkRecord(t, srv)
				checkResults(t, srv)
				assert.NotZero(t, srv.Close.TestResultsRecordId)
				assert.Equal(t, testresult.TestResultsServiceCedar, comm.ResultsService)
				assert.False(t, comm.ResultsFailed)
				results[0].LogTestName = logTestName
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
				comm.ResultsService = ""
				comm.ResultsFailed = false
				testCase(ctx, t, srv.TestResults, comm)
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
