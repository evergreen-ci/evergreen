package systemmetrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/internal"
	"github.com/evergreen-ci/timber/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type mockClient struct {
	createErr       bool
	addErr          bool
	closeErr        bool
	streamCreateErr bool
	streamSendErr   bool
	streamCloseErr  bool
	info            *internal.SystemMetrics
	data            *internal.SystemMetricsData
	stream          *mockStreamClient
	close           *internal.SystemMetricsSeriesEnd
}

func (mc *mockClient) CreateSystemMetricRecord(_ context.Context, in *internal.SystemMetrics, opts ...grpc.CallOption) (*internal.SystemMetricsResponse, error) {
	if mc.createErr {
		return nil, errors.New("create error")
	}
	mc.info = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (mc *mockClient) AddSystemMetrics(_ context.Context, in *internal.SystemMetricsData, opts ...grpc.CallOption) (*internal.SystemMetricsResponse, error) {
	if mc.addErr {
		return nil, errors.New("add error")
	}
	mc.data = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

type mockStreamClient struct {
	mu       sync.Mutex
	sendErr  bool
	closeErr bool
	data     []*internal.SystemMetricsData
	close    bool
}

func (m *mockStreamClient) Send(data *internal.SystemMetricsData) error {
	if m.sendErr {
		return errors.New("problem sending data")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = append(m.data, data)
	return nil
}

func (m *mockStreamClient) CloseAndRecv() (*internal.SystemMetricsResponse, error) {
	if m.closeErr {
		return nil, errors.New("problem closing data")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.close = true
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (m *mockStreamClient) Header() (metadata.MD, error) {
	return map[string][]string{}, nil
}
func (m *mockStreamClient) Trailer() metadata.MD {
	return map[string][]string{}
}
func (m *mockStreamClient) CloseSend() error {
	return nil
}
func (m *mockStreamClient) Context() context.Context {
	return nil
}
func (m *mockStreamClient) SendMsg(i interface{}) error {
	return nil
}
func (m *mockStreamClient) RecvMsg(i interface{}) error {
	return nil
}

func (mc *mockClient) StreamSystemMetrics(_ context.Context, opts ...grpc.CallOption) (internal.CedarSystemMetrics_StreamSystemMetricsClient, error) {
	if mc.streamCreateErr {
		return nil, errors.New("problem creating stream")
	}
	stream := &mockStreamClient{
		closeErr: mc.streamCloseErr,
		sendErr:  mc.streamSendErr,
	}
	mc.stream = stream
	return stream, nil
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

type mockServer struct {
	createErr bool
	addErr    bool
	closeErr  bool
	info      bool
	data      bool
	close     bool
}

func (mc *mockServer) CreateSystemMetricRecord(_ context.Context, in *internal.SystemMetrics) (*internal.SystemMetricsResponse, error) {
	if mc.createErr {
		return nil, errors.New("create error")
	}
	mc.info = true
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (mc *mockServer) AddSystemMetrics(_ context.Context, in *internal.SystemMetricsData) (*internal.SystemMetricsResponse, error) {
	if mc.addErr {
		return nil, errors.New("add error")
	}
	mc.data = true
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func (mc *mockServer) StreamSystemMetrics(internal.CedarSystemMetrics_StreamSystemMetricsServer) error {
	return nil
}

func (mc *mockServer) CloseMetrics(_ context.Context, in *internal.SystemMetricsSeriesEnd) (*internal.SystemMetricsResponse, error) {
	if mc.closeErr {
		return nil, errors.New("close error")
	}
	mc.close = true
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

func TestNewSystemMetricsClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &mockServer{}
	port := testutil.GetPortNumber(3000)
	require.NoError(t, startRPCService(ctx, srv, port))
	t.Run("ValidOptions", func(t *testing.T) {
		connOpts := ConnectionOptions{
			Client: http.Client{},
			DialOpts: timber.DialCedarOptions{
				BaseAddress: "localhost",
				RPCPort:     strconv.Itoa(port),
			},
		}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID"))
		assert.True(t, srv.close)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		connOpts := ConnectionOptions{}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.Error(t, err)
		require.Nil(t, client)
	})
}

func TestNewSystemMetricsClientWithExistingClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &mockServer{}
	port := testutil.GetPortNumber(3000)
	require.NoError(t, startRPCService(ctx, srv, port))
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
	require.NoError(t, err)

	t.Run("ValidOptions", func(t *testing.T) {
		client, err := NewSystemMetricsClientWithExistingConnection(ctx, conn)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID"))
		assert.True(t, srv.close)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		client, err := NewSystemMetricsClientWithExistingConnection(ctx, nil)
		require.Error(t, err)
		require.Nil(t, client)
	})
}

func TestCloseClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &mockServer{}
	port := testutil.GetPortNumber(3000)
	require.NoError(t, startRPCService(ctx, srv, port))
	t.Run("WithoutExistingConnection", func(t *testing.T) {
		connOpts := ConnectionOptions{
			Client: http.Client{},
			DialOpts: timber.DialCedarOptions{
				BaseAddress: "localhost",
				RPCPort:     strconv.Itoa(port),
			},
		}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID"))
		assert.True(t, srv.close)

		require.NoError(t, client.CloseClient())
		require.Error(t, client.CloseSystemMetrics(ctx, "ID"))
	})
	t.Run("WithExistingConnection", func(t *testing.T) {
		addr := fmt.Sprintf("localhost:%d", port)
		conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure())
		require.NoError(t, err)
		client, err := NewSystemMetricsClientWithExistingConnection(ctx, conn)
		require.NoError(t, err)
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID"))
		assert.True(t, srv.close)

		require.NoError(t, client.CloseClient())
		require.NoError(t, client.CloseSystemMetrics(ctx, "ID"))
		require.NoError(t, conn.Close())
	})
	t.Run("AlreadyClosed", func(t *testing.T) {
		connOpts := ConnectionOptions{
			Client: http.Client{},
			DialOpts: timber.DialCedarOptions{
				BaseAddress: "localhost",
				RPCPort:     strconv.Itoa(port),
			},
		}
		client, err := NewSystemMetricsClient(ctx, connOpts)
		require.NoError(t, err)
		require.NoError(t, client.CloseClient())

		require.Error(t, client.CloseClient())
	})
}

func TestCreateSystemMetricsRecord(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		id, err := s.CreateSystemMetricRecord(ctx, SystemMetricsOptions{
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
		s := SystemMetricsClient{
			client: mc,
		}
		id, err := s.CreateSystemMetricRecord(ctx, SystemMetricsOptions{
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
		require.Error(t, err)
		assert.Equal(t, id, "")
		assert.Nil(t, mc.data)
		id, err = s.CreateSystemMetricRecord(ctx, SystemMetricsOptions{
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
		require.Error(t, err)
		assert.Equal(t, id, "")
		assert.Nil(t, mc.data)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			createErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		id, err := s.CreateSystemMetricRecord(ctx, SystemMetricsOptions{
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
		require.Error(t, err)
		assert.Equal(t, id, "")
		assert.Nil(t, mc.data)
	})
}

func TestAddSystemMetrics(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		require.NoError(t, s.AddSystemMetrics(ctx, "ID", "Test", []byte("Test byte string")))
		assert.Equal(t, &internal.SystemMetricsData{
			Id:   "ID",
			Type: "Test",
			Data: []byte("Test byte string"),
		}, mc.data)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		require.Error(t, s.AddSystemMetrics(ctx, "", "Test", []byte("Test byte string")))
		assert.Nil(t, mc.data)
		require.Error(t, s.AddSystemMetrics(ctx, "ID", "", []byte("Test byte string")))
		assert.Nil(t, mc.data)
		require.Error(t, s.AddSystemMetrics(ctx, "ID", "Test", []byte{}))
		assert.Nil(t, mc.data)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			addErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		require.Error(t, s.AddSystemMetrics(ctx, "ID", "Test", []byte("Test byte string")))
		assert.Nil(t, mc.data)
	})
}

func TestStreamSystemMetrics(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidStreamOpts", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			FlushInterval: time.Second,
			MaxBufferSize: 1e5,
		})
		require.NoError(t, err)
		stream.mu.Lock()
		assert.NotNil(t, stream.ctx)
		assert.NotNil(t, stream.cancel)
		assert.NotNil(t, stream.catcher)
		assert.Equal(t, "ID", stream.id)
		assert.Equal(t, "Test", stream.metricType)
		assert.Equal(t, mc.stream, stream.stream)
		assert.NotNil(t, stream.buffer)
		assert.Equal(t, int(1e5), stream.maxBufferSize)
		assert.NotEqual(t, time.Time{}, stream.lastFlush)
		stream.mu.Unlock()
		time.Sleep(time.Second)
		stream.mu.Lock()
		assert.NotNil(t, stream.timer)
		assert.Equal(t, time.Second, stream.flushInterval)
		assert.False(t, stream.closed)
		stream.mu.Unlock()
		require.NoError(t, stream.Close())
	})
	t.Run("DefaultStreamOpts", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{})
		require.NoError(t, err)
		stream.mu.Lock()
		assert.NotNil(t, stream.ctx)
		assert.NotNil(t, stream.cancel)
		assert.NotNil(t, stream.catcher)
		assert.Equal(t, "ID", stream.id)
		assert.Equal(t, "Test", stream.metricType)
		assert.Equal(t, mc.stream, stream.stream)
		assert.NotNil(t, stream.buffer)
		assert.Equal(t, defaultMaxBufferSize, stream.maxBufferSize)
		assert.NotEqual(t, time.Time{}, stream.lastFlush)
		stream.mu.Unlock()
		time.Sleep(time.Second)
		stream.mu.Lock()
		assert.NotNil(t, stream.timer)
		assert.Equal(t, defaultFlushInterval, stream.flushInterval)
		assert.False(t, stream.closed)
		stream.mu.Unlock()
		require.NoError(t, stream.Close())
	})
	t.Run("NoTimedFlush", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		assert.NotNil(t, stream.ctx)
		assert.NotNil(t, stream.cancel)
		assert.NotNil(t, stream.catcher)
		assert.Equal(t, "ID", stream.id)
		assert.Equal(t, "Test", stream.metricType)
		assert.Equal(t, mc.stream, stream.stream)
		assert.NotNil(t, stream.buffer)
		assert.Equal(t, defaultMaxBufferSize, stream.maxBufferSize)
		assert.Equal(t, time.Time{}, stream.lastFlush)
		time.Sleep(time.Second)
		assert.Nil(t, stream.timer)
		assert.Equal(t, time.Duration(0), stream.flushInterval)
		assert.False(t, stream.closed)
		require.NoError(t, stream.Close())
	})
	t.Run("InvalidStreamOpts", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "", "Test", StreamOpts{})
		require.Error(t, err)
		assert.Nil(t, stream)
		stream, err = s.StreamSystemMetrics(ctx, "ID", "", StreamOpts{})
		require.Error(t, err)
		assert.Nil(t, stream)
		stream, err = s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			MaxBufferSize: -1,
		})
		require.Error(t, err)
		assert.Nil(t, stream)
		stream, err = s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			FlushInterval: -1,
		})
		require.Error(t, err)
		assert.Nil(t, stream)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			streamCreateErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{})
		require.Error(t, err)
		assert.Nil(t, stream)
	})
}

func TestStreamWrite(t *testing.T) {
	ctx := context.Background()
	t.Run("UnderBufferSize", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		testString := []byte("small test string")

		n, err := stream.Write(testString)
		require.NoError(t, err)
		assert.Equal(t, len(testString), n)
		assert.Equal(t, 0, len(mc.stream.data))
		assert.Equal(t, testString, stream.buffer)
	})
	t.Run("OverBufferSize", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush:  true,
			MaxBufferSize: 1,
		})
		require.NoError(t, err)

		testString := []byte("large test string")

		n, err := stream.Write(testString)
		require.NoError(t, err)
		assert.Equal(t, len(testString), n)
		mc.stream.mu.Lock()
		assert.Equal(t, 1, len(mc.stream.data))
		assert.Equal(t, testString, mc.stream.data[0].Data)
		mc.stream.mu.Unlock()
		stream.mu.Lock()
		assert.Equal(t, []byte{}, stream.buffer)
		stream.mu.Unlock()
	})
	t.Run("ExistingDataInBuffer", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		testString1 := []byte("first test string")
		testString2 := []byte("second test string")

		_, err = stream.Write(testString1)
		require.NoError(t, err)
		n, err := stream.Write(testString2)
		require.NoError(t, err)
		assert.Equal(t, len(testString2), n)
		stream.mu.Lock()
		assert.Equal(t, append(testString1, testString2...), stream.buffer)
		stream.mu.Unlock()
	})
	t.Run("AlreadyClosed", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		require.NoError(t, stream.Close())

		n, err := stream.Write([]byte("small test string"))
		require.Error(t, err)
		assert.Equal(t, 0, n)
	})
	t.Run("NoData", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		n, err := stream.Write([]byte{})
		require.Error(t, err)
		assert.Equal(t, 0, n)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			streamSendErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush:  true,
			MaxBufferSize: 1,
		})
		require.NoError(t, err)

		n, err := stream.Write([]byte("small test string"))
		require.Error(t, err)
		assert.Equal(t, 0, n)
	})
}

func TestStreamTimedFlush(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidOutput", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		testString := []byte("small test string")
		_, err = stream.Write(testString)
		require.NoError(t, err)
		mc.stream.mu.Lock()
		assert.Equal(t, 0, len(mc.stream.data))
		mc.stream.mu.Unlock()

		time.Sleep(2 * time.Second)
		mc.stream.mu.Lock()
		assert.Equal(t, 1, len(mc.stream.data))
		assert.Equal(t, testString, mc.stream.data[0].Data)
		mc.stream.mu.Unlock()
		stream.mu.Lock()
		assert.Equal(t, []byte{}, stream.buffer)
		stream.mu.Unlock()
	})
	t.Run("TimedFlushResetsTimer", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		testString := []byte("small test string")
		_, err = stream.Write(testString)
		require.NoError(t, err)
		mc.stream.mu.Lock()
		assert.Equal(t, 0, len(mc.stream.data))
		mc.stream.mu.Unlock()

		stream.mu.Lock()
		lastFlush := stream.lastFlush
		stream.mu.Unlock()

		time.Sleep(2 * time.Second)
		stream.mu.Lock()
		assert.True(t, stream.lastFlush.After(lastFlush))
		assert.NotNil(t, stream.timer)
		stream.mu.Unlock()
	})
	t.Run("WriteResetsTimer", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			MaxBufferSize: 1,
		})
		require.NoError(t, err)
		time.Sleep(time.Second)
		assert.False(t, time.Since(stream.lastFlush) < time.Second)

		_, err = stream.Write([]byte("large test string"))
		require.NoError(t, err)
		stream.mu.Lock()
		assert.True(t, time.Since(stream.lastFlush) < time.Second)
		stream.mu.Unlock()
		require.NoError(t, stream.Close())
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			streamSendErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		_, err = stream.Write([]byte("smaller than buf"))
		require.NoError(t, err)

		time.Sleep(2 * time.Second)
		_, err = stream.Write([]byte("random string"))
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "writer already closed due to error"))
		err = stream.Close()
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "writer already closed due to error"))
		stream.mu.Lock()
		assert.True(t, stream.closed)
		stream.mu.Unlock()
	})
}

func TestStreamClose(t *testing.T) {
	ctx := context.Background()
	t.Run("NoError", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		testString := []byte("small test string")
		_, err = stream.Write(testString)
		require.NoError(t, err)
		assert.Equal(t, 0, len(mc.stream.data))

		require.NoError(t, stream.Close())
		assert.Equal(t, 1, len(mc.stream.data))
		assert.Equal(t, testString, mc.stream.data[0].Data)
		assert.True(t, mc.stream.close)
	})
	t.Run("AlreadyClosed", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)
		stream.closed = true

		require.Error(t, stream.Close())
	})
	t.Run("NoTimedFlush", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			FlushInterval: time.Second,
		})
		require.NoError(t, err)
		require.NoError(t, stream.Close())
		lastFlush := stream.lastFlush

		time.Sleep(2 * time.Second)
		assert.Equal(t, lastFlush, stream.lastFlush)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			streamCloseErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		stream, err := s.StreamSystemMetrics(ctx, "ID", "Test", StreamOpts{
			NoTimedFlush: true,
		})
		require.NoError(t, err)

		require.Error(t, stream.Close())
	})
}

func TestCloseSystemMetrics(t *testing.T) {
	ctx := context.Background()
	t.Run("ValidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		require.NoError(t, s.CloseSystemMetrics(ctx, "ID"))
		assert.Equal(t, &internal.SystemMetricsSeriesEnd{
			Id: "ID",
		}, mc.close)
	})
	t.Run("InvalidOptions", func(t *testing.T) {
		mc := &mockClient{}
		s := SystemMetricsClient{
			client: mc,
		}
		require.Error(t, s.CloseSystemMetrics(ctx, ""))
		assert.Nil(t, mc.data)
	})
	t.Run("RPCError", func(t *testing.T) {
		mc := &mockClient{
			closeErr: true,
		}
		s := SystemMetricsClient{
			client: mc,
		}
		require.Error(t, s.CloseSystemMetrics(ctx, "ID"))
		assert.Nil(t, mc.data)
	})
}

func startRPCService(ctx context.Context, service internal.CedarSystemMetricsServer, port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return errors.WithStack(err)
	}

	s := grpc.NewServer()
	internal.RegisterCedarSystemMetricsServer(s, service)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	return nil
}
