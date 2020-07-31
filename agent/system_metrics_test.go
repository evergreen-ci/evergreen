package agent

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
)

type MockMetricCollector struct {
	collectErr bool
	prefix     string
	count      int
}

func (m *MockMetricCollector) Name() string {
	return "MockMetrics"
}

func (m *MockMetricCollector) Format() DataFormat {
	return DataFormatText
}

func (m *MockMetricCollector) Collect() ([]byte, error) {
	if m.collectErr {
		return nil, errors.New("Error collecting metrics")
	} else {
		m.count += 1
		return []byte(fmt.Sprintf("%s-%d", m.prefix, m.count)), nil
	}
}

type mockServer struct {
	createErr bool
	addErr    bool
	closeErr  bool
	info      bool
	data      bool
	close     bool
}

func (mc *mockServer) CreateSystemMetricsRecord(_ context.Context, in *internal.SystemMetrics) (*internal.SystemMetricsResponse, error) {
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

// Mock server

// Test created with correct options

// Test created with correct options and existing collection

// Test created with incorrect options

// Test no processes are running if startup fails

// Test processes are running if startup succeeds

// Test process is collecting metrics

// Test process error logs error and only shuts down self

// Test global context cancel shuts down all processes and connections

// Test global context cancel logs errors to global

// Test Close shuts down all processes and connections

// Test Close returns collected errors
