package taskexec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogFile(t *testing.T) {
	t.Run("WriteAndRead", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")

		lf, err := newLogFile(path)
		require.NoError(t, err)
		defer lf.Close()

		lf.WriteLogLine(3, "hello world")
		lf.WriteLogLine(3, "second line")

		require.NoError(t, lf.Close())

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "[step:3] hello world")
		assert.Contains(t, content, "[step:3] second line")
		lines := strings.Split(strings.TrimSpace(content), "\n")
		assert.Len(t, lines, 2)
	})

	t.Run("StepDelimiters", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")

		lf, err := newLogFile(path)
		require.NoError(t, err)
		defer lf.Close()

		lf.WriteStepStart(5, "shell.exec", "main")
		lf.WriteLogLine(5, "command output")
		lf.WriteStepEnd(5, true, "1.2s")

		require.NoError(t, lf.Close())

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		content := string(data)

		assert.Contains(t, content, "=== STEP 5 START shell.exec (main) ===")
		assert.Contains(t, content, "=== STEP 5 END success=true duration=1.2s ===")
	})
}

func TestLogFilterOptions(t *testing.T) {
	t.Run("FilterByStep", func(t *testing.T) {
		tmpDir := t.TempDir()
		logDir := filepath.Join(tmpDir, "session")
		require.NoError(t, os.MkdirAll(logDir, 0755))

		// Write a test log file with multiple steps
		content := `=== STEP 0 START shell.exec (pre) ===
[2024-01-15T10:30:45.123Z] [step:0] pre command
=== STEP 0 END success=true duration=0.1s ===
=== STEP 1 START shell.exec (main) ===
[2024-01-15T10:30:46.123Z] [step:1] main command
[2024-01-15T10:30:46.456Z] [step:1] main output
=== STEP 1 END success=true duration=1.0s ===
=== STEP 2 START shell.exec (post) ===
[2024-01-15T10:30:47.123Z] [step:2] post command
=== STEP 2 END success=true duration=0.5s ===
`
		require.NoError(t, os.WriteFile(filepath.Join(logDir, "output.log"), []byte(content), 0644))

		lines, err := readLogFileLines(filepath.Join(logDir, "output.log"), LogFilterOptions{
			Step:    1,
			HasStep: true,
		})
		require.NoError(t, err)

		assert.Len(t, lines, 4)
		for _, line := range lines {
			assert.True(t, strings.Contains(line, "STEP 1") || strings.Contains(line, "step:1"),
				"unexpected line: %s", line)
		}
	})

	t.Run("NoFilter", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")
		content := "line1\nline2\nline3\n"
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))

		lines, err := readLogFileLines(path, LogFilterOptions{})
		require.NoError(t, err)
		assert.Len(t, lines, 3)
	})
}

func TestStreamWriter(t *testing.T) {
	t.Run("WriteChannelMessage", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 3)

		err := sw.WriteChannelMessage(TaskChannel, "hello world")
		require.NoError(t, err)

		var line StreamLine
		err = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
		require.NoError(t, err)
		assert.Equal(t, TaskChannel, line.Channel)
		assert.Equal(t, 3, line.Step)
		assert.Equal(t, "hello world", line.Message)
	})

	t.Run("WriteDone", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 5)

		err := sw.WriteDone(true, 1200, 6, "")
		require.NoError(t, err)

		var line StreamLine
		err = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
		require.NoError(t, err)
		assert.Equal(t, DoneChannel, line.Channel)
		assert.Equal(t, 5, line.Step)
		assert.NotNil(t, line.Success)
		assert.True(t, *line.Success)
		assert.NotNil(t, line.DurationMs)
		assert.Equal(t, int64(1200), *line.DurationMs)
		assert.NotNil(t, line.NextStep)
		assert.Equal(t, 6, *line.NextStep)
		assert.Empty(t, line.Error)
	})

	t.Run("WriteDoneWithError", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 2)

		err := sw.WriteDone(false, 500, 2, "command failed")
		require.NoError(t, err)

		var line StreamLine
		err = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
		require.NoError(t, err)
		assert.NotNil(t, line.Success)
		assert.False(t, *line.Success)
		assert.Equal(t, "command failed", line.Error)
	})

	t.Run("SetStep", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 0)

		sw.SetStep(7)
		err := sw.WriteChannelMessage(ExecChannel, "step changed")
		require.NoError(t, err)

		var line StreamLine
		err = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
		require.NoError(t, err)
		assert.Equal(t, 7, line.Step)
	})

	t.Run("MultipleLines", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 1)

		require.NoError(t, sw.WriteChannelMessage(ExecChannel, "starting"))
		require.NoError(t, sw.WriteChannelMessage(TaskChannel, "output line 1"))
		require.NoError(t, sw.WriteChannelMessage(TaskChannel, "output line 2"))
		require.NoError(t, sw.WriteDone(true, 100, 2, ""))

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		assert.Len(t, lines, 4)

		var first StreamLine
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
		assert.Equal(t, ExecChannel, first.Channel)

		var last StreamLine
		require.NoError(t, json.Unmarshal([]byte(lines[3]), &last))
		assert.Equal(t, DoneChannel, last.Channel)
	})
}

func TestStreamingSender(t *testing.T) {
	t.Run("SendWritesToStream", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 0)
		sender := newStreamingSender("test", TaskChannel, sw)

		sender.Send(message.NewDefaultMessage(sender.Level().Default, "test message"))

		var line StreamLine
		err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line)
		require.NoError(t, err)
		assert.Equal(t, TaskChannel, line.Channel)
		assert.Equal(t, "test message", line.Message)
	})

	t.Run("DifferentChannels", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 0)

		taskSender := newStreamingSender("task", TaskChannel, sw)
		execSender := newStreamingSender("exec", ExecChannel, sw)

		taskSender.Send(message.NewDefaultMessage(taskSender.Level().Default, "task msg"))
		execSender.Send(message.NewDefaultMessage(execSender.Level().Default, "exec msg"))

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		require.Len(t, lines, 2)

		var taskLine StreamLine
		require.NoError(t, json.Unmarshal([]byte(lines[0]), &taskLine))
		assert.Equal(t, TaskChannel, taskLine.Channel)

		var execLine StreamLine
		require.NoError(t, json.Unmarshal([]byte(lines[1]), &execLine))
		assert.Equal(t, ExecChannel, execLine.Channel)
	})
}

func TestStreamingLoggerProducer(t *testing.T) {
	t.Run("ProducesDistinctChannels", func(t *testing.T) {
		var buf bytes.Buffer
		sw := newStreamWriter(&buf, nil, nil, 0)
		producer := newStreamingLoggerProducer(sw)

		assert.NotNil(t, producer.Task())
		assert.NotNil(t, producer.Execution())
		assert.NotNil(t, producer.System())
		assert.False(t, producer.Closed())

		require.NoError(t, producer.Close())
		assert.True(t, producer.Closed())
	})
}

func TestStepTimer(t *testing.T) {
	timer := newStepTimer()
	assert.GreaterOrEqual(t, timer.DurationMs(), int64(0))
	assert.NotEmpty(t, timer.DurationString())
}
