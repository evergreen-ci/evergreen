package log

import (
	"context"
	"strings"
	"testing"
	"time"

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

	t.Run("WriteError", func(t *testing.T) {
		service := &mockService{hasErr: true}
		s, local := createTestSender(ctx, service.write)
		s.opts.MaxBufferSize = 1

		m := message.ConvertToComposer(level.Info, "not going to make it")
		s.Send(m)
		assert.NotEmpty(t, s.buffer)
		assert.NotZero(t, s.bufferSize)
		assert.Nil(t, service.lines)
		assert.Contains(t, local.lastMessage, "writing log")
	})
	t.Run("ClosedSender", func(t *testing.T) {
		service := &mockService{}
		s, local := createTestSender(ctx, service.write)
		s.closed = true

		m := message.ConvertToComposer(level.Info, "not going to make it")
		s.Send(m)
		assert.Empty(t, s.buffer)
		assert.Zero(t, s.bufferSize)
		assert.Nil(t, service.lines)
		assert.Contains(t, local.lastMessage, "closed Sender")
	})
	t.Run("SplitsAndParsesData", func(t *testing.T) {
		service := &mockService{}
		s, local := createTestSender(ctx, service.write)

		rawLines := []string{"Hello world!", "This is a log line.", "Goodbye world!"}
		m := message.ConvertToComposer(level.Info, strings.Join(rawLines, "\n"))
		s.Send(m)
		require.Len(t, s.buffer, len(rawLines))
		for i, line := range s.buffer {
			require.WithinDuration(t, time.Now(), time.Unix(0, line.Timestamp), time.Second)
			require.Equal(t, rawLines[i], line.Data)
			require.Empty(t, local.lastMessage)
		}
	})
	t.Run("RespectsPriority", func(t *testing.T) {
		service := &mockService{}
		s, local := createTestSender(ctx, service.write)

		require.NoError(t, s.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Emergency}))
		m := message.ConvertToComposer(level.Alert, "alert")
		s.Send(m)
		assert.Empty(t, s.buffer)
		assert.Empty(t, local.lastMessage)
		m = message.ConvertToComposer(level.Emergency, "emergency")
		s.Send(m)
		require.NotEmpty(t, s.buffer)
		assert.Equal(t, level.Emergency, s.buffer[len(s.buffer)-1].Priority)
		assert.Equal(t, m.String(), s.buffer[len(s.buffer)-1].Data)
		assert.Empty(t, local.lastMessage)

		require.NoError(t, s.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Debug}))
		m = message.ConvertToComposer(level.Debug, "debug")
		s.Send(m)
		require.NotEmpty(t, s.buffer)
		assert.Equal(t, level.Debug, s.buffer[len(s.buffer)-1].Priority)
		assert.EqualValues(t, m.String(), s.buffer[len(s.buffer)-1].Data)
		assert.Empty(t, local.lastMessage)
	})
	t.Run("FlushesAtCapacity", func(t *testing.T) {
		service := &mockService{}
		s, local := createTestSender(ctx, service.write)
		s.opts.MaxBufferSize = 4096

		var rawLines []string
		for s.bufferSize < s.opts.MaxBufferSize {
			// Create two lines, each with a length of 128
			// characters and create a single message.
			rawLines = append(rawLines, utility.MakeRandomString(64), utility.MakeRandomString(64))
			m := message.ConvertToComposer(level.Debug, strings.Join(rawLines[len(rawLines)-2:], "\n"))

			s.Send(m)
			require.Len(t, s.buffer, len(rawLines))
			require.Equal(t, 128*len(rawLines), s.bufferSize)
			require.Empty(t, local.lastMessage)
		}

		rawLines = append(rawLines, "overflow")
		m := message.ConvertToComposer(level.Debug, rawLines[len(rawLines)-1])
		s.Send(m)
		assert.Empty(t, local.lastMessage)
		require.Len(t, service.lines, len(rawLines))
		for i, line := range service.lines {
			require.Equal(t, rawLines[i], line.Data)
		}
	})
	t.Run("FlushesAtInterval", func(t *testing.T) {
		service := &mockService{}
		s, local := createTestSender(ctx, service.write)
		s.opts.FlushInterval = 2 * time.Second

		m := message.ConvertToComposer(level.Debug, utility.RandomString())
		s.Send(m)
		require.NotEmpty(t, s.buffer)
		go s.timedFlush()
		time.Sleep(2 * time.Second)
		s.mu.Lock()
		require.Empty(t, s.buffer)
		s.mu.Unlock()
		require.Len(t, service.lines, 1)
		assert.Equal(t, m.String(), service.lines[0].Data)
		assert.Empty(t, local.lastMessage)

		// Should reset the flush interval and flush again after 2
		// seconds.
		m = message.ConvertToComposer(level.Debug, utility.RandomString())
		s.Send(m)
		s.mu.Lock()
		require.NotEmpty(t, s.buffer)
		s.mu.Unlock()
		time.Sleep(2 * time.Second)
		s.mu.Lock()
		require.Empty(t, s.buffer)
		s.mu.Unlock()
		require.Len(t, service.lines, 2)
		assert.Equal(t, m.String(), service.lines[1].Data)
		assert.Empty(t, local.lastMessage)
	})
}

func TestFlush(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("WriteError", func(t *testing.T) {
		service := &mockService{hasErr: true}
		s, _ := createTestSender(ctx, service.write)

		m := message.ConvertToComposer(level.Info, "going to be an error")
		s.Send(m)
		require.NotEmpty(t, s.buffer)

		assert.Error(t, s.Flush(ctx))
		assert.NotEmpty(t, s.buffer)
		assert.NotZero(t, s.bufferSize)
		assert.Empty(t, service.lines)
	})
	t.Run("ClosedSender", func(t *testing.T) {
		service := &mockService{}
		s, _ := createTestSender(ctx, service.write)

		m := message.ConvertToComposer(level.Info, "not going to make it")
		s.Send(m)
		assert.NotEmpty(t, s.buffer)
		s.closed = true

		require.NoError(t, s.Flush(ctx))
		assert.NotEmpty(t, s.buffer)
		assert.NotZero(t, s.bufferSize)
		assert.Empty(t, service.lines)
	})
	t.Run("PersistsParsedData", func(t *testing.T) {
		service := &mockService{}
		s, _ := createTestSender(ctx, service.write)

		m := message.ConvertToComposer(level.Info, "some message")
		s.Send(m)
		require.NotEmpty(t, s.buffer)

		require.NoError(t, s.Flush(ctx))
		assert.Empty(t, s.buffer)
		assert.Zero(t, s.bufferSize)
		assert.WithinDuration(t, time.Now(), s.lastFlush, time.Second)
		require.Len(t, service.lines, 1)
		assert.Equal(t, level.Info, service.lines[0].Priority)
		assert.WithinDuration(t, time.Now(), time.Unix(0, service.lines[0].Timestamp), time.Second)
		assert.Equal(t, "some message", service.lines[0].Data)
	})
}

func TestClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("WriteError", func(t *testing.T) {
		writer := &mockService{hasErr: true}
		s, _ := createTestSender(ctx, writer.write)
		s.buffer = []LogLine{{Priority: level.Critical, Timestamp: time.Now().UnixNano(), Data: "last line"}}

		assert.Error(t, s.Close())
		assert.True(t, s.closed)
		assert.Equal(t, context.Canceled, s.ctx.Err())
		assert.Empty(t, writer.lines)
	})
	t.Run("EmptyBuffer", func(t *testing.T) {
		writer := &mockService{}
		s, _ := createTestSender(ctx, writer.write)

		require.NoError(t, s.Close())
		assert.True(t, s.closed)
		assert.Equal(t, context.Canceled, s.ctx.Err())
		assert.Empty(t, writer.lines)
	})
	t.Run("FlushesNonEmptyBuffer", func(t *testing.T) {
		writer := &mockService{}
		s, _ := createTestSender(ctx, writer.write)
		expectedLines := []LogLine{{Priority: level.Critical, Timestamp: time.Now().UnixNano(), Data: "last line"}}
		s.buffer = expectedLines

		require.NoError(t, s.Close())
		assert.True(t, s.closed)
		assert.Equal(t, context.Canceled, s.ctx.Err())
		require.Len(t, writer.lines, 1)
		assert.Equal(t, expectedLines, writer.lines)
	})
	t.Run("NoopWhenAlreadyClosed", func(t *testing.T) {
		writer := &mockService{}
		s, _ := createTestSender(ctx, writer.write)
		s.buffer = []LogLine{{Priority: level.Critical, Timestamp: time.Now().UnixNano(), Data: "last line"}}
		s.closed = true

		require.NoError(t, s.Close())
		assert.True(t, s.closed)
		assert.Equal(t, context.Canceled, s.ctx.Err())
		assert.Empty(t, writer.lines)
	})
}

func createTestSender(ctx context.Context, write logWriter) (*sender, *mockSender) {
	ctx, cancel := context.WithCancel(ctx)
	local := &mockSender{Base: send.NewBase("test")}
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
			Local:         local,
			MaxBufferSize: 4096,
		},
		write: write,
		Base:  send.NewBase("test"),
	}, local
}

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

type mockService struct {
	lines  []LogLine
	hasErr bool
}

func (s *mockService) write(ctx context.Context, lines []LogLine) error {
	if s.hasErr {
		return errors.New("write error")
	}
	s.lines = append(s.lines, lines...)

	return nil
}
