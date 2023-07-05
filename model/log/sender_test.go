package log

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("WriteError", func(t *testing.T) {
		writer := &testLogWriter{hasErr: true}
		s := createTestSender(ctx, writer.write)
		s.opts.MaxBufferSize = 4096

		m := message.ConvertToComposer(level.Info, "going to be an error")
		s.Send(m)
		require.NotEmpty(t, s.buffer)

		assert.Error(t, s.Flush(ctx))
		assert.NotEmpty(t, s.buffer)
		assert.NotZero(t, s.bufferSize)
		assert.Nil(t, writer.lines)
	})
	t.Run("ClosedSender", func(t *testing.T) {
		writer := &testLogWriter{}
		s := createTestSender(ctx, writer.write)
		s.opts.MaxBufferSize = 4096

		m := message.ConvertToComposer(level.Info, "last line")
		s.Send(m)
		require.NotEmpty(t, s.buffer)
		s.closed = true

		require.NoError(t, s.Flush(ctx))
		assert.NotEmpty(t, s.buffer)
		assert.NotZero(t, s.bufferSize)
		assert.Nil(t, writer.lines)
	})
	t.Run("ForcedFlush", func(t *testing.T) {
		writer := &testLogWriter{}
		s := createTestSender(ctx, writer.write)
		s.opts.MaxBufferSize = 4096

		m := message.ConvertToComposer(level.Info, "some message")
		s.Send(m)
		require.NotEmpty(t, s.buffer)

		require.NoError(t, s.Flush(ctx))
		assert.Empty(t, s.buffer)
		assert.Zero(t, s.bufferSize)
		assert.WithinDuration(t, time.Now(), s.lastFlush, time.Second)
		require.Len(t, writer.lines, 1)
		assert.Equal(t, level.Info, writer.lines[0].Priority)
		assert.WithinDuration(t, time.Now(), time.Unix(0, writer.lines[0].Timestamp), time.Second)
		assert.Equal(t, "some message", writer.lines[0].Data)
	})
}

func createTestSender(ctx context.Context, write logWriter) *sender {
	ctx, cancel := context.WithCancel(ctx)
	return &sender{
		ctx:    ctx,
		cancel: cancel,
		opts: LoggerOptions{
			Parse: func(rawLine string) (LogLine, error) {
				return LogLine{
					Timestamp: time.Now().UnixNano(),
					Data:      rawLine,
				}, nil
			},
		},
		write: write,
		Base:  send.NewBase("test"),
	}
}

/*
type mockSender struct {
	*send.Base
	lastMessage string
}

func (ms *mockSender) Send(m message.Composer) {
	if ms.Level().ShouldLog(m) {
		ms.lastMessage = m.String()
	}
}

func (ms *mockSender) Flush(_ context.Context) error { return nil }
*/

type testLogWriter struct {
	lines  []LogLine
	hasErr bool
}

func (w *testLogWriter) write(ctx context.Context, lines []LogLine) error {
	if w.hasErr {
		return errors.New("write error")
	}
	w.lines = append(w.lines, lines...)

	return nil
}
