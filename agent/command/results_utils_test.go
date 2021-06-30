package command

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/timber/buildlogger"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestSendTestResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := &task.LocalTestResults{
		Results: []task.TestResult{
			{
				TestFile:        "test",
				DisplayTestName: "display",
				GroupID:         "group",
				Status:          "pass",
				URL:             "https://url.com",
				URLRaw:          "https://rawurl.com",
				LogTestName:     "log_test_name",
				LineNum:         123,
				StartTime:       float64(time.Now().Add(-time.Hour).Unix()),
				EndTime:         float64(time.Now().Unix()),
			},
		},
	}
	conf := &internal.TaskConfig{
		Task: &task.Task{
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
		ProjectRef: &model.ProjectRef{
			FilesIgnoredFromCache: []string{"ignoreMe"},
			DisabledStatsCache:    utility.ToBoolPtr(true),
		},
	}
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	comm := client.NewMock("url")
	displayTaskInfo, err := comm.GetDisplayTaskInfoFromExecution(ctx, td)
	require.NoError(t, err)
	logger, err := comm.GetLoggerProducer(ctx, td, nil)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, logger.Close())
	}()

	t.Run("ToCedar", func(t *testing.T) {
		conf.ProjectRef.CedarTestResultsEnabled = utility.TruePtr()
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
			assert.Equal(t, conf.ProjectRef.FilesIgnoredFromCache, srv.Create.HistoricalDataIgnore)
			assert.Equal(t, conf.ProjectRef.IsStatsCacheDisabled(), srv.Create.HistoricalDataDisabled)
		}
		checkResults := func(t *testing.T, srv *timberutil.MockTestResultsServer) {
			require.Len(t, srv.Results, 1)
			for id, res := range srv.Results {
				assert.NotEmpty(t, id)
				require.Len(t, res, 1)
				require.Len(t, res[0].Results, 1)
				assert.NotEmpty(t, res[0].Results[0].TestName)
				assert.NotEqual(t, results.Results[0].TestFile, res[0].Results[0].TestName)
				if results.Results[0].DisplayTestName != "" {
					assert.Equal(t, results.Results[0].DisplayTestName, res[0].Results[0].DisplayTestName)
				} else {
					assert.Equal(t, results.Results[0].TestFile, res[0].Results[0].DisplayTestName)
				}
				assert.Equal(t, results.Results[0].Status, res[0].Results[0].Status)
				assert.Equal(t, results.Results[0].GroupID, res[0].Results[0].GroupId)
				if results.Results[0].LogTestName != "" {
					assert.Equal(t, results.Results[0].LogTestName, res[0].Results[0].LogTestName)
				} else {
					assert.Equal(t, results.Results[0].TestFile, res[0].Results[0].LogTestName)
				}
				assert.Equal(t, results.Results[0].URL, res[0].Results[0].LogUrl)
				assert.Equal(t, results.Results[0].URLRaw, res[0].Results[0].RawLogUrl)
				assert.EqualValues(t, results.Results[0].LineNum, res[0].Results[0].LineNum)
				assert.Equal(t, int64(results.Results[0].StartTime), res[0].Results[0].TestStartTime.Seconds)
				assert.Equal(t, int64(results.Results[0].EndTime), res[0].Results[0].TestEndTime.Seconds)
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
					assert.True(t, comm.HasCedarResults)
					assert.False(t, comm.CedarResultsFailed)
				})
				t.Run("FailingResults", func(t *testing.T) {
					results.Results[0].Status = evergreen.TestFailedStatus
					require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

					assert.True(t, comm.HasCedarResults)
					assert.True(t, comm.CedarResultsFailed)
					results.Results[0].Status = "pass"
				})
			},
			"SucceedsNoDisplayTestName": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				displayTestName := results.Results[0].DisplayTestName
				results.Results[0].DisplayTestName = ""
				require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

				assert.Equal(t, srv.Close.TestResultsRecordId, conf.CedarTestResultsID)
				checkRecord(t, srv)
				checkResults(t, srv)
				assert.NotZero(t, srv.Close.TestResultsRecordId)
				assert.True(t, comm.HasCedarResults)
				assert.False(t, comm.CedarResultsFailed)
				results.Results[0].DisplayTestName = displayTestName
			},
			"SucceedsNoLogTestName": func(ctx context.Context, t *testing.T, srv *timberutil.MockTestResultsServer, comm *client.Mock) {
				logTestName := results.Results[0].LogTestName
				results.Results[0].LogTestName = ""
				require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))

				assert.Equal(t, srv.Close.TestResultsRecordId, conf.CedarTestResultsID)
				checkRecord(t, srv)
				checkResults(t, srv)
				assert.NotZero(t, srv.Close.TestResultsRecordId)
				assert.True(t, comm.HasCedarResults)
				assert.False(t, comm.CedarResultsFailed)
				results.Results[0].LogTestName = logTestName
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
				comm.HasCedarResults = false
				comm.CedarResultsFailed = false
				testCase(ctx, t, srv.TestResults, comm)
			})
		}
	})
	t.Run("ToEvergreen", func(t *testing.T) {
		conf.ProjectRef.CedarTestResultsEnabled = utility.FalsePtr()

		require.NoError(t, sendTestResults(ctx, comm, logger, conf, results))
		assert.Equal(t, results, comm.LocalTestResults)
	})
}

func TestSendTestLog(t *testing.T) {
	ctx := context.TODO()
	conf := &internal.TaskConfig{
		Task: &task.Task{
			Id:           "id",
			Project:      "project",
			Version:      "version",
			BuildVariant: "build_variant",
			Execution:    5,
			Requester:    evergreen.GithubPRRequester,
		},
		ProjectRef: &model.ProjectRef{},
	}
	log := &model.TestLog{
		Id:            "id",
		Name:          "test",
		Task:          "task",
		TaskExecution: 5,
		Lines:         []string{"log line 1", "log line 2"},
	}
	comm := client.NewMock("url")

	t.Run("ToCedar", func(t *testing.T) {
		conf.ProjectRef.CedarTestResultsEnabled = utility.TruePtr()
		for _, test := range []struct {
			name     string
			testCase func(*testing.T, *timberutil.MockBuildloggerServer)
		}{
			{
				name: "CreateSenderFails",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					srv.CreateErr = true

					_, err := sendTestLog(ctx, comm, conf, log)
					assert.Error(t, err)
				},
			},
			{
				name: "SendFails",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					srv.AppendErr = true

					_, err := sendTestLog(ctx, comm, conf, log)
					assert.Error(t, err)
				},
			},
			{
				name: "CloseSenderFails",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					srv.CloseErr = true

					_, err := sendTestLog(ctx, comm, conf, log)
					assert.Error(t, err)
				},
			},
			{
				name: "SendSucceeds",
				testCase: func(t *testing.T, srv *timberutil.MockBuildloggerServer) {
					_, err := sendTestLog(ctx, comm, conf, log)
					assert.NoError(t, err)

					require.NotEmpty(t, srv.Create)
					assert.Equal(t, conf.Task.Project, srv.Create.Info.Project)
					assert.Equal(t, conf.Task.Version, srv.Create.Info.Version)
					assert.Equal(t, conf.Task.BuildVariant, srv.Create.Info.Variant)
					assert.Equal(t, conf.Task.DisplayName, srv.Create.Info.TaskName)
					assert.Equal(t, conf.Task.Id, srv.Create.Info.TaskId)
					assert.Equal(t, int32(conf.Task.Execution), srv.Create.Info.Execution)
					assert.Equal(t, log.Name, srv.Create.Info.TestName)
					assert.Equal(t, !conf.Task.IsPatchRequest(), srv.Create.Info.Mainline)
					assert.Equal(t, buildlogger.LogStorageS3, buildlogger.LogStorage(srv.Create.Storage))

					require.Len(t, srv.Data, 1)
					for _, data := range srv.Data {
						require.Len(t, data, 1)
						require.Len(t, data[0].Lines, 2)
						assert.EqualValues(t, strings.Trim(log.Lines[0], "\n"), data[0].Lines[0].Data)
						assert.EqualValues(t, strings.Trim(log.Lines[1], "\n"), data[0].Lines[1].Data)
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
	t.Run("ToEvergreen", func(t *testing.T) {
		conf.ProjectRef.CedarTestResultsEnabled = utility.FalsePtr()

		logId, err := sendTestLog(ctx, comm, conf, log)
		require.NoError(t, err)
		assert.NotEmpty(t, logId)

		require.Len(t, comm.TestLogs, 1)
		assert.Equal(t, log, comm.TestLogs[0])
	})
}

func setupCedarServer(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockCedarServer {
	srv, err := timberutil.NewMockCedarServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}
