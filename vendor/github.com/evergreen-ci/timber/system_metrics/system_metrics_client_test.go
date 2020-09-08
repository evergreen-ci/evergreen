package systemmetrics

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/timber/internal"
	"github.com/evergreen-ci/timber/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type mockClient struct {
	mu        sync.Mutex
	createErr bool
	addErr    bool
	closeErr  bool
	info      *internal.SystemMetrics
	data      *internal.SystemMetricsData
	close     *internal.SystemMetricsSeriesEnd
}

func (mc *mockClient) CreateSystemMetricsRecord(_ context.Context, in *internal.SystemMetrics, opts ...grpc.CallOption) (*internal.SystemMetricsResponse, error) {
	if mc.createErr {
		return nil, errors.New("create error")
	}
	mc.info = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (mc *mockClient) AddSystemMetrics(_ context.Context, in *internal.SystemMetricsData, opts ...grpc.CallOption) (*internal.SystemMetricsResponse, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.addErr {
		return nil, errors.New("add error")
	}
	mc.data = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (mc *mockClient) StreamSystemMetrics(_ context.Context, opts ...grpc.CallOption) (internal.CedarSystemMetrics_StreamSystemMetricsClient, error) {
	return nil, nil
}

func (mc *mockClient) CloseMetrics(_ context.Context, in *internal.SystemMetricsSeriesEnd, opts ...grpc.CallOption) (*internal.SystemMetricsResponse, error) {
	if mc.closeErr {
		return nil, errors.New("close error")
	}
	mc.close = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (mc *mockClient) GetData() *internal.SystemMetricsData {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.data == nil {
		return nil
	}
	ref := *mc.data
	return &ref
}

func TestNewSystemMetricsClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := testutil.NewMockMetricsServer(ctx, 3000)
	require.NoError(t, err)
	t.Run("ValidOptions", func(t *testing.T) {
		connOpts := ConnectionOptions{
			Client:   http.Client{},
			DialOpts: srv.DialOpts,
		}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID", true))
		srv.Mu.Lock()
		defer srv.Mu.Unlock()
		assert.NotNil(t, srv.Close)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		connOpts := ConnectionOptions{}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestNewSystemMetricsClientWithExistingClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := testutil.NewMockMetricsServer(ctx, 3000)
	require.NoError(t, err)
	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)

	t.Run("ValidOptions", func(t *testing.T) {
		client, err := NewSystemMetricsClientWithExistingConnection(ctx, conn)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID", true))
		srv.Mu.Lock()
		defer srv.Mu.Unlock()
		assert.NotNil(t, srv.Close)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		client, err := NewSystemMetricsClientWithExistingConnection(ctx, nil)
		assert.Error(t, err)
		assert.Nil(t, client)
	})
}

func TestCloseClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := testutil.NewMockMetricsServer(ctx, 3000)
	require.NoError(t, err)
	t.Run("WithoutExistingConnection", func(t *testing.T) {
		connOpts := ConnectionOptions{
			Client:   http.Client{},
			DialOpts: srv.DialOpts,
		}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID", true))
		srv.Mu.Lock()
		assert.NotNil(t, srv.Close)
		srv.Mu.Unlock()

		require.NoError(t, client.CloseClient())
		assert.Error(t, client.CloseSystemMetrics(ctx, "ID", true))
	})
	t.Run("WithExistingConnection", func(t *testing.T) {
		conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
		require.NoError(t, err)
		client, err := NewSystemMetricsClientWithExistingConnection(ctx, conn)
		require.NoError(t, err)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID", true))
		srv.Mu.Lock()
		assert.NotNil(t, srv.Close)
		srv.Mu.Unlock()

		require.NoError(t, client.CloseClient())
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID", true))
		require.NoError(t, conn.Close())
	})
	t.Run("AlreadyClosed", func(t *testing.T) {
		connOpts := ConnectionOptions{
			Client:   http.Client{},
			DialOpts: srv.DialOpts,
		}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.NoError(t, err)
		require.NoError(t, client.CloseClient())

		assert.Error(t, client.CloseClient())
	})
}

func TestCreateSystemMetricsRecord(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		id, err := s.CreateSystemMetricsRecord(ctx, SystemMetricsOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "taskname",
			TaskId:      "taskid",
			Execution:   1,
			Mainline:    true,
			Compression: CompressionTypeNone,
			Schema:      SchemaTypeRawEvents,
		})
		require.NoError(t, err)
		assert.Equal(t, id, "ID")
		assert.Equal(t, &internal.SystemMetrics{
			Info: &internal.SystemMetricsInfo{
				Project:   "project",
				Version:   "version",
				Variant:   "variant",
				TaskName:  "taskname",
				TaskId:    "taskid",
				Execution: 1,
				Mainline:  true,
			},
			Artifact: &internal.SystemMetricsArtifactInfo{
				Compression: internal.CompressionType(CompressionTypeNone),
				Schema:      internal.SchemaType(SchemaTypeRawEvents),
			},
		}, mc.info)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		id, err := s.CreateSystemMetricsRecord(ctx, SystemMetricsOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "taskname",
			TaskId:      "taskid",
			Execution:   1,
			Mainline:    true,
			Compression: 6,
			Schema:      SchemaTypeRawEvents,
		})
		assert.Error(t, err)
		assert.Equal(t, id, "")
		assert.Nil(t, mc.GetData())

		id, err = s.CreateSystemMetricsRecord(ctx, SystemMetricsOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "taskname",
			TaskId:      "taskid",
			Execution:   1,
			Mainline:    true,
			Compression: CompressionTypeNone,
			Schema:      6,
		})
		assert.Error(t, err)
		assert.Equal(t, id, "")
		assert.Nil(t, mc.GetData())
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{createErr: true}
		s := &SystemMetricsClient{client: mc}

		id, err := s.CreateSystemMetricsRecord(ctx, SystemMetricsOptions{
			Project:     "project",
			Version:     "version",
			Variant:     "variant",
			TaskName:    "taskname",
			TaskId:      "taskid",
			Execution:   1,
			Mainline:    true,
			Compression: CompressionTypeNone,
			Schema:      SchemaTypeRawEvents,
		})
		assert.Error(t, err)
		assert.Equal(t, id, "")
		assert.Nil(t, mc.GetData())
	})
}

func TestAddSystemMetrics(t *testing.T) {
	ctx := context.Background()
	dataOpts := MetricDataOptions{
		Id:         "ID",
		MetricType: "Test",
		Format:     DataFormatFTDC,
	}
	t.Run("ValidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		require.NoError(t, s.AddSystemMetrics(ctx, dataOpts, []byte("Test byte string")))
		assert.Equal(t, &internal.SystemMetricsData{
			Id:     "ID",
			Type:   "Test",
			Format: internal.DataFormat(DataFormatFTDC),
			Data:   []byte("Test byte string"),
		}, mc.GetData())
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		assert.Error(t, s.AddSystemMetrics(ctx, MetricDataOptions{
			Id:         "",
			MetricType: "Test",
			Format:     DataFormatFTDC,
		}, []byte("Test byte string")))
		assert.Nil(t, mc.GetData())

		assert.Error(t, s.AddSystemMetrics(ctx, MetricDataOptions{
			Id:         "Id",
			MetricType: "",
			Format:     DataFormatFTDC,
		}, []byte("Test byte string")))
		assert.Nil(t, mc.GetData())

		assert.Error(t, s.AddSystemMetrics(ctx, MetricDataOptions{
			Id:         "Id",
			MetricType: "Test",
			Format:     7,
		}, []byte("Test byte string")))
		assert.Nil(t, mc.GetData())

		assert.Error(t, s.AddSystemMetrics(ctx, dataOpts, []byte{}))
		assert.Nil(t, mc.GetData())
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{addErr: true}
		s := &SystemMetricsClient{client: mc}

		assert.Error(t, s.AddSystemMetrics(ctx, dataOpts, []byte("Test byte string")))
		assert.Nil(t, mc.GetData())
	})
}

func TestNewSystemMetricsWriteCloser(t *testing.T) {
	ctx := context.Background()
	dataOpts := MetricDataOptions{
		Id:         "ID",
		MetricType: "Test",
		Format:     DataFormatFTDC,
	}
	t.Run("ValidOpts", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			FlushInterval: time.Second,
			MaxBufferSize: 1e5,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		raw.mu.Lock()
		assert.NotNil(t, raw.ctx)
		assert.NotNil(t, raw.cancel)
		assert.NotNil(t, raw.catcher)
		assert.Equal(t, "ID", raw.opts.Id)
		assert.Equal(t, "Test", raw.opts.MetricType)
		assert.Equal(t, DataFormatFTDC, raw.opts.Format)
		assert.Equal(t, s, raw.client)
		assert.NotNil(t, raw.buffer)
		assert.Equal(t, int(1e5), raw.maxBufferSize)
		assert.NotEqual(t, time.Time{}, raw.lastFlush)
		raw.mu.Unlock()
		time.Sleep(time.Second)
		raw.mu.Lock()
		assert.NotNil(t, raw.timer)
		assert.Equal(t, time.Second, raw.flushInterval)
		assert.False(t, raw.closed)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("DefaultWriteCloserOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		raw.mu.Lock()
		assert.NotNil(t, raw.ctx)
		assert.NotNil(t, raw.cancel)
		assert.NotNil(t, raw.catcher)
		assert.Equal(t, "ID", raw.opts.Id)
		assert.Equal(t, "Test", raw.opts.MetricType)
		assert.Equal(t, DataFormatFTDC, raw.opts.Format)
		assert.Equal(t, s, raw.client)
		assert.NotNil(t, raw.buffer)
		assert.Equal(t, defaultMaxBufferSize, raw.maxBufferSize)
		assert.NotEqual(t, time.Time{}, raw.lastFlush)
		raw.mu.Unlock()
		time.Sleep(time.Second)
		raw.mu.Lock()
		assert.NotNil(t, raw.timer)
		assert.Equal(t, defaultFlushInterval, raw.flushInterval)
		assert.False(t, raw.closed)
		raw.mu.Unlock()
		assert.NoError(t, w.Close())
	})
	t.Run("NoTimedFlush", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		raw, ok := w.(*systemMetricsWriteCloser)
		require.True(t, ok)

		assert.NotNil(t, raw.ctx)
		assert.NotNil(t, raw.cancel)
		assert.NotNil(t, raw.catcher)
		assert.Equal(t, "ID", raw.opts.Id)
		assert.Equal(t, "Test", raw.opts.MetricType)
		assert.Equal(t, DataFormatFTDC, raw.opts.Format)
		assert.Equal(t, s, raw.client)
		assert.NotNil(t, raw.buffer)
		assert.Equal(t, defaultMaxBufferSize, raw.maxBufferSize)
		assert.Equal(t, time.Time{}, raw.lastFlush)
		time.Sleep(time.Second)
		assert.Nil(t, raw.timer)
		assert.Equal(t, time.Duration(0), raw.flushInterval)
		assert.False(t, raw.closed)
		assert.NoError(t, w.Close())
	})
	t.Run("InvalidWriteCloserOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			MaxBufferSize: -1,
		})
		assert.Error(t, err)
		assert.Nil(t, w)

		w, err = s.NewSystemMetricsWriteCloser(ctx, dataOpts, WriteCloserOptions{
			FlushInterval: -1,
		})
		assert.Error(t, err)
		assert.Nil(t, w)
	})
	t.Run("InvalidDataOpts", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		w, err := s.NewSystemMetricsWriteCloser(ctx, MetricDataOptions{
			Id:         "",
			MetricType: "Test",
			Format:     DataFormatFTDC,
		}, WriteCloserOptions{})
		assert.Error(t, err)
		assert.Nil(t, w)

		w, err = s.NewSystemMetricsWriteCloser(ctx, MetricDataOptions{
			Id:         "ID",
			MetricType: "",
			Format:     DataFormatFTDC,
		}, WriteCloserOptions{})
		assert.Error(t, err)
		assert.Nil(t, w)

		w, err = s.NewSystemMetricsWriteCloser(ctx, MetricDataOptions{
			Id:         "ID",
			MetricType: "Test",
			Format:     7,
		}, WriteCloserOptions{})
		assert.Error(t, err)
		assert.Nil(t, w)
	})
}

func TestCloseSystemMetrics(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		require.NoError(t, s.CloseSystemMetrics(ctx, "ID", true))
		assert.Equal(t, &internal.SystemMetricsSeriesEnd{
			Id:      "ID",
			Success: true,
		}, mc.close)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := &SystemMetricsClient{client: mc}

		assert.Error(t, s.CloseSystemMetrics(ctx, "", true))
		assert.Nil(t, mc.GetData())
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{closeErr: true}
		s := &SystemMetricsClient{client: mc}

		assert.Error(t, s.CloseSystemMetrics(ctx, "ID", true))
		assert.Nil(t, mc.GetData())
	})
}
