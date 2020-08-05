package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/timber"
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
		Task:                     s.task,
		Interval:                 time.Minute,
		Collectors:               collectors,
		Conn:                     s.conn,
		BufferTimedFlushInterval: time.Minute,
		NoBufferTimedFlush:       true,
		MaxBufferSize:            1e7,
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

// TestNewSystemMetricsCollectorWithDialOpts tests that newSystemMetricsCollector
// properly sets all values and sets up a connection to the server when given
// options to dial into Cedar.
func (s *SystemMetricsSuite) TestNewSystemMetricsCollectorWithDialOpts() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		name: "first",
	}, &mockMetricCollector{
		name: "second",
	}}

	var comm *client.Mock
	comm = client.NewMock("url")
	_, err := comm.GetBuildloggerInfo(ctx)
	s.Require().NoError(err)

	dialOpts := timber.DialCedarOptions{
		BaseAddress: "localhost",
		RPCPort:     "3000",
	}
	dialServer, err := testutil.NewMockMetricsServerWithAddress(ctx, dialOpts)
	s.Require().NoError(err)

	c, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		Task:       s.task,
		Collectors: collectors,
		DialOpts:   &dialOpts,
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
	s.Require().NotNil(c.client)
	s.Require().NoError(c.client.CloseSystemMetrics(ctx, "ID", true))
	s.Assert().NotNil(dialServer.Close)
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
		Task:       nil,
		Interval:   time.Minute,
		Collectors: collectors,
		Conn:       s.conn,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		Task:       s.task,
		Interval:   -1 * time.Minute,
		Collectors: collectors,
		Conn:       s.conn,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		Task:       s.task,
		Interval:   time.Minute,
		Collectors: []metricCollector{},
		Conn:       s.conn,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		Task:       s.task,
		Interval:   time.Minute,
		Collectors: collectors,
		Conn:       nil,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		Task:                     s.task,
		Interval:                 time.Minute,
		Collectors:               collectors,
		Conn:                     s.conn,
		BufferTimedFlushInterval: -1 * time.Minute,
	})
	s.Assert().Error(err)
	s.Assert().Nil(c)

	c, err = newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		Task:          s.task,
		Interval:      time.Minute,
		Collectors:    collectors,
		Conn:          s.conn,
		MaxBufferSize: -1,
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
		Task:               s.task,
		Interval:           time.Second,
		Collectors:         collectors,
		Conn:               s.conn,
		MaxBufferSize:      1,
		NoBufferTimedFlush: true,
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
}

// TestSystemMetricsCollectorStream tests that an individual streaming process
// properly is able to send data to the server, and properly handles any errors.
func (s *SystemMetricsSuite) TestSystemMetricsCollectorStream() {
	// check data is sending

	// check only a single process shuts down on an error
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
		Task:       s.task,
		Interval:   time.Second,
		Collectors: collectors,
		Conn:       s.conn,
	})
	s.Require().NoError(err)
	s.Require().NotNil(c)
	s.Assert().Nil(s.server.Create)
	s.Require().NoError(c.Start(ctx))

	s.Require().NoError(c.Close())
	s.server.Mu.Lock()
	s.Assert().NotNil(s.server.Close)
	s.server.Mu.Unlock()
}
