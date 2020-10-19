package command

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/timber/buildlogger"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestSendTestResultsToCedar(t *testing.T) {
	results := &task.LocalTestResults{
		Results: []task.TestResult{
			{
				TestFile:  "test",
				Status:    "status",
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
	td := client.TaskData{
		ID:     tsk.Id,
		Secret: tsk.Secret,
	}
	comm := client.NewMock("url")
	displayTaskName, err := comm.GetDisplayTaskNameFromExecution(context.Background(), td)
	require.NoError(t, err)

	checkRecord := func(t *testing.T, srv *timberutil.MockTestResultsServer) {
		require.NotZero(t, srv.Create)
		assert.Equal(t, tsk.Id, srv.Create.TaskId)
		assert.Equal(t, tsk.Project, srv.Create.Project)
		assert.Equal(t, tsk.BuildVariant, srv.Create.Variant)
		assert.Equal(t, tsk.Version, srv.Create.Version)
		assert.EqualValues(t, tsk.Execution, srv.Create.Execution)
		assert.Equal(t, tsk.Requester, srv.Create.RequestType)
		assert.Equal(t, tsk.DisplayName, srv.Create.TaskName)
		assert.Equal(t, displayTaskName, srv.Create.DisplayTaskName)
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
			assert.EqualValues(t, results.Results[0].LineNum, res[0].Results[0].LineNum)
			assert.Equal(t, int64(results.Results[0].StartTime), res[0].Results[0].TestStartTime.Seconds)
			assert.Equal(t, int64(results.Results[0].EndTime), res[0].Results[0].TestEndTime.Seconds)
		}
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock){
		"Succeeds": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
			require.NoError(t, sendTestResultsToCedar(ctx, tsk, td, comm, results))

			checkRecord(t, srv)
			checkResults(t, srv)
			assert.NotZero(t, srv.Close.TestResultsRecordId)
		},
		"FailsIfCreatingRecordFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
			srv.CreateErr = true

			require.Error(t, sendTestResultsToCedar(ctx, tsk, td, comm, results))
			assert.Empty(t, srv.Results)
			assert.Zero(t, srv.Close)
		},
		"FailsIfAddingResultsFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
			srv.AddErr = true

			require.Error(t, sendTestResultsToCedar(ctx, tsk, td, comm, results))
			checkRecord(t, srv)
			assert.Empty(t, srv.Results)
			assert.Zero(t, srv.Close)
		},
		"FailsIfClosingRecordFails": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
			srv.CloseErr = true

			require.Error(t, sendTestResultsToCedar(ctx, tsk, td, comm, results))
			checkRecord(t, srv)
			checkResults(t, srv)
			assert.Zero(t, srv.Close)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			srv := setupCedarTestResults(ctx, t, comm)

			testCase(ctx, t, srv, comm)
		})
	}
}

func TestSendTestLogToCedar(t *testing.T) {
	ctx := context.TODO()
	tsk := &task.Task{
		Id:           "id",
		Project:      "project",
		Version:      "version",
		BuildVariant: "build_variant",
		Execution:    5,
		Requester:    evergreen.GithubPRRequester,
	}
	log := &model.TestLog{
		Id:            "id",
		Name:          "test",
		Task:          "task",
		TaskExecution: 5,
		Lines:         []string{"log line 1", "log line 2"},
	}
	comm := client.NewMock("url")

	for _, test := range []struct {
		name     string
		testCase func(*testing.T, *timberutil.MockBuildloggerServer)
	}{
		{
			name: "CreateSenderFails",
			testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
				srv.CreateErr = true
				assert.Error(t, sendTestLogToCedar(ctx, tsk, comm, log))
			},
		},
		{
			name: "SendFails",
			testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
				srv.AppendErr = true
				assert.Error(t, sendTestLogToCedar(ctx, tsk, comm, log))
			},
		},
		{
			name: "CloseSenderFails",
			testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
				srv.CloseErr = true
				assert.Error(t, sendTestLogToCedar(ctx, tsk, comm, log))
			},
		},
		{
			name: "SendSucceeds",
			testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
				assert.NoError(t, sendTestLogToCedar(ctx, tsk, comm, log))

				require.NotEmpty(t, srv.Create)
				assert.Equal(t, tsk.Project, srv.Create.Info.Project)
				assert.Equal(t, tsk.Version, srv.Create.Info.Version)
				assert.Equal(t, tsk.BuildVariant, srv.Create.Info.Variant)
				assert.Equal(t, tsk.DisplayName, srv.Create.Info.TaskName)
				assert.Equal(t, tsk.Id, srv.Create.Info.TaskId)
				assert.Equal(t, int32(tsk.Execution), srv.Create.Info.Execution)
				assert.Equal(t, log.Name, srv.Create.Info.TestName)
				assert.Equal(t, !tsk.IsPatchRequest(), srv.Create.Info.Mainline)
				assert.Equal(t, buildlogger.LogStorageS3, buildlogger.LogStorage(srv.Create.Storage))

				require.Len(t, srv.Data, 1)
				for _, data := range srv.Data {
					require.Len(t, data, 1)
					require.Len(t, data[0].Lines, 2)
					assert.Equal(t, strings.Trim(log.Lines[0], "\n"), data[0].Lines[0].Data)
					assert.Equal(t, strings.Trim(log.Lines[1], "\n"), data[0].Lines[1].Data)
				}

			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := setupCedarBuildlogger(ctx, t, comm)
			test.testCase(t, srv)
		})
	}
}

func setupCedarTestResults(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockTestResultsServer {
	srv, err := timberutil.NewMockTestResultsServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}

func setupCedarBuildlogger(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockBuildloggerServer {
	srv, err := timberutil.NewMockBuildloggerServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}
