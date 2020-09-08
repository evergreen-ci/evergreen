package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/internal"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// MockMetricsServer sets up a mock cedar server for testing sending system
// metrics data to cedar using grpc.
type MockMetricsServer struct {
	Mu         sync.Mutex
	CreateErr  bool
	AddErr     bool
	StreamErr  bool
	CloseErr   bool
	Create     *internal.SystemMetrics
	Data       *internal.SystemMetricsData
	StreamData map[string][]*internal.SystemMetricsData
	Close      *internal.SystemMetricsSeriesEnd
	DialOpts   timber.DialCedarOptions
}

// CreateSystemMetricsRecord will return an error if CreateErr is true, and
// otherwise set Create to true.
func (ms *MockMetricsServer) CreateSystemMetricsRecord(_ context.Context, in *internal.SystemMetrics) (*internal.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CreateErr {
		return nil, errors.New("create error")
	}
	ms.Create = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

// AddSystemMetrics will return an error if AddErr is true, and
// otherwise set Data to true.
func (ms *MockMetricsServer) AddSystemMetrics(_ context.Context, in *internal.SystemMetricsData) (*internal.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.AddErr {
		return nil, errors.New("add error")
	}
	ms.Data = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

// StreamSystemMetrics will, when receiving from the stream, return an error
// if StreamErr is true, and otherwise set Stream to true.
func (ms *MockMetricsServer) StreamSystemMetrics(stream internal.CedarSystemMetrics_StreamSystemMetricsServer) error {
	ctx := stream.Context()
	id := ""

	ms.Mu.Lock()
	if ms.StreamData == nil {
		ms.StreamData = map[string][]*internal.SystemMetricsData{}
	}
	ms.Mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			return errors.New("error from context in mock stream")
		}

		chunk, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&internal.SystemMetricsResponse{Id: id})
		}
		if err != nil {
			return fmt.Errorf("error in stream for id %s", id)
		}

		if id == "" {
			id = chunk.Id
		} else if chunk.Id != id {
			return fmt.Errorf("chunk id %s doesn't match id %s", chunk.Id, id)
		}

		ms.Mu.Lock()
		if ms.StreamErr {
			ms.Mu.Unlock()
			return errors.New("error in stream")
		}
		ms.StreamData[chunk.Type] = append(ms.StreamData[chunk.Type], chunk)
		ms.Mu.Unlock()
	}
}

// CloseMetrics will return an error if CloseErr is true, and otherwise set
// Close to true.
func (ms *MockMetricsServer) CloseMetrics(_ context.Context, in *internal.SystemMetricsSeriesEnd) (*internal.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CloseErr {
		return nil, errors.New("close error")
	}
	ms.Close = in
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

// Address returns the address the server is listening on.
func (ms *MockMetricsServer) Address() string {
	return fmt.Sprintf("%s:%s", ms.DialOpts.BaseAddress, ms.DialOpts.RPCPort)
}

// NewMockMetricsServer will return a new MockMetricsServer listening on
// a port near the provided port.
func NewMockMetricsServer(ctx context.Context, basePort int) (*MockMetricsServer, error) {
	srv := &MockMetricsServer{}
	port := GetPortNumber(basePort)

	srv.DialOpts = timber.DialCedarOptions{
		BaseAddress: "localhost",
		RPCPort:     strconv.Itoa(port),
	}

	lis, err := net.Listen("tcp", srv.Address())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	s := grpc.NewServer()
	internal.RegisterCedarSystemMetricsServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return srv, nil
}

// NewMockMetricsServerWithDialOpts will return a new MockMetricsServer listening
// on the port and url from the specified dial options
func NewMockMetricsServerWithDialOpts(ctx context.Context, opts timber.DialCedarOptions) (*MockMetricsServer, error) {
	srv := &MockMetricsServer{}
	srv.DialOpts = opts
	lis, err := net.Listen("tcp", srv.Address())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	s := grpc.NewServer()
	internal.RegisterCedarSystemMetricsServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return srv, nil
}
