package taskoutput

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("ParseError", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{}, errors.New("parse error")
		}

		m := message.ConvertToComposer(level.Info, "not going to make it")
		mock.sender.Send(m)
		assert.Empty(t, mock.sender.buffer)
		assert.Zero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
		assert.Contains(t, mock.local.lastMessage, "parsing log line")
	})
	t.Run("InvalidLogLinePriority", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{
				Priority:  -1,
				Timestamp: time.Now().UnixNano(),
				Data:      rawLine,
			}, nil
		}

		m := message.ConvertToComposer(level.Info, "not going to make it")
		mock.sender.Send(m)
		assert.Empty(t, mock.sender.buffer)
		assert.Zero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
		assert.Contains(t, mock.local.lastMessage, "invalid log line priority")
	})
	t.Run("InvalidLogLineTimestamp", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{
				Priority:  level.Info,
				Timestamp: -1,
				Data:      rawLine,
			}, nil
		}

		m := message.ConvertToComposer(level.Info, "not going to make it")
		mock.sender.Send(m)
		assert.Empty(t, mock.sender.buffer)
		assert.Zero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
		assert.Contains(t, mock.local.lastMessage, "invalid log line timestamp")
	})
	t.Run("WriteError", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.service.hasWriteErr = true
		mock.sender.opts.MaxBufferSize = 1

		m := message.ConvertToComposer(level.Info, "not going to make it")
		mock.sender.Send(m)
		assert.NotEmpty(t, mock.sender.buffer)
		assert.NotZero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
		assert.Contains(t, mock.local.lastMessage, "write error")
	})
	t.Run("ClosedSender", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.closed = true

		m := message.ConvertToComposer(level.Info, "not going to make it")
		mock.sender.Send(m)
		assert.Empty(t, mock.sender.buffer)
		assert.Zero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
		assert.Contains(t, mock.local.lastMessage, "closed sender")
	})
	t.Run("SplitsAndParsesData", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		ts := time.Now().UnixNano()
		mock.sender.opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{
				Priority:  level.Error,
				Timestamp: ts,
				Data:      rawLine,
			}, nil
		}

		rawLines := []string{"Hello world!", "This is a log line.", "Goodbye world!"}
		m := message.ConvertToComposer(level.Info, strings.Join(rawLines, "\n"))
		mock.sender.Send(m)
		assert.Empty(t, mock.local.lastMessage)
		require.Len(t, mock.sender.buffer, len(rawLines))
		for i, line := range mock.sender.buffer {
			require.Equal(t, level.Error, line.Priority)
			require.Equal(t, ts, line.Timestamp)
			require.Equal(t, rawLines[i], line.Data)
		}
	})
	t.Run("RespectsMessagePriority", func(t *testing.T) {
		mock := newSenderTestMock(ctx)

		require.NoError(t, mock.sender.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Emergency}))
		m := message.ConvertToComposer(level.Alert, "alert")
		mock.sender.Send(m)
		assert.Empty(t, mock.sender.buffer)
		assert.Empty(t, mock.local.lastMessage)
		m = message.ConvertToComposer(level.Emergency, "emergency")
		mock.sender.Send(m)
		require.Len(t, mock.sender.buffer, 1)
		assert.Equal(t, level.Emergency, mock.sender.buffer[0].Priority)
		assert.Empty(t, mock.local.lastMessage)

		require.NoError(t, mock.sender.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Debug}))
		m = message.ConvertToComposer(level.Debug, "debug")
		mock.sender.Send(m)
		require.Len(t, mock.sender.buffer, 2)
		assert.Equal(t, level.Debug, mock.sender.buffer[1].Priority)
		assert.Empty(t, mock.local.lastMessage)
	})
	t.Run("SetsDefaultLogLineTimestamp", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{
				Priority: level.Info,
				Data:     rawLine,
			}, nil
		}

		rawLines := []string{"Hello world!", "This is a log line.", "Goodbye world!"}
		m := message.ConvertToComposer(level.Info, strings.Join(rawLines, "\n"))
		mock.sender.Send(m)
		require.Len(t, mock.sender.buffer, len(rawLines))
		assert.WithinDuration(t, time.Now(), time.Unix(0, mock.sender.buffer[0].Timestamp), time.Second)
		for i := 1; i < len(mock.sender.buffer); i++ {
			require.Equal(t, mock.sender.buffer[0].Timestamp, mock.sender.buffer[i].Timestamp)
		}
	})
	t.Run("FlushesAtCapacity", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.opts.MaxBufferSize = 4096

		var rawLines []string
		for mock.sender.bufferSize < mock.sender.opts.MaxBufferSize {
			// Create two lines, each with a length of 128
			// characters and create a single message.
			rawLines = append(rawLines, utility.MakeRandomString(64), utility.MakeRandomString(64))
			m := message.ConvertToComposer(level.Debug, strings.Join(rawLines[len(rawLines)-2:], "\n"))

			mock.sender.Send(m)
			require.Len(t, mock.sender.buffer, len(rawLines))
			require.Equal(t, 128*len(rawLines), mock.sender.bufferSize)
			require.Empty(t, mock.local.lastMessage)
		}

		rawLines = append(rawLines, "overflow")
		m := message.ConvertToComposer(level.Debug, rawLines[len(rawLines)-1])
		mock.sender.Send(m)
		assert.Empty(t, mock.local.lastMessage)
		require.Len(t, mock.service.lines, len(rawLines))
		for i, line := range mock.service.lines {
			require.Equal(t, rawLines[i], line.Data)
		}
	})
	t.Run("FlushesAtInterval", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.opts.FlushInterval = 2 * time.Second

		m := message.ConvertToComposer(level.Debug, utility.RandomString())
		mock.sender.Send(m)
		require.NotEmpty(t, mock.sender.buffer)
		go mock.sender.timedFlush()
		time.Sleep(3 * time.Second)
		mock.sender.mu.Lock()
		require.Empty(t, mock.sender.buffer)
		mock.sender.mu.Unlock()
		require.Len(t, mock.service.lines, 1)
		assert.Equal(t, m.String(), mock.service.lines[0].Data)
		assert.Empty(t, mock.local.lastMessage)

		// Should reset the flush interval and flush again after 2
		// seconds.
		m = message.ConvertToComposer(level.Debug, utility.RandomString())
		mock.sender.Send(m)
		mock.sender.mu.Lock()
		require.NotEmpty(t, mock.sender.buffer)
		mock.sender.mu.Unlock()
		time.Sleep(3 * time.Second)
		mock.sender.mu.Lock()
		require.Empty(t, mock.sender.buffer)
		mock.sender.mu.Unlock()
		require.Len(t, mock.service.lines, 2)
		assert.Equal(t, m.String(), mock.service.lines[1].Data)
		assert.Empty(t, mock.local.lastMessage)
	})
}

func TestFlush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("WriteError", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.service.hasWriteErr = true

		m := message.ConvertToComposer(level.Info, "going to be an error")
		mock.sender.Send(m)
		require.NotEmpty(t, mock.sender.buffer)

		assert.Error(t, mock.sender.Flush(ctx))
		assert.NotEmpty(t, mock.sender.buffer)
		assert.NotZero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
	})
	t.Run("ClosedSender", func(t *testing.T) {
		mock := newSenderTestMock(ctx)

		m := message.ConvertToComposer(level.Info, "not going to make it")
		mock.sender.Send(m)
		assert.NotEmpty(t, mock.sender.buffer)
		mock.sender.closed = true

		require.NoError(t, mock.sender.Flush(ctx))
		assert.NotEmpty(t, mock.sender.buffer)
		assert.NotZero(t, mock.sender.bufferSize)
		assert.Empty(t, mock.service.lines)
	})
	t.Run("PersistsParsedData", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		ts := time.Now().UnixNano()
		mock.sender.opts.parse = func(rawLine string) (log.LogLine, error) {
			return log.LogLine{
				Timestamp: ts,
				Data:      rawLine,
			}, nil
		}

		m := message.ConvertToComposer(level.Info, "some message")
		mock.sender.Send(m)
		require.NotEmpty(t, mock.sender.buffer)

		require.NoError(t, mock.sender.Flush(ctx))
		assert.Empty(t, mock.sender.buffer)
		assert.Zero(t, mock.sender.bufferSize)
		assert.WithinDuration(t, time.Now(), mock.sender.lastFlush, time.Second)
		require.Len(t, mock.service.lines, 1)
		assert.Equal(t, level.Info, mock.service.lines[0].Priority)
		assert.Equal(t, ts, mock.service.lines[0].Timestamp)
		assert.Equal(t, "some message", mock.service.lines[0].Data)
	})
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("WriteError", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.service.hasWriteErr = true
		mock.sender.buffer = []log.LogLine{{Priority: level.Critical, Timestamp: time.Now().UnixNano(), Data: "last line"}}

		assert.Error(t, mock.sender.Close())
		assert.True(t, mock.sender.closed)
		assert.Equal(t, context.Canceled, mock.sender.ctx.Err())
		assert.Empty(t, mock.service.lines)
	})
	t.Run("CancelsContext", func(t *testing.T) {
		mock := newSenderTestMock(ctx)

		require.NoError(t, mock.sender.Close())
		assert.True(t, mock.sender.closed)
		assert.Equal(t, context.Canceled, mock.sender.ctx.Err())
		assert.Empty(t, mock.service.lines)
	})
	t.Run("FlushesNonEmptyBuffer", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		expectedLines := []log.LogLine{{Priority: level.Critical, Timestamp: time.Now().UnixNano(), Data: "last line"}}
		mock.sender.buffer = expectedLines

		require.NoError(t, mock.sender.Close())
		assert.True(t, mock.sender.closed)
		assert.Equal(t, context.Canceled, mock.sender.ctx.Err())
		require.Len(t, mock.service.lines, 1)
		assert.Equal(t, expectedLines, mock.service.lines)
	})
	t.Run("NoopWhenAlreadyClosed", func(t *testing.T) {
		mock := newSenderTestMock(ctx)
		mock.sender.buffer = []log.LogLine{{Priority: level.Critical, Timestamp: time.Now().UnixNano(), Data: "last line"}}
		mock.sender.closed = true

		require.NoError(t, mock.sender.Close())
		assert.True(t, mock.sender.closed)
		assert.Equal(t, context.Canceled, mock.sender.ctx.Err())
		assert.Empty(t, mock.service.lines)
	})
}

// senderTestMock is a convenience struct for testing that contains a mock
// logging service, Evergreen log sender, and local mock sender.
type senderTestMock struct {
	service *mockLogService
	sender  *evergreenSender
	local   *mockSender
}

// newSenderTestMock returns a mock configured with a mock logging service,
// Evergreen log sender, and a local mock sender. The log sender is configured
// with the mock log service, a basic line parser function, max buffer size of
// 4096, and the mock sender as the local sender. Invidiual tests may require
// further customization of the configuration after calling.
func newSenderTestMock(ctx context.Context) *senderTestMock {
	ctx, cancel := context.WithCancel(ctx)
	svc := &mockLogService{}
	local := &mockSender{Base: send.NewBase("test")}

	return &senderTestMock{
		service: svc,
		local:   local,
		sender: &evergreenSender{
			ctx:    ctx,
			cancel: cancel,
			opts: EvergreenSenderOptions{
				Local:         local,
				MaxBufferSize: 4096,
				parse: func(rawLine string) (log.LogLine, error) {
					return log.LogLine{Data: rawLine}, nil
				},
				appendLines: func(ctx context.Context, lines []log.LogLine) error {
					return svc.Append(ctx, lines)
				},
			},
			Base: send.NewBase("test"),
		},
	}
}

// mockSender implements a mock send.Sender for testing.
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

// mockLogService implements a mock log service for testing.
type mockLogService struct {
	lines       []log.LogLine
	hasWriteErr bool
}

func (s *mockLogService) Append(ctx context.Context, lines []log.LogLine) error {
	if s.hasWriteErr {
		return errors.New("write error")
	}
	s.lines = append(s.lines, lines...)

	return nil
}
