package send

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferedSend(t *testing.T) {
	s, err := NewInternalLogger("buffs", LevelInfo{level.Debug, level.Debug})
	require.NoError(t, err)

	t.Run("RespectsPriority", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)
		defer bs.cancel()

		bs.Send(message.ConvertToComposer(level.Trace, fmt.Sprintf("should not send")))
		assert.Empty(t, bs.buffer)
		_, ok := s.GetMessageSafe()
		assert.False(t, ok)
	})
	t.Run("FlushesAtCapactiy", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)
		defer bs.cancel()

		for i := 0; i < 12; i++ {
			require.Len(t, bs.buffer, i%10)
			bs.Send(message.ConvertToComposer(level.Debug, fmt.Sprintf("message %d", i+1)))
		}
		assert.Len(t, bs.buffer, 2)
		msg, ok := s.GetMessageSafe()
		require.True(t, ok)
		msgs := strings.Split(msg.Message.String(), "\n")
		assert.Len(t, msgs, 10)
		for i, msg := range msgs {
			require.Equal(t, fmt.Sprintf("message %d", i+1), msg)
		}
	})
	t.Run("FlushesOnInterval", func(t *testing.T) {
		bs := newBufferedSender(s, 5*time.Second, 10)
		defer bs.cancel()

		bs.Send(message.ConvertToComposer(level.Debug, "should flush"))
		time.Sleep(6 * time.Second)
		bs.mu.Lock()
		assert.True(t, time.Since(bs.lastFlush) <= 2*time.Second)
		bs.mu.Unlock()
		msg, ok := s.GetMessageSafe()
		require.True(t, ok)
		assert.Equal(t, "should flush", msg.Message.String())
	})
	t.Run("ClosedSender", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)
		bs.closed = true
		defer bs.cancel()

		bs.Send(message.ConvertToComposer(level.Debug, "should not send"))
		assert.Empty(t, bs.buffer)
		_, ok := s.GetMessageSafe()
		assert.False(t, ok)
	})
}

func TestFlush(t *testing.T) {
	s, err := NewInternalLogger("buffs", LevelInfo{level.Debug, level.Debug})
	require.NoError(t, err)

	t.Run("ForceFlush", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)
		defer bs.cancel()

		bs.Send(message.ConvertToComposer(level.Debug, "message"))
		assert.Len(t, bs.buffer, 1)
		require.NoError(t, bs.Flush(context.TODO()))
		bs.mu.Lock()
		assert.True(t, time.Since(bs.lastFlush) <= time.Second)
		bs.mu.Unlock()
		assert.Empty(t, bs.buffer)
		msg, ok := s.GetMessageSafe()
		require.True(t, ok)
		assert.Equal(t, "message", msg.Message.String())
	})
	t.Run("ClosedSender", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)
		bs.buffer = append(bs.buffer, message.ConvertToComposer(level.Debug, "message"))
		bs.cancel()
		bs.closed = true

		assert.NoError(t, bs.Flush(context.TODO()))
		assert.Len(t, bs.buffer, 1)
		_, ok := s.GetMessageSafe()
		assert.False(t, ok)
	})
}

func TestBufferedClose(t *testing.T) {
	s, err := NewInternalLogger("buffs", LevelInfo{level.Debug, level.Debug})
	require.NoError(t, err)

	t.Run("EmptyBuffer", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)

		assert.Nil(t, bs.Close())
		assert.True(t, bs.closed)
		_, ok := s.GetMessageSafe()
		assert.False(t, ok)
	})
	t.Run("NonEmptyBuffer", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)
		bs.buffer = append(
			bs.buffer,
			message.ConvertToComposer(level.Debug, "message1"),
			message.ConvertToComposer(level.Debug, "message2"),
			message.ConvertToComposer(level.Debug, "message3"),
		)

		assert.Nil(t, bs.Close())
		assert.True(t, bs.closed)
		assert.Empty(t, bs.buffer)
		msgs, ok := s.GetMessageSafe()
		require.True(t, ok)
		assert.Equal(t, "message1\nmessage2\nmessage3", msgs.Message.String())
	})
	t.Run("NoopWhenClosed", func(t *testing.T) {
		bs := newBufferedSender(s, time.Minute, 10)

		assert.NoError(t, bs.Close())
		assert.True(t, bs.closed)
		assert.NoError(t, bs.Close())
	})
}

func TestIntervalFlush(t *testing.T) {
	s, err := NewInternalLogger("buffs", LevelInfo{level.Debug, level.Debug})
	require.NoError(t, err)

	t.Run("ReturnsWhenClosed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		bs := &bufferedSender{
			Sender: s,
			buffer: []message.Composer{},
			cancel: cancel,
		}
		canceled := make(chan bool)

		go func() {
			bs.intervalFlush(ctx, time.Minute)
			canceled <- true
		}()
		assert.NoError(t, bs.Close())
		assert.True(t, <-canceled)
	})
}

func newBufferedSender(sender Sender, interval time.Duration, size int) *bufferedSender {
	bs := NewBufferedSender(sender, interval, size)
	return bs.(*bufferedSender)
}
