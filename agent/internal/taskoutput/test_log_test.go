package taskoutput

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v2"
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

func TestTestLogDirectoryHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := client.NewMock("url")
	taskOutputInfo := &taskoutput.TaskOutput{
		TestLogs: taskoutput.TestLogOutput{
			Version: 1,
			BucketConfig: evergreen.BucketConfig{
				Name: t.TempDir(),
				Type: evergreen.BucketTypeLocal,
			},
		},
	}

	for _, test := range []struct {
		name string
		spec *testLogSpec
	}{
		{
			name: "NoSpecFile",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tsk := &task.Task{
				Project:        "project",
				Id:             utility.RandomString(),
				TaskOutputInfo: taskOutputInfo,
			}
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: tsk.Id}, nil)
			require.NoError(t, err)
			h := &testLogDirectoryHandler{
				dir:    t.TempDir(),
				logger: logger,
			}
			h.createSender = func(ctx context.Context, logPath string) (send.Sender, error) {
				return tsk.TaskOutputInfo.TestLogs.NewSender(ctx,
					taskoutput.TaskOptions{
						ProjectID: tsk.Project,
						TaskID:    tsk.Id,
						Execution: tsk.Execution,
					},
					taskoutput.EvergreenSenderOptions{
						Local:         logger.Task().GetSender(),
						FlushInterval: time.Second,
						Parse:         h.spec.getParser(),
					},
					logPath,
				)
			}
			require.NoError(t, h.start(ctx))

			if test.spec != nil {
				data, err := yaml.Marshal(test.spec)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(h.dir, "log_spec.yaml"), data, 0777))
			}

			writerCtx, writerCancel := context.WithCancel(ctx)
			var wg sync.WaitGroup
			wg.Add(1)
			logs := map[string]string{
				utility.RandomString(): "",
				utility.RandomString(): "",
			}
			files := map[string]*os.File{}
			for fn := range logs {
				f, err := os.Create(filepath.Join(h.dir, fn))
				require.NoError(t, err)

				files[fn] = f
			}

			go func() {
				defer func() {
					for _, f := range files {
						assert.NoError(t, f.Close())
					}
					wg.Done()
				}()

				for {
					select {
					case <-writerCtx.Done():
						return
					default:
						for fn, f := range files {
							line := fmt.Sprintf("%s\n", utility.RandomString())
							logs[fn] += line

							_, err := f.WriteString(line)
							require.NoError(t, err)
						}
					}
				}
			}()

			// Check that logs are ingested continuously.
			time.Sleep(5 * time.Second)
			for logPath := range logs {
				it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{logPath}})
				require.NoError(t, err)
				assert.True(t, it.Next())
				assert.NoError(t, it.Close())
			}

			writerCancel()
			wg.Wait()
			require.NoError(t, h.close(ctx))

			for logPath, expectedLogData := range logs {
				it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{logPath}})
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, it.Close())
				}()

				var actualLogData string
				for it.Next() {
					actualLogData += fmt.Sprintf("%s\n", it.Item().Data)
				}
				require.NoError(t, it.Err())

				assert.Equal(t, expectedLogData, actualLogData)
			}
		})
	}
}

func setupCedarServer(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockCedarServer {
	srv, err := timberutil.NewMockCedarServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}
