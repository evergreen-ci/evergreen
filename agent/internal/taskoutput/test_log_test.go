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
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestAppendTestLog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tsk := &task.Task{
		Id:           "id",
		Project:      "project",
		Version:      "version",
		BuildVariant: "build_variant",
		Execution:    5,
		Requester:    evergreen.GithubPRRequester,
		TaskOutputInfo: &task.TaskOutput{
			TestLogs: task.TestLogOutput{
				Version: 1,
				BucketConfig: evergreen.BucketConfig{
					Type: evergreen.BucketTypeLocal,
				},
			},
		},
	}
	testLog := &testlog.TestLog{
		Id:            "id",
		Name:          "test",
		Task:          "task",
		TaskExecution: 5,
	}

	for _, testCase := range []struct {
		name           string
		input          []string
		expectedOutput []string
		redactOpts     redactor.RedactionOptions
	}{
		{
			name:           "Newlines",
			input:          []string{"log line 1\nlog line 2", "log line 3\n", "log line 4"},
			expectedOutput: []string{"log line 1", "log line 2", "log line 3", "log line 4"},
		},
		{
			name:           "Redacted",
			input:          []string{"the secret is: DEADBEEF, and the second secret is: DEADC0DE"},
			expectedOutput: []string{"the secret is: <REDACTED:secret_name>, and the second secret is: <REDACTED:another_secret>"},
			redactOpts: redactor.RedactionOptions{
				Expansions:         util.NewDynamicExpansions(map[string]string{"secret_name": "DEADBEEF"}),
				Redacted:           []string{"secret_name"},
				InternalRedactions: util.NewDynamicExpansions(map[string]string{"another_secret": "DEADC0DE"}),
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tsk.TaskOutputInfo.TestLogs.BucketConfig.Name = t.TempDir()
			testLog.Lines = testCase.input

			require.NoError(t, AppendTestLog(ctx, tsk, testCase.redactOpts, testLog))
			it, err := tsk.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{testLog.Name}})
			require.NoError(t, err)

			var actual []string
			for it.Next() {
				line := it.Item()
				assert.Equal(t, level.Info, line.Priority)
				assert.WithinDuration(t, time.Now(), time.Unix(0, line.Timestamp), time.Second)
				actual = append(actual, line.Data)
			}
			assert.Equal(t, testCase.expectedOutput, actual)
		})
	}
}

func TestTestLogDirectoryHandlerRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comm := client.NewMock("url")

	type logInfo struct {
		logPath       string
		inputLines    []string
		expectedLines []string
	}

	for _, test := range []struct {
		name       string
		logs       []logInfo
		redactOpts redactor.RedactionOptions
	}{
		{
			name: "RecursiveDirectoryWalk",
			logs: []logInfo{
				{
					logPath: "log0.log",
					inputLines: []string{
						"This is a small test log.",
						"With only a few lines.",
					},
				},
				{
					logPath: "log1.log",
					inputLines: []string{
						"This is a another small test log.",
						"With only a few lines.",
						"But it should get there.",
					},
				},
				{
					logPath: "nested/log2.log",
					inputLines: []string{
						"This is a another small test log.",
						"With only a few lines.",
						"But this time it is in nested directory.",
					},
				},
				{
					logPath: "nested/nested/log3.log",
					inputLines: []string{
						"This is a small test log, again.",
						"In an an even more nested directory.",
						"With some a friend!",
					},
				},
				{
					logPath: "nested/nested/log4.log",
					inputLines: []string{
						"This is the last test log.",
						"Blah blah blah blah.",
						"Something something something.",
						"We are done here.",
					},
				},
			},
		},
		{
			name: "RedactedLog",
			logs: []logInfo{
				{
					logPath: "secret.log",
					inputLines: []string{
						"This log contains a big secret: DEADBEEF, and another bigger secret: DEADC0DE",
					},
					expectedLines: []string{
						"This log contains a big secret: <REDACTED:secret_name>, and another bigger secret: <REDACTED:another_secret>",
					},
				},
			},
			redactOpts: redactor.RedactionOptions{
				Expansions:         util.NewDynamicExpansions(map[string]string{"secret_name": "DEADBEEF"}),
				Redacted:           []string{"secret_name"},
				InternalRedactions: util.NewDynamicExpansions(map[string]string{"another_secret": "DEADC0DE"}),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tsk, h := setupTestTestLogDirectoryHandler(t, comm, test.redactOpts, 20)
			for _, li := range test.logs {
				require.NoError(t, os.MkdirAll(filepath.Join(h.dir, filepath.Dir(li.logPath)), 0777))
				require.NoError(t, os.WriteFile(filepath.Join(h.dir, li.logPath), []byte(strings.Join(li.inputLines, "\n")+"\n"), 0777))
			}

			require.NoError(t, h.run(ctx))
			for _, li := range test.logs {
				it, err := tsk.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{li.logPath}})
				require.NoError(t, err)
				var persistedRawLines []string
				for it.Next() {
					persistedRawLines = append(persistedRawLines, it.Item().Data)
				}
				assert.NoError(t, it.Close())
				require.NoError(t, it.Err())

				expectedLines := li.expectedLines
				if expectedLines == nil {
					expectedLines = li.inputLines
				}
				assert.Equal(t, expectedLines, persistedRawLines)
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
		specData any
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
			tsk, h := setupTestTestLogDirectoryHandler(t, comm, redactor.RedactionOptions{}, 0)

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
			it, err := tsk.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{logPath}})
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
		test func(*testing.T, log.LineParser)
	}{
		{
			name: "DefaultFormat",
			spec: testLogSpec{},
			test: func(t *testing.T, parser log.LineParser) {
				assert.Nil(t, parser)
			},
		},
		{
			name: "Text",
			spec: testLogSpec{Format: testLogFormatDefault},
			test: func(t *testing.T, parser log.LineParser) {
				assert.Nil(t, parser)
			},
		},
		{
			name: "TextTimestamp",
			spec: testLogSpec{Format: testLogFormatTextTimestamp},
			test: func(t *testing.T, parser log.LineParser) {
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
				t.Run("ParseRawLineWithJustTimestamp", func(t *testing.T) {
					line, err := parser(fmt.Sprintf("%d\n", ts))
					require.NoError(t, err)
					assert.Zero(t, line.Priority)
					assert.Equal(t, ts, line.Timestamp)
					assert.Empty(t, line.Data)
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

func setupTestTestLogDirectoryHandler(t *testing.T, comm *client.Mock, redactOpts redactor.RedactionOptions, seqSize int64) (*task.Task, *testLogDirectoryHandler) {
	tsk := &task.Task{
		Project: "project",
		Id:      utility.RandomString(),
		TaskOutputInfo: &task.TaskOutput{
			TestLogs: task.TestLogOutput{
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
	handlerOpts := directoryHandlerOpts{
		redactorOpts: redactOpts,
		output:       tsk.TaskOutputInfo,
		tsk:          tsk,
	}
	h := newTestLogDirectoryHandler(t.TempDir(), logger, handlerOpts).(*testLogDirectoryHandler)
	h.sequenceSize = seqSize

	return tsk, h
}
