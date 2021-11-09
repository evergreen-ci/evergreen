package systemmetrics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemMetricsWriteCloserWrite(t *testing.T) {
	ctx := context.Background()
	dataOpts := MetricDataOptions{
		Id:         "ID",
		MetricType: "Test",
		Format:     DataFormatFTDC,
	}
	t.Run("UnderBufferSize", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		testString := []byte("small test string")
		n, err := w.Write(testString)
		require.NoError(t, err)
		assert.Equal(t, len(testString), n)
		assert.Nil(t, mc.GetData())
		assert.Equal(t, testString, raw.buffer)
		assert.NoError(t, w.Close())
	})
	t.Run("OverBufferSize", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush:  true,
			MaxBufferSize: 1,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		testString := []byte("large test string")
		n, err := w.Write(testString)
		require.NoError(t, err)
		assert.Equal(t, len(testString), n)
		require.NotNil(t, mc.GetData())
		assert.Equal(t, testString, mc.GetData().Data)
		raw.mu.Lock()
		assert.Equal(t, []byte{}, raw.buffer)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("ExistingDataInBuffer", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		testString1 := []byte("first test string")
		testString2 := []byte("second test string")
		_, err = w.Write(testString1)
		require.NoError(t, err)
		n, err := w.Write(testString2)
		require.NoError(t, err)
		assert.Equal(t, len(testString2), n)
		raw.mu.Lock()
		assert.Equal(t, append(testString1, testString2...), raw.buffer)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("AlreadyClosed", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		require.NoError(t, w.Close())

		n, err := w.Write([]byte("small test string"))
		assert.Error(t, err)
		assert.Equal(t, 0, n)
	})
	t.Run("NoData", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		n, err := w.Write([]byte{})
		require.Error(t, err)
		assert.Equal(t, 0, n)
		assert.NoError(t, w.Close())
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{addErr: true}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush:  true,
			MaxBufferSize: 1,
		})
		require.NoError(t, err)

		n, err := w.Write([]byte("small test string"))
		assert.Error(t, err)
		assert.Equal(t, 0, n)
	})
}

func TestSystemMetricsWriteCloserTimedFlush(t *testing.T) {
	ctx := context.Background()
	dataOpts := MetricDataOptions{
		Id:         "ID",
		MetricType: "Test",
		Format:     DataFormatFTDC,
	}
	t.Run("ValidOutput", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		testString := []byte("small test string")
		_, err = w.Write(testString)
		require.NoError(t, err)
		assert.Nil(t, mc.GetData())
		time.Sleep(2 * time.Second)
		require.NotNil(t, mc.GetData())
		assert.Equal(t, testString, mc.GetData().Data)
		raw.mu.Lock()
		assert.Equal(t, []byte{}, raw.buffer)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("TimedFlushResetsTimer", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		testString := []byte("small test string")
		_, err = w.Write(testString)
		require.NoError(t, err)
		assert.Nil(t, mc.GetData())

		raw.mu.Lock()
		lastFlush := raw.lastFlush
		raw.mu.Unlock()

		time.Sleep(2 * time.Second)
		raw.mu.Lock()
		assert.True(t, raw.lastFlush.After(lastFlush))
		assert.NotNil(t, raw.timer)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("WriteResetsTimer", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			MaxBufferSize: 1,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		time.Sleep(time.Second)
		assert.False(t, time.Since(raw.lastFlush) < time.Second)

		_, err = w.Write([]byte("large test string"))
		require.NoError(t, err)
		raw.mu.Lock()
		assert.True(t, time.Since(raw.lastFlush) < time.Second)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{addErr: true}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		_, err = w.Write([]byte("smaller than buf"))
		require.NoError(t, err)
		time.Sleep(2 * time.Second)
		_, err = w.Write([]byte("random string"))
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "writer already closed due to error"))
		err = w.Close()
		assert.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "writer already closed due to error"))
		raw.mu.Lock()
		assert.True(t, raw.closed)
		raw.mu.Unlock()
	})
}

func TestSystemMetricsWriteCloserClose(t *testing.T) {
	ctx := context.Background()
	dataOpts := MetricDataOptions{
		Id:         "ID",
		MetricType: "Test",
		Format:     DataFormatFTDC,
	}
	t.Run("NoError", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		testString := []byte("small test string")
		_, err = w.Write(testString)
		require.NoError(t, err)
		assert.Nil(t, mc.GetData())

		require.NoError(t, w.Close())
		require.NotNil(t, mc.GetData())
		assert.Equal(t, testString, mc.GetData().Data)
	})
	t.Run("AlreadyClosed", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{
			client: mc,
		}
		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		raw.closed = true
		assert.Error(t, w.Close())
	})
	t.Run("NoTimedFlushAfterClose", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		require.NoError(t, w.Close())
		lastFlush := raw.lastFlush
		time.Sleep(2 * time.Second)
		assert.Equal(t, lastFlush, raw.lastFlush)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{addErr: true}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		_, err = w.Write([]byte("some data"))
		require.NoError(t, err)
		assert.Error(t, w.Close())
	})
}
