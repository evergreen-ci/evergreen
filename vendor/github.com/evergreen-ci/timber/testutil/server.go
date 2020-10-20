package testutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/timber"
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
	Create     *gopb.SystemMetrics
	Data       map[string][]*gopb.SystemMetricsData
	StreamData map[string][]*gopb.SystemMetricsData
	Close      *gopb.SystemMetricsSeriesEnd
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
	gopb.RegisterCedarSystemMetricsServer(s, srv)

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
	gopb.RegisterCedarSystemMetricsServer(s, srv)

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
func (ms *MockMetricsServer) CreateSystemMetricsRecord(_ context.Context, in *gopb.SystemMetrics) (*gopb.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CreateErr {
		return nil, errors.New("create error")
	}
	ms.Create = in
	return &gopb.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

// AddSystemMetrics returns an error if AddErr is true, otherwise it adds the
// input to Data.
func (ms *MockMetricsServer) AddSystemMetrics(_ context.Context, in *gopb.SystemMetricsData) (*gopb.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.AddErr {
		return nil, errors.New("add error")
	}
	if ms.Data == nil {
		ms.Data = make(map[string][]*gopb.SystemMetricsData)
	}
	ms.Data[in.Type] = append(ms.Data[in.Type], in)
	return &gopb.SystemMetricsResponse{
		Id: "ID",
	}, nil
}

// StreamSystemMetrics, when receiving from the stream, returns an error if
// if StreamErr is true, otherwise it adds the stream input to StreamData.
func (ms *MockMetricsServer) StreamSystemMetrics(stream gopb.CedarSystemMetrics_StreamSystemMetricsServer) error {
	ctx := stream.Context()
	id := ""

	ms.Mu.Lock()
	if ms.StreamData == nil {
		ms.StreamData = map[string][]*gopb.SystemMetricsData{}
	}
	ms.Mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			return errors.New("error from context in mock stream")
		}

		chunk, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&gopb.SystemMetricsResponse{Id: id})
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
func (ms *MockMetricsServer) CloseMetrics(_ context.Context, in *gopb.SystemMetricsSeriesEnd) (*gopb.SystemMetricsResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CloseErr {
		return nil, errors.New("close error")
	}
	ms.Close = in
	return &gopb.SystemMetricsResponse{
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
	Create        *gopb.TestResultsInfo
	Results       map[string][]*gopb.TestResults
	StreamResults map[string][]*gopb.TestResults
	Close         *gopb.TestResultsEndInfo
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
	gopb.RegisterCedarTestResultsServer(s, srv)

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
	gopb.RegisterCedarTestResultsServer(s, srv)

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
func (m *MockTestResultsServer) CreateTestResultsRecord(_ context.Context, in *gopb.TestResultsInfo) (*gopb.TestResultsResponse, error) {
	if m.CreateErr {
		return nil, errors.New("create error")
	}
	m.Create = in
	return &gopb.TestResultsResponse{TestResultsRecordId: utility.RandomString()}, nil
}

// AddTestResults returns an error if AddErr is true, otherwise it adds the
// input to Results.
func (m *MockTestResultsServer) AddTestResults(_ context.Context, in *gopb.TestResults) (*gopb.TestResultsResponse, error) {
	if m.AddErr {
		return nil, errors.New("add error")
	}
	if m.Results == nil {
		m.Results = make(map[string][]*gopb.TestResults)
	}
	m.Results[in.TestResultsRecordId] = append(m.Results[in.TestResultsRecordId], in)
	return &gopb.TestResultsResponse{TestResultsRecordId: in.TestResultsRecordId}, nil
}

// StreamTestResults returns a not implemented error.
func (ms *MockTestResultsServer) StreamTestResults(gopb.CedarTestResults_StreamTestResultsServer) error {
	return errors.New("not implemented")
}

// CloseTestResults returns an error if CloseErr is true, otherwise it sets
// Close to the input.
func (m *MockTestResultsServer) CloseTestResultsRecord(_ context.Context, in *gopb.TestResultsEndInfo) (*gopb.TestResultsResponse, error) {
	if m.CloseErr {
		return nil, errors.New("close error")
	}
	m.Close = in
	return &gopb.TestResultsResponse{TestResultsRecordId: in.TestResultsRecordId}, nil
}

// MockBuildloggerServer sets up a mock cedar server for testing buildlogger
// logs to cedar using gRPC.
type MockBuildloggerServer struct {
	Mu        sync.Mutex
	CreateErr bool
	AppendErr bool
	CloseErr  bool
	Create    *gopb.LogData
	Data      map[string][]*gopb.LogLines
	Close     *gopb.LogEndInfo
	DialOpts  timber.DialCedarOptions
}

// NewMockBuildloggerServer returns a new MockBuildloggerServer listening on a
// port near the provided port.
func NewMockBuildloggerServer(ctx context.Context, basePort int) (*MockBuildloggerServer, error) {
	srv := &MockBuildloggerServer{
		Data: make(map[string][]*gopb.LogLines),
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
	gopb.RegisterBuildloggerServer(s, srv)

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
	gopb.RegisterBuildloggerServer(s, srv)

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
func (ms *MockBuildloggerServer) CreateLog(_ context.Context, in *gopb.LogData) (*gopb.BuildloggerResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CreateErr {
		return nil, errors.New("create error")
	}

	ms.Create = in
	return &gopb.BuildloggerResponse{}, nil
}

// AppendLogLines returns an error if AppendErr is true, otherwise it adds the
// input to Data.
func (ms *MockBuildloggerServer) AppendLogLines(_ context.Context, in *gopb.LogLines) (*gopb.BuildloggerResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.AppendErr {
		return nil, errors.New("append error")
	}

	if ms.Data == nil {
		ms.Data = make(map[string][]*gopb.LogLines)
	}
	ms.Data[in.LogId] = append(ms.Data[in.LogId], in)

	return &gopb.BuildloggerResponse{LogId: in.LogId}, nil
}

// StreamLogLines returns a not implemented error.
func (ms *MockBuildloggerServer) StreamLogLines(in gopb.Buildlogger_StreamLogLinesServer) error {
	return errors.New("not implemented")
}

// CloseLog returns an error if CloseErr is true, otherwise it sets Close to
// the input.
func (ms *MockBuildloggerServer) CloseLog(_ context.Context, in *gopb.LogEndInfo) (*gopb.BuildloggerResponse, error) {
	ms.Mu.Lock()
	defer ms.Mu.Unlock()

	if ms.CloseErr {
		return nil, errors.New("close error")
	}

	ms.Close = in
	return &gopb.BuildloggerResponse{LogId: in.LogId}, nil
}
