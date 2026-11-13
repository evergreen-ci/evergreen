package taskoutput

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/agent/internal/redactor"
	"github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/s3usage"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestAppendTestLog(t *testing.T) {
	ctx := t.Context()

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

	initedS3Usage := &s3usage.S3Usage{}
	initedS3Usage.Init()

	for _, testCase := range []struct {
		name           string
		input          []string
		expectedOutput []string
		redactOpts     redactor.RedactionOptions
		s3Usage        *s3usage.S3Usage
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
		{
			name:           "NonNilS3Usage",
			input:          []string{"log line 1"},
			expectedOutput: []string{"log line 1"},
			s3Usage:        initedS3Usage,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tsk.TaskOutputInfo.TestLogs.BucketConfig.Name = t.TempDir()
			testLog.Lines = testCase.input

			require.NoError(t, AppendTestLog(ctx, tsk, testCase.redactOpts, testLog, testCase.s3Usage))
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
	ctx := t.Context()

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

func TestTestLogDirectoryHandlerSymlink(t *testing.T) {
	ctx := t.Context()

	comm := client.NewMock("url")

	t.Run("SymlinkedFileIngestedInFull", func(t *testing.T) {
		tsk, h := setupTestTestLogDirectoryHandler(t, comm, redactor.RedactionOptions{}, 32)

		targetDir := t.TempDir()
		targetPath := filepath.Join(targetDir, "real.log")
		inputLines := []string{
			"Line 1 of the symlinked log file content.",
			"Line 2 with more bytes to cross sequence size.",
			"Line 3 keeps going past the seqSize threshold.",
			"Line 4 is the last line of the target file.",
		}
		require.NoError(t, os.WriteFile(targetPath, []byte(strings.Join(inputLines, "\n")+"\n"), 0777))
		require.NoError(t, os.Symlink(targetPath, filepath.Join(h.dir, "symlink.log")))

		require.NoError(t, h.run(ctx))

		it, err := tsk.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{"symlink.log"}})
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, it.Close()) })
		var persistedLines []string
		for it.Next() {
			persistedLines = append(persistedLines, it.Item().Data)
		}
		require.NoError(t, it.Err())
		assert.Equal(t, inputLines, persistedLines)
	})

	t.Run("BrokenSymlinkSkipped", func(t *testing.T) {
		tsk, h := setupTestTestLogDirectoryHandler(t, comm, redactor.RedactionOptions{}, 32)

		require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "does_not_exist"), filepath.Join(h.dir, "broken.log")))

		require.NoError(t, h.run(ctx))

		it, err := tsk.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{"broken.log"}})
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, it.Close()) })
		assert.False(t, it.Next(), "broken symlink should produce no ingested lines")
		require.NoError(t, it.Err())
	})

	t.Run("DirectorySymlinkSkippedWithWarning", func(t *testing.T) {
		tsk, h := setupTestTestLogDirectoryHandler(t, comm, redactor.RedactionOptions{}, 32)

		targetDir := t.TempDir()
		require.NoError(t, os.Symlink(targetDir, filepath.Join(h.dir, "dir_link")))

		require.NoError(t, h.run(ctx))

		it, err := tsk.GetTestLogs(ctx, task.TestLogGetOptions{LogPaths: []string{"dir_link"}})
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, it.Close()) })
		assert.False(t, it.Next(), "directory symlink should not produce any ingested log content")
		require.NoError(t, it.Err())

		sawWarning := slices.ContainsFunc(comm.GetTaskLogs(tsk.Id), func(line log.LogLine) bool {
			return line.Priority == level.Warning && strings.Contains(line.Data, "targets a directory")
		})
		assert.True(t, sawWarning, "expected a warning explaining the directory symlink was skipped")
	})
}

func TestUploadTestLogFilesForOutput(t *testing.T) {
	comm := client.NewMock("url")

	setup := func(t *testing.T) (task.Task, string, TestLogFilesOptions) {
		runner := &task.Task{Id: "runner", Project: "project"}
		logger, err := comm.GetLoggerProducer(t.Context(), runner, nil)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, logger.Close()) })

		vTask := task.Task{
			Id:      "virtual",
			Project: "project",
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
		dir := filepath.Join(t.TempDir(), "logs")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "job0"), 0777))
		return vTask, dir, TestLogFilesOptions{
			Dir:    dir,
			Task:   vTask,
			Output: vTask.TaskOutputInfo.TestLogs,
			Logger: logger,
		}
	}
	writeLog := func(t *testing.T, path string, lines ...string) {
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0777))
	}
	getLines := func(t *testing.T, vTask task.Task, logPath string) []log.LogLine {
		it, err := vTask.GetTestLogs(t.Context(), task.TestLogGetOptions{LogPaths: []string{logPath}})
		require.NoError(t, err)
		var lines []log.LogLine
		for it.Next() {
			lines = append(lines, it.Item())
		}
		assert.NoError(t, it.Close())
		require.NoError(t, it.Err())
		return lines
	}
	getData := func(lines []log.LogLine) []string {
		var data []string
		for _, line := range lines {
			data = append(data, line.Data)
		}
		return data
	}

	t.Run("UploadsReferencedLogsUnderTask", func(t *testing.T) {
		vTask, dir, opts := setup(t)
		writeLog(t, filepath.Join(dir, "job0", "t.log"), "test log line")
		writeLog(t, filepath.Join(dir, "job0", "global.log"), "global log line")
		opts.LogPaths = []string{"job0/t.log", "job0/global.log"}

		require.NoError(t, UploadTestLogFilesForOutput(t.Context(), opts))

		assert.Equal(t, []string{"test log line"}, getData(getLines(t, vTask, "job0/t.log")))
		assert.Equal(t, []string{"global log line"}, getData(getLines(t, vTask, "job0/global.log")))
	})
	t.Run("OnlyUploadsReferencedLogsWithinDir", func(t *testing.T) {
		vTask, dir, opts := setup(t)
		writeLog(t, filepath.Join(dir, "job0", "t.log"), "test log line")
		writeLog(t, filepath.Join(dir, "unreferenced.log"), "unreferenced log line")
		writeLog(t, filepath.Join(filepath.Dir(dir), "outside.log"), "outside log line")
		writeLog(t, filepath.Join(dir, testLogSpecFilename), "format: text")
		opts.LogPaths = []string{"job0/t.log", "../outside.log", "/etc/passwd", "job0/../../outside.log", "job0/missing.log", testLogSpecFilename}

		require.NoError(t, UploadTestLogFilesForOutput(t.Context(), opts))

		bucketDir := vTask.TaskOutputInfo.TestLogs.BucketConfig.Name
		var keys []string
		require.NoError(t, filepath.WalkDir(bucketDir, func(path string, d os.DirEntry, err error) error {
			require.NoError(t, err)
			if !d.IsDir() {
				rel, err := filepath.Rel(bucketDir, path)
				require.NoError(t, err)
				keys = append(keys, filepath.ToSlash(rel))
			}
			return nil
		}))
		require.NotEmpty(t, keys)
		for _, key := range keys {
			assert.True(t, strings.HasPrefix(key, "project/virtual/0/"), "log '%s' should be uploaded under the virtual task", key)
			assert.Contains(t, key, "job0/t.log", "only the referenced log within the log directory should be uploaded")
		}
	})
	t.Run("RedactsLogs", func(t *testing.T) {
		vTask, dir, opts := setup(t)
		writeLog(t, filepath.Join(dir, "secret.log"), "secret DEADBEEF")
		opts.LogPaths = []string{"secret.log"}
		opts.RedactorOpts = redactor.RedactionOptions{
			Expansions: util.NewDynamicExpansions(map[string]string{"secret_name": "DEADBEEF"}),
			Redacted:   []string{"secret_name"},
		}

		require.NoError(t, UploadTestLogFilesForOutput(t.Context(), opts))

		assert.Equal(t, []string{"secret <REDACTED:secret_name>"}, getData(getLines(t, vTask, "secret.log")))
	})
	t.Run("HonorsSpecFileFormat", func(t *testing.T) {
		vTask, dir, opts := setup(t)
		data, err := yaml.Marshal(&testLogSpec{Format: testLogFormatTextTimestamp})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, testLogSpecFilename), data, 0777))
		ts := time.Now().Add(-time.Hour).UnixNano()
		writeLog(t, filepath.Join(dir, "t.log"), fmt.Sprintf("%d timestamped line", ts))
		opts.LogPaths = []string{"t.log"}

		require.NoError(t, UploadTestLogFilesForOutput(t.Context(), opts))

		lines := getLines(t, vTask, "t.log")
		require.Len(t, lines, 1)
		assert.Equal(t, "timestamped line", lines[0].Data)
		assert.Equal(t, ts, lines[0].Timestamp)
	})
}

func TestTestLogDirectoryHandlerGetSpecFile(t *testing.T) {
	ctx := t.Context()

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
			t.Cleanup(func() {
				assert.NoError(t, it.Close())
			})
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
	logger, err := comm.GetLoggerProducer(t.Context(), tsk, nil)
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
