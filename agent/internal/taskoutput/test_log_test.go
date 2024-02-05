package taskoutput

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	serviceutil "github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/timber/buildlogger"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
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
		Lines:         []string{"log line 1\nlog line 2", "log line 3\n", "log line 4"},
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

		var actual string
		for it.Next() {
			line := it.Item()
			assert.Equal(t, level.Info, line.Priority)
			assert.WithinDuration(t, time.Now(), time.Unix(0, line.Timestamp), time.Second)
			actual += line.Data + "\n"
		}
		expectedLines := "log line 1\nlog line 2\nlog line 3\nlog line 4\n"
		assert.Equal(t, expectedLines, actual)
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
						require.Len(t, data[0].Lines, 4)
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

func TestTestLogDirectoryHandlerRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := client.NewMock("url")

	type logInfo struct {
		logPath       string
		expectedLines []string
	}

	for _, test := range []struct {
		name string
		logs []logInfo
	}{
		{
			name: "RecursiveDirectoryWalk",
			logs: []logInfo{
				{
					logPath: "log0.log",
					expectedLines: []string{
						"This is a small test log.",
						"With only a few lines.",
					},
				},
				{
					logPath: "log1.log",
					expectedLines: []string{
						"This is a another small test log.",
						"With only a few lines.",
						"But it should get there.",
					},
				},
				{
					logPath: "nested/log2.log",
					expectedLines: []string{
						"This is a another small test log.",
						"With only a few lines.",
						"But this time it is in nested directory.",
					},
				},
				{
					logPath: "nested/nested/log3.log",
					expectedLines: []string{
						"This is a small test log, again.",
						"In an an even more nested directory.",
						"With some a friend!",
					},
				},
				{
					logPath: "nested/nested/log4.log",
					expectedLines: []string{
						"This is the last test log.",
						"Blah blah blah blah.",
						"Something something something.",
						"We are done here.",
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tsk, h := setupTestTestLogDirectoryHandler(t, comm)
			for _, li := range test.logs {
				require.NoError(t, os.MkdirAll(filepath.Join(h.dir, filepath.Dir(li.logPath)), 0777))
				require.NoError(t, os.WriteFile(filepath.Join(h.dir, li.logPath), []byte(strings.Join(li.expectedLines, "\n")+"\n"), 0777))
			}

			require.NoError(t, h.run(ctx))
			for _, li := range test.logs {
				it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{li.logPath}})
				require.NoError(t, err)
				var persistedRawLines []string
				for it.Next() {
					persistedRawLines = append(persistedRawLines, it.Item().Data)
				}
				assert.NoError(t, it.Close())
				require.NoError(t, it.Err())
				assert.Equal(t, li.expectedLines, persistedRawLines)
			}
		})
	}
}

func TestTestLogDirectoryHandlerGetSpecFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := client.NewMock("url")
	getRawLinesAndFormatter := func(format testLogFormat) ([]string, func(log.LogLine) string) {
		rawLines := []string{
			"This is a log line.",
			"This is another log line...",
			"This is the last log line.",
		}

		var formatLine func(log.LogLine) string
		switch format {
		case testLogFormatTextTimestamp:
			for i := range rawLines {
				rawLines[i] = fmt.Sprintf("%d %s", time.Now().UnixNano(), rawLines[i])
			}
			formatLine = func(line log.LogLine) string { return fmt.Sprintf("%d %s", line.Timestamp, line.Data) }
		default:
			formatLine = func(line log.LogLine) string { return line.Data }
		}

		return rawLines, formatLine
	}

	for _, test := range []struct {
		name     string
		specData interface{}
	}{
		{
			name:     "InvalidYAML",
			specData: []byte("this is not YAML"),
		},
		{
			name: "NoSpecFile",
		},
		{
			name:     "TextFormat",
			specData: testLogSpec{Format: testLogFormatDefault},
		},
		{
			name:     "TextTimestampFormat",
			specData: testLogSpec{Format: testLogFormatTextTimestamp},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tsk, h := setupTestTestLogDirectoryHandler(t, comm)

			// Set the expected test log spec and write spec file
			// if spec data exists.
			var (
				data         []byte
				expectedSpec testLogSpec
			)
			switch v := test.specData.(type) {
			case []byte:
				data = v
			case testLogSpec:
				var err error
				data, err = yaml.Marshal(&v)
				require.NoError(t, err)
				expectedSpec = v
			}
			if data != nil {
				require.NoError(t, os.WriteFile(filepath.Join(h.dir, testLogSpecFilename), data, 0777))
			}

			// Write a log to the empty test log directory to
			// trigger the handler to look for the spec file.
			rawLines, formatLine := getRawLinesAndFormatter(expectedSpec.Format)
			logPath := utility.RandomString()
			require.NoError(t, os.WriteFile(filepath.Join(h.dir, logPath), []byte(strings.Join(rawLines, "\n")+"\n"), 0777))
			require.NoError(t, h.run(ctx))

			assert.Equal(t, expectedSpec, h.spec)

			// Check that only log files are handled.
			assert.Equal(t, 1, h.logFileCount)

			// Check that raw log lines were correctly parsed in
			// accordance with their format.
			it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{logPath}})
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, it.Close())
			}()
			var persistedRawLines []string
			for it.Next() {
				persistedRawLines = append(persistedRawLines, formatLine(it.Item()))
			}
			require.NoError(t, it.Err())
			assert.Equal(t, rawLines, persistedRawLines)
		})
	}
}

func TestTestLogSpecGetParser(t *testing.T) {
	for _, test := range []struct {
		name string
		spec testLogSpec
		test func(*testing.T, taskoutput.LogLineParser)
	}{
		{
			name: "DefaultFormat",
			spec: testLogSpec{},
			test: func(t *testing.T, parser taskoutput.LogLineParser) {
				assert.Nil(t, parser)
			},
		},
		{
			name: "Text",
			spec: testLogSpec{Format: testLogFormatDefault},
			test: func(t *testing.T, parser taskoutput.LogLineParser) {
				assert.Nil(t, parser)
			},
		},
		{
			name: "TextTimestamp",
			spec: testLogSpec{Format: testLogFormatTextTimestamp},
			test: func(t *testing.T, parser taskoutput.LogLineParser) {
				ts := time.Now().UnixNano()
				data := "This is a log line."

				t.Run("MalformedLine", func(t *testing.T) {
					_, err := parser("NoSpaces\n")
					assert.Error(t, err)
				})
				t.Run("InvalidTimestamp", func(t *testing.T) {
					_, err := parser(fmt.Sprintf("%v %s\n", time.Now(), data))
					assert.Error(t, err)
				})
				t.Run("ParseRawLine", func(t *testing.T) {
					line, err := parser(fmt.Sprintf("%d %s\n", ts, data))
					require.NoError(t, err)
					assert.Zero(t, line.Priority)
					assert.Equal(t, ts, line.Timestamp)
					assert.Equal(t, data, line.Data)
				})
				t.Run("ParseRawLineWithLeadingWhiteSpace", func(t *testing.T) {
					line, err := parser(fmt.Sprintf("   %d %s\n", ts, data))
					require.NoError(t, err)
					assert.Zero(t, line.Priority)
					assert.Equal(t, ts, line.Timestamp)
					assert.Equal(t, data, line.Data)
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.test(t, test.spec.getParser())
		})
	}
}

func TestTestLogFormatValidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		format testLogFormat
		hasErr bool
	}{
		{
			name:   "Text",
			format: testLogFormatDefault,
		},
		{
			name:   "TextTimestamp",
			format: testLogFormatTextTimestamp,
		},
		{
			name:   "Invalid",
			format: testLogFormat("invalid"),
			hasErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.format.validate()
			if test.hasErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func setupTestTestLogDirectoryHandler(t *testing.T, comm *client.Mock) (*task.Task, *testLogDirectoryHandler) {
	tsk := &task.Task{
		Project: "project",
		Id:      utility.RandomString(),
		TaskOutputInfo: &taskoutput.TaskOutput{
			TestLogs: taskoutput.TestLogOutput{
				Version: 1,
				BucketConfig: evergreen.BucketConfig{
					Name: t.TempDir(),
					Type: evergreen.BucketTypeLocal,
				},
			},
		},
	}
	logger, err := comm.GetLoggerProducer(context.TODO(), tsk, nil)
	require.NoError(t, err)
	h := newTestLogDirectoryHandler(t.TempDir(), tsk.TaskOutputInfo, taskoutput.TaskOptions{
		ProjectID: tsk.Project,
		TaskID:    tsk.Id,
		Execution: tsk.Execution,
	}, logger)

	return tsk, h.(*testLogDirectoryHandler)
}

func setupCedarServer(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockCedarServer {
	srv, err := timberutil.NewMockCedarServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}
