package taskexec

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
