package command

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func setupCedarTestResults(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockTestResultsServer {
	srv, err := timberutil.NewMockTestResultsServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}

func TestSendTestResultsToCedar(t *testing.T) {
	results := &task.LocalTestResults{
		Results: []task.TestResult{
			{
				TestFile:  "test",
				Status:    "status",
				URL:       "url",
				LineNum:   123,
				StartTime: float64(time.Now().Add(-time.Hour).Unix()),
				EndTime:   float64(time.Now().Unix()),
			},
		},
	}
	tsk := &task.Task{
		Id:           "id",
		Secret:       "secret",
		CreateTime:   time.Now().Add(-time.Hour),
		Project:      "project",
		Version:      "version",
		BuildVariant: "build_variant",
		Execution:    5,
		Requester:    evergreen.GithubPRRequester,
	}

	checkRecord := func(t *testing.T, srv *timberutil.MockTestResultsServer) {
		require.NotZero(t, srv.Create)
		assert.Equal(t, tsk.Id, srv.Create.TaskId)
		assert.Equal(t, tsk.Project, srv.Create.Project)
		assert.Equal(t, tsk.BuildVariant, srv.Create.Variant)
		assert.Equal(t, tsk.Version, srv.Create.Version)
		assert.EqualValues(t, tsk.Execution, srv.Create.Execution)
		assert.Equal(t, tsk.Requester, srv.Create.RequestType)
		assert.Equal(t, tsk.DisplayName, srv.Create.TaskName)
		assert.False(t, srv.Create.Mainline)
	}
	checkResults := func(t *testing.T, srv *timberutil.MockTestResultsServer) {
		require.Len(t, srv.Results, 1)
		for id, res := range srv.Results {
			assert.NotEmpty(t, id)
			require.Len(t, res, 1)
			require.Len(t, res[0].Results, 1)
			assert.Equal(t, results.Results[0].TestFile, res[0].Results[0].TestName)
			assert.Equal(t, results.Results[0].Status, res[0].Results[0].Status)
			assert.Equal(t, results.Results[0].URL, res[0].Results[0].LogUrl)
			assert.EqualValues(t, results.Results[0].LineNum, res[0].Results[0].LineNum)
			assert.Equal(t, int64(results.Results[0].StartTime), res[0].Results[0].TestStartTime.Seconds)
			assert.Equal(t, int64(results.Results[0].EndTime), res[0].Results[0].TestEndTime.Seconds)
		}
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock, logger client.LoggerProducer){
		"Succeeds": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock, logger client.LoggerProducer) {
			require.NoError(t, sendTestResultsToCedar(ctx, tsk, comm, results, logger))

			checkRecord(t, srv)
			checkResults(t, srv)
			assert.NotZero(t, srv.Close.TestResultsRecordId)
		},
		"FailsIfCreatingRecordFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock, logger client.LoggerProducer) {
			srv.CreateErr = true

			require.Error(t, sendTestResultsToCedar(ctx, tsk, comm, results, logger))
			assert.Empty(t, srv.Results)
			assert.Zero(t, srv.Close)
		},
		"FailsIfAddingResultsFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock, logger client.LoggerProducer) {
			srv.AddErr = true

			require.Error(t, sendTestResultsToCedar(ctx, tsk, comm, results, logger))
			checkRecord(t, srv)
			assert.Empty(t, srv.Results)
			assert.Zero(t, srv.Close)
		},
		"FailsIfClosingRecordFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock, logger client.LoggerProducer) {
			srv.CloseErr = true

			require.Error(t, sendTestResultsToCedar(ctx, tsk, comm, results, logger))
			checkRecord(t, srv)
			checkResults(t, srv)
			assert.Zero(t, srv.Close)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			comm := client.NewMock("http://localhost.com")
			srv := setupCedarTestResults(ctx, t, comm)

			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: tsk.Id, Secret: tsk.Secret}, nil)
			require.NoError(t, err)

			testCase(ctx, t, srv, comm, logger)
		})
	}

}
