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
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// MockMetricsServer sets up a mock cedar server for testing sending system
// metrics data to cedar using gRPC.
type MockMetricsServer struct {
	Mu         sync.Mutex
	CreateErr  bool
	AddErr     bool
	StreamErr  bool
	CloseErr   bool
	Create     *internal.SystemMetrics
	Data       map[string][]*internal.SystemMetricsData
	StreamData map[string][]*internal.SystemMetricsData
	Close      *internal.SystemMetricsSeriesEnd
	DialOpts   timber.DialCedarOptions
}

// NewMockMetricsServer will return a new MockMetricsServer listening on a port
// near the provided port.
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

// NewMockMetricsServerWithDialOpts will return a new MockMetricsServer
// listening on the port and url from the specified dial options.
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

// Address returns the address the server is listening on.
func (ms *MockMetricsServer) Address() string {
	return fmt.Sprintf("%s:%s", ms.DialOpts.BaseAddress, ms.DialOpts.RPCPort)
}

// CreateSystemMetricsRecord returns an error if CreateErr is true, otherwise
// it sets Create to the input.
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

// AddSystemMetrics returns an error if AddErr is true, otherwise it adds the
// input to Data.
func (ms *MockMetricsServer) AddSystemMetrics(_ context.Context, in *internal.SystemMetricsData) (*internal.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.AddErr {
		return nil, errors.New("add error")
	}
	if ms.Data == nil {
		ms.Data = make(map[string][]*internal.SystemMetricsData)
	}
	ms.Data[in.Type] = append(ms.Data[in.Type], in)
	return &internal.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

// StreamSystemMetrics, when receiving from the stream, returns an error if
// if StreamErr is true, otherwise it adds the stream input to StreamData.
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

// CloseMetrics returns an error if CloseErr is true, otherwise sets Close to
// the input.
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

// MockTestResultsServer sets up a mock Cedar server for sending test results
// using gRPC.
type MockTestResultsServer struct {
	CreateErr     bool
	AddErr        bool
	StreamErr     bool
	CloseErr      bool
	Create        *internal.TestResultsInfo
	Results       map[string][]*internal.TestResults
	StreamResults map[string][]*internal.TestResults
	Close         *internal.TestResultsEndInfo
	DialOpts      timber.DialCedarOptions
}

// Address returns the address the server is listening on.
func (ms *MockTestResultsServer) Address() string {
	return fmt.Sprintf("%s:%s", ms.DialOpts.BaseAddress, ms.DialOpts.RPCPort)
}

// NewMockTestResultsServer returns a new MockTestResultsServer listening on a
// port near the provided port.
func NewMockTestResultsServer(ctx context.Context, basePort int) (*MockTestResultsServer, error) {
	srv := &MockTestResultsServer{}
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
	internal.RegisterCedarTestResultsServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return srv, nil
}

// NewMockTestResultsServerWithDialOpts returns a new MockTestResultsServer
// listening on the port and url from the specified dial options.
func NewMockTestResultsServerWithDialOpts(ctx context.Context, opts timber.DialCedarOptions) (*MockTestResultsServer, error) {
	srv := &MockTestResultsServer{}
	srv.DialOpts = opts
	lis, err := net.Listen("tcp", srv.Address())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	s := grpc.NewServer()
	internal.RegisterCedarTestResultsServer(s, srv)

	go func() {
		grip.Error(errors.Wrap(s.Serve(lis), "running server"))
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return srv, nil
}

// CreateTestResultsRecord returns an error if CreateErr is true, otherwise it
// sets Create to the input.
func (m *MockTestResultsServer) CreateTestResultsRecord(_ context.Context, in *internal.TestResultsInfo) (*internal.TestResultsResponse, error) {
	if m.CreateErr {
		return nil, errors.New("create error")
	}
	m.Create = in
	return &internal.TestResultsResponse{TestResultsRecordId: utility.RandomString()}, nil
}

// AddTestResults returns an error if AddErr is true, otherwise it adds the
// input to Results.
func (m *MockTestResultsServer) AddTestResults(_ context.Context, in *internal.TestResults) (*internal.TestResultsResponse, error) {
	if m.AddErr {
		return nil, errors.New("add error")
	}
	if m.Results == nil {
		m.Results = make(map[string][]*internal.TestResults)
	}
	m.Results[in.TestResultsRecordId] = append(m.Results[in.TestResultsRecordId], in)
	return &internal.TestResultsResponse{TestResultsRecordId: in.TestResultsRecordId}, nil
}

// StreamTestResults returns a not implemented error.
func (ms *MockTestResultsServer) StreamTestResults(internal.CedarTestResults_StreamTestResultsServer) error {
	return errors.New("not implemented")
}

// CloseTestResults returns an error if CloseErr is true, otherwise it sets
// Close to the input.
func (m *MockTestResultsServer) CloseTestResultsRecord(_ context.Context, in *internal.TestResultsEndInfo) (*internal.TestResultsResponse, error) {
	if m.CloseErr {
		return nil, errors.New("close error")
	}
	m.Close = in
	return &internal.TestResultsResponse{TestResultsRecordId: in.TestResultsRecordId}, nil
}

// MockBuildloggerServer sets up a mock cedar server for testing buildlogger
// logs to cedar using gRPC.
type MockBuildloggerServer struct {
	Mu        sync.Mutex
	CreateErr bool
	AppendErr bool
	CloseErr  bool
	Create    *internal.LogData
	Data      map[string][]*internal.LogLines
	Close     *internal.LogEndInfo
	DialOpts  timber.DialCedarOptions
}

// NewMockBuildloggerServer returns a new MockBuildloggerServer listening on a
// port near the provided port.
func NewMockBuildloggerServer(ctx context.Context, basePort int) (*MockBuildloggerServer, error) {
	srv := &MockBuildloggerServer{
		Data: make(map[string][]*internal.LogLines),
	}
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
	internal.RegisterBuildloggerServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return srv, nil
}

// NewMockBuildloggerServerWithDialOpts returns a new MockBuildloggerServer
// listening on the port and url from the specified dial options.
func NewMockBuildloggerServerWithDialOpts(ctx context.Context, opts timber.DialCedarOptions) (*MockBuildloggerServer, error) {
	srv := &MockBuildloggerServer{}
	srv.DialOpts = opts
	lis, err := net.Listen("tcp", srv.Address())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	s := grpc.NewServer()
	internal.RegisterBuildloggerServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	go func() {
		<-ctx.Done()
		s.Stop()
	}()
	return srv, nil
}

// Address returns the address the server is listening on.
func (ms *MockBuildloggerServer) Address() string {
	return fmt.Sprintf("%s:%s", ms.DialOpts.BaseAddress, ms.DialOpts.RPCPort)
}

// CreateLog returns an error if CreateErr is true, otherwise it sets Create to
// the input.
func (ms *MockBuildloggerServer) CreateLog(_ context.Context, in *internal.LogData) (*internal.BuildloggerResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CreateErr {
		return nil, errors.New("create error")
	}

	ms.Create = in
	return &internal.BuildloggerResponse{}, nil
}

// AppendLogLines returns an error if AppendErr is true, otherwise it adds the
// input to Data.
func (ms *MockBuildloggerServer) AppendLogLines(_ context.Context, in *internal.LogLines) (*internal.BuildloggerResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.AppendErr {
		return nil, errors.New("append error")
	}

	if ms.Data == nil {
		ms.Data = make(map[string][]*internal.LogLines)
	}
	ms.Data[in.LogId] = append(ms.Data[in.LogId], in)

	return &internal.BuildloggerResponse{LogId: in.LogId}, nil
}

// StreamLogLines returns a not implemented error.
func (ms *MockBuildloggerServer) StreamLogLines(in internal.Buildlogger_StreamLogLinesServer) error {
	return errors.New("not implemented")
}

// CloseLog returns an error if CloseErr is true, otherwise it sets Close to
// the input.
func (ms *MockBuildloggerServer) CloseLog(_ context.Context, in *internal.LogEndInfo) (*internal.BuildloggerResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CloseErr {
		return nil, errors.New("close error")
	}

	ms.Close = in
	return &internal.BuildloggerResponse{LogId: in.LogId}, nil
}
