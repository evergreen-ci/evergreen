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
	tsk, h := setupTestTestLogDirectoryHandler(t, comm)
	require.NoError(t, h.start(ctx, t.TempDir()))

	// Track files written to the test log directory.
	type logFile struct {
		fn       string
		f        *os.File
		rawLines []string
	}
	var files []logFile
	for i := 0; i < 2; i++ {
		var err error
		file := logFile{fn: utility.RandomString()}
		file.f, err = os.Create(filepath.Join(h.dir, file.fn))
		require.NoError(t, err)

		files = append(files, file)
	}

	// Set up asynchronous test log file writing.
	writerCtx, writerCancel := context.WithCancel(ctx)
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer func() {
			for _, file := range files {
				file.f.Close()
			}
			wg.Done()
		}()

		for {
			select {
			case <-writerCtx.Done():
				return
			default:
				mu.Lock()
				for i := range files {
					files[i].rawLines = append(
						files[i].rawLines,
						fmt.Sprintf("%s %s.", utility.RandomString(), utility.RandomString()),
					)

					_, err := files[i].f.WriteString(files[i].rawLines[len(files[i].rawLines)-1] + "\n")
					require.NoError(t, err)
				}
				mu.Unlock()
			}
		}
	}()

	// Check that logs are ingested continuously.
	time.Sleep(5 * time.Second)
	mu.Lock()
	for _, file := range files {
		it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{file.fn}})
		require.NoError(t, err)
		assert.True(t, it.Next())
		assert.NoError(t, it.Close())
	}
	mu.Unlock()

	// Add a third, nested, test log file.
	nestedFile := logFile{
		fn: filepath.Join(utility.RandomString(), utility.RandomString()),
		rawLines: []string{
			"This is a small test log.",
			"With only a few lines.",
			"In a nested directory.",
		},
	}
	require.NoError(t, os.Mkdir(filepath.Join(h.dir, filepath.Dir(nestedFile.fn)), 0777))
	//require.NoError(t, os.WriteFile(filepath.Join(h.dir, nestedFile.fn), []byte(strings.Join(nestedFile.rawLines, "\n")+"\n"), 0777))
	var err error
	nestedFile.f, err = os.Create(filepath.Join(h.dir, nestedFile.fn))
	require.NoError(t, err)
	_, err = nestedFile.f.WriteString(strings.Join(nestedFile.rawLines, "\n") + "\n")
	require.NoError(t, err)

	// Non-atomic line write.
	firstPart := "This is the first part of the line, "
	_, err = nestedFile.f.WriteString(firstPart)
	require.NoError(t, err)
	time.Sleep(time.Second)
	secondPart := "this is the second part of the line."
	_, err = nestedFile.f.WriteString(secondPart + "\n")
	require.NoError(t, err)
	nestedFile.rawLines = append(nestedFile.rawLines, firstPart+secondPart)

	// Wait for async writing to exit cleanly and close the test log
	// directory handler.
	time.Sleep(time.Second)
	writerCancel()
	wg.Wait()
	require.NoError(t, h.close(ctx))

	// Check persisted test logs.
	for _, file := range append(files, nestedFile) {
		it, err := tsk.GetTestLogs(ctx, taskoutput.TestLogGetOptions{LogPaths: []string{file.fn}})
		require.NoError(t, err)

		var persistedRawLines []string
		for it.Next() {
			persistedRawLines = append(persistedRawLines, it.Item().Data)
		}
		assert.NoError(t, it.Close())
		require.NoError(t, it.Err())
		assert.Equal(t, file.rawLines, persistedRawLines)
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
			require.NoError(t, h.start(ctx, t.TempDir()))

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

			// Sleep before continuing to avoid racing to detect
			// the new file.
			time.Sleep(time.Second)
			require.NoError(t, h.close(ctx))
			assert.Equal(t, expectedSpec, h.spec)

			// Check that only log files are followed.
			assert.Equal(t, 1, h.followFileCount)

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
	logger, err := comm.GetLoggerProducer(context.TODO(), client.TaskData{ID: tsk.Id}, nil)
	require.NoError(t, err)
	h := newTestLogDirectoryHandler(tsk.TaskOutputInfo.TestLogs, taskoutput.TaskOptions{
		ProjectID: tsk.Project,
		TaskID:    tsk.Id,
		Execution: tsk.Execution,
	}, logger)

	return tsk, h
}

func setupCedarServer(ctx context.Context, t *testing.T, comm *client.Mock) *timberutil.MockCedarServer {
	srv, err := timberutil.NewMockCedarServer(ctx, serviceutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	comm.CedarGRPCConn = conn
	return srv
}
