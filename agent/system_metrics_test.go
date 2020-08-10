package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	metrics "github.com/evergreen-ci/timber/system_metrics"
	"github.com/evergreen-ci/timber/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
)

type mockMetricCollector struct {
	collectErr bool
	name       string
	count      int
}

func (m *mockMetricCollector) Name() string {
	return m.name
}

func (m *mockMetricCollector) Format() dataFormat {
	return dataFormatText
}

func (m *mockMetricCollector) Collect() ([]byte, error) {
	if m.collectErr {
		return nil, errors.New("Error collecting metrics")
	} else {
		m.count += 1
		return []byte(fmt.Sprintf("%s-%d", m.name, m.count)), nil
	}
}

type SystemMetricsSuite struct {
	suite.Suite
	task   *task.Task
	conn   *grpc.ClientConn
	server *testutil.MockMetricsServer
	cancel context.CancelFunc
}

func TestSystemMetricsSuite(t *testing.T) {
	suite.Run(t, new(SystemMetricsSuite))
}

func (s *SystemMetricsSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	var err error
	s.server, err = testutil.NewMockMetricsServer(ctx, 5000)
	s.Require().NoError(err)

	s.conn, err = grpc.DialContext(ctx, s.server.Address(), grpc.WithInsecure())
	s.Require().NoError(err)

	s.task = &task.Task{
		Project:      "Project",
		Version:      "Version",
		BuildVariant: "Variant",
		DisplayName:  "TaskName",
		Id:           "Id",
		Execution:    0,
	}
}

func (s *SystemMetricsSuite) TearDownTest() {
	s.cancel()
}

// TestNewSystemMetricsCollectorWithConnection tests that newSystemMetricsCollector
// properly sets all values and sets up a connection to the server when given
// a valid client connection.
func (s *SystemMetricsSuite) TestNewSystemMetricsCollectorWithConnection() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		name: "first",
	}, &mockMetricCollector{
		name: "second",
	}}

	c, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:                     s.task,
		interval:                 time.Minute,
		collectors:               collectors,
		conn:                     s.conn,
		bufferTimedFlushInterval: time.Minute,
		noBufferTimedFlush:       true,
		maxBufferSize:            1e7,
	})
	s.Require().NoError(err)
	s.Require().NotNil(c)
	s.Assert().Equal(collectors, c.collectors)
	s.Assert().Equal(metrics.SystemMetricsOptions{
		Project:     "Project",
		Version:     "Version",
		Variant:     "Variant",
		TaskName:    "TaskName",
		TaskId:      "Id",
		Execution:   0,
		Mainline:    !s.task.IsPatchRequest(),
		Compression: metrics.CompressionTypeNone,
		Schema:      metrics.SchemaTypeRawEvents,
	}, *c.taskOpts)
	s.Assert().Equal(time.Minute, c.interval)
	s.Assert().Equal(metrics.StreamOpts{
		FlushInterval: time.Minute,
		NoTimedFlush:  true,
		MaxBufferSize: 1e7,
	}, *c.streamOpts)
	s.Require().NotNil(c.client)
	s.Require().NoError(c.client.CloseSystemMetrics(ctx, "ID", true))
	s.Assert().NotNil(s.server.Close)
	s.Assert().NotNil(c.catcher)

}

// TestNewSystemMetricsCollectorWithInvalidOpts tests that newSystemMetricsCollector
// properly returns an error and a nil object when passed invalid options.
func (s *SystemMetricsSuite) TestNewSystemMetricsCollectorWithInvalidOpts() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		name: "first",
	}, &mockMetricCollector{
		name: "second",
	}}

	c, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:       nil,
		interval:   time.Minute,
		collectors: collectors,
		conn:       s.conn,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:       s.task,
		interval:   -1 * time.Minute,
		collectors: collectors,
		conn:       s.conn,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:       s.task,
		interval:   time.Minute,
		collectors: []metricCollector{},
		conn:       s.conn,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:       s.task,
		interval:   time.Minute,
		collectors: collectors,
		conn:       nil,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:                     s.task,
		interval:                 time.Minute,
		collectors:               collectors,
		conn:                     s.conn,
		bufferTimedFlushInterval: -1 * time.Minute,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:          s.task,
		interval:      time.Minute,
		collectors:    collectors,
		conn:          s.conn,
		maxBufferSize: -1,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)
}

// TestStartSystemMetricsCollector tests that Start properly begins collecting
// each metric.
func (s *SystemMetricsSuite) TestStartSystemMetricsCollector() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		name: "first",
	}, &mockMetricCollector{
		name: "second",
	}}

	c, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:               s.task,
		interval:           time.Second,
		collectors:         collectors,
		conn:               s.conn,
		maxBufferSize:      1,
		noBufferTimedFlush: true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(c)
	s.Assert().Nil(s.server.Create)

	s.Require().NoError(c.Start(ctx))
	time.Sleep(2 * time.Second)
	s.server.Mu.Lock()
	s.Assert().NotNil(s.server.Create)
	s.Assert().Len(s.server.StreamData["first"], 2)
	s.server.Mu.Unlock()
	s.Assert().NoError(c.catcher.Resolve())

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:               s.task,
		interval:           time.Second,
		collectors:         collectors,
		conn:               s.conn,
		maxBufferSize:      1,
		noBufferTimedFlush: true,
	})
	s.Require().NoError(err)
	s.server.CreateErr = true
	s.Require().Error(c.Start(ctx))
}

// TestSystemMetricsCollectorStream tests that an individual streaming process
// properly handles any errors.
func (s *SystemMetricsSuite) TestSystemMetricsCollectorStreamError() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		name: "first",
	}, &mockMetricCollector{
		name: "second",
	}}

	c, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:               s.task,
		interval:           time.Second,
		collectors:         collectors,
		conn:               s.conn,
		maxBufferSize:      1,
		noBufferTimedFlush: true,
	})
	s.Require().NoError(err)
	s.server.StreamErr = true
	s.Require().NoError(c.Start(ctx))

	s.Require().Error(c.Close())
}

// TestCloseSystemMetricsCollector tests that Close properly shuts
// down all connections and processes, and handles errors that occur
// in the process.
func (s *SystemMetricsSuite) TestCloseSystemMetricsCollector() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		name: "first",
	}, &mockMetricCollector{
		name: "second",
	}}

	c, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:       s.task,
		interval:   time.Second,
		collectors: collectors,
		conn:       s.conn,
	})
	s.Require().NoError(err)
	s.Require().NoError(c.Start(ctx))

	s.Require().NoError(c.Close())
	s.server.Mu.Lock()
	s.Assert().NotNil(s.server.Close)
	s.server.Mu.Unlock()

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:       s.task,
		interval:   time.Second,
		collectors: collectors,
		conn:       s.conn,
	})
	s.Require().NoError(err)
	s.server.CloseErr = true
	s.Require().NoError(c.Start(ctx))

	s.Require().Error(c.Close())
}
