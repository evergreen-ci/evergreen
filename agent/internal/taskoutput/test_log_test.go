package taskoutput

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/timber/buildlogger"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestAppendTestLog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tsk := &task.Task{
		Id:             "id",
		Project:        "project",
		Version:        "version",
		BuildVariant:   "build_variant",
		Execution:      5,
		Requester:      evergreen.GithubPRRequester,
		TaskOutputInfo: &taskoutput.TaskOutput{},
	}
	testLog := &testlog.TestLog{
		Id:            "id",
		Name:          "test",
		Task:          "task",
		TaskExecution: 5,
		Lines:         []string{"log line 1\nlog line 2", "log line 3"},
	}
	comm := client.NewMock("url")

	t.Run("RoundTrip", func(t *testing.T) {
		tsk.TaskOutputInfo = &taskoutput.TaskOutput{
			TestLogs: taskoutput.TestLogOutput{
				Version: 1,
				BucketConfig: evergreen.BucketConfig{
					Name: t.TempDir(),
					Type: evergreen.BucketTypeLocal,
				},
			},
		}

		require.NoError(t, AppendTestLog(ctx, comm, tsk, testLog))
		it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{testLog.Name}})
		require.NoError(t, err)

		var lines []string
		for it.Next() {
			line := it.Item()
			assert.Equal(t, level.Info, line.Priority)
			assert.WithinDuration(t, time.Now(), time.Unix(0, line.Timestamp), time.Second)
			lines = append(lines, line.Data)
		}
		assert.Equal(t, strings.Join(testLog.Lines, "\n"), strings.Join(lines, "\n"))
	})
	t.Run("ToCedar", func(t *testing.T) {
		tsk.TaskOutputInfo = &taskoutput.TaskOutput{}

		for _, test := range []struct {
			name     string
			testCase func(*testing.T, *timberutil.MockBuildloggerServer)
		}{
			{
				name: "CreateSenderFails",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					srv.CreateErr = true
					assert.Error(t, AppendTestLog(ctx, comm, tsk, testLog))
				},
			},
			{
				name: "SendFails",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					srv.AppendErr = true
					assert.Error(t, AppendTestLog(ctx, comm, tsk, testLog))
				},
			},
			{
				name: "CloseSenderFails",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					srv.CloseErr = true
					assert.Error(t, AppendTestLog(ctx, comm, tsk, testLog))
				},
			},
			{
				name: "SendSucceeds",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					require.NoError(t, AppendTestLog(ctx, comm, tsk, testLog))

					require.NotEmpty(t, srv.Create)
					assert.Equal(t, tsk.Project, srv.Create.Info.Project)
					assert.Equal(t, tsk.Version, srv.Create.Info.Version)
					assert.Equal(t, tsk.BuildVariant, srv.Create.Info.Variant)
					assert.Equal(t, tsk.DisplayName, srv.Create.Info.TaskName)
					assert.Equal(t, tsk.Id, srv.Create.Info.TaskId)
					assert.Equal(t, int32(tsk.Execution), srv.Create.Info.Execution)
					assert.Equal(t, testLog.Name, srv.Create.Info.TestName)
					assert.Equal(t, !tsk.IsPatchRequest(), srv.Create.Info.Mainline)
					assert.Equal(t, buildlogger.LogStorageS3, buildlogger.LogStorage(srv.Create.Storage))

					require.Len(t, srv.Data, 1)
					for _, data := range srv.Data {
						require.Len(t, data, 1)
						require.Len(t, data[0].Lines, 3)
					}

				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				srv := setupCedarServer(ctx, t, comm)
				test.testCase(t, srv.Buildlogger)
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
