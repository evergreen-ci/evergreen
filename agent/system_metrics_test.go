package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/timber/systemmetrics"
	timberutil "github.com/evergreen-ci/timber/testutil"
	"github.com/mongodb/ftdc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
)

type mockMetricCollector struct {
	collectErr    bool
	collectorName string
	count         int
}

func (m *mockMetricCollector) name() string {
	return m.collectorName
}

func (m *mockMetricCollector) format() systemmetrics.DataFormat {
	return systemmetrics.DataFormatText
}

func (m *mockMetricCollector) collect(context.Context) ([]byte, error) {
	if m.collectErr {
		return nil, errors.New("Error collecting metrics")
	} else {
		m.count += 1
		return []byte(fmt.Sprintf("%s-%d", m.name(), m.count)), nil
	}
}

type SystemMetricsSuite struct {
	suite.Suite
	task   *task.Task
	conn   *grpc.ClientConn
	server *timberutil.MockMetricsServer
	cancel context.CancelFunc
}

func TestSystemMetricsSuite(t *testing.T) {
	suite.Run(t, new(SystemMetricsSuite))
}

func (s *SystemMetricsSuite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	var err error
	s.server, err = timberutil.NewMockMetricsServer(ctx, testutil.NextPort())
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
		collectorName: "first",
	}, &mockMetricCollector{
		collectorName: "second",
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
	s.Assert().Equal(systemmetrics.SystemMetricsOptions{
		Project:     "Project",
		Version:     "Version",
		Variant:     "Variant",
		TaskName:    "TaskName",
		TaskId:      "Id",
		Execution:   0,
		Mainline:    !s.task.IsPatchRequest(),
		Compression: systemmetrics.CompressionTypeNone,
		Schema:      systemmetrics.SchemaTypeRawEvents,
	}, c.taskOpts)
	s.Assert().Equal(time.Minute, c.interval)
	s.Assert().Equal(systemmetrics.WriteCloserOptions{
		FlushInterval: time.Minute,
		NoTimedFlush:  true,
		MaxBufferSize: 1e7,
	}, c.writeOpts)
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
		collectorName: "first",
	}, &mockMetricCollector{
		collectorName: "second",
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
		collectorName: "first",
	}, &mockMetricCollector{
		collectorName: "second",
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
	s.Assert().Len(s.server.Data["first"], 2)
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
		collectorName: "first",
	}, &mockMetricCollector{
		collectorName: "second",
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
	s.server.AddErr = true
	s.Require().NoError(c.Start(ctx))
	time.Sleep(2 * c.interval)

	s.Require().Error(c.Close())
}

// TestCloseSystemMetricsCollector tests that Close properly shuts
// down all connections and processes, and handles errors that occur
// in the process.
func (s *SystemMetricsSuite) TestCloseSystemMetricsCollector() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectors := []metricCollector{&mockMetricCollector{
		collectorName: "first",
	}, &mockMetricCollector{
		collectorName: "second",
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

func TestSystemMetricsCollectors(t *testing.T) {
	for testName, testCase := range map[string]struct {
		makeCollector func(t *testing.T) metricCollector
		expectedKeys  []string
	}{
		"DiskUsage": {
			makeCollector: func(t *testing.T) metricCollector {
				dir, err := os.Getwd()
				require.NoError(t, err)
				return newDiskUsageCollector(dir)
			},
			expectedKeys: []string{
				"total",
				"free",
				"used",
			},
		},
		"Uptime": {
			makeCollector: func(t *testing.T) metricCollector {
				return newUptimeCollector()
			},
			expectedKeys: []string{"uptime"},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			coll := testCase.makeCollector(t)

			output, err := coll.collect(ctx)
			require.NoError(t, err)

			iter := ftdc.ReadMetrics(ctx, bytes.NewReader(output))
			require.True(t, iter.Next())

			doc := iter.Document()
			for _, key := range testCase.expectedKeys {
				val := doc.Lookup(key)
				assert.NotNil(t, val, "key '%s' missing", key)
			}
		})
	}
}

func TestCollectProcesses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coll := &processCollector{}
	output, err := coll.collect(ctx)

	assert.NoError(t, err)
	assert.NotEmpty(t, output)

	var procs processesWrapper
	require.NoError(t, json.Unmarshal(output, &procs))
	assert.NotEmpty(t, procs)

	for _, proc := range procs.Processes {
		// Windows sometimes doesn't have PIDs for its processes.
		if runtime.GOOS == "windows" {
			assert.NotZero(t, proc.Command)
		} else {
			assert.NotZero(t, proc.PID)
		}
	}
}

func TestSystemMetricsCollectorWithMetricCollectorImplementation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := timberutil.NewMockMetricsServer(ctx, testutil.NextPort())
	require.NoError(t, err)

	conn, err := grpc.DialContext(ctx, srv.Address(), grpc.WithInsecure())
	require.NoError(t, err)

	tsk := &task.Task{
		Id:           "id",
		Project:      "project",
		Version:      "version",
		BuildVariant: "buildvariant",
		DisplayName:  "display_name",
		Execution:    0,
	}

	mc := newUptimeCollector()
	coll, err := newSystemMetricsCollector(ctx, &systemMetricsCollectorOptions{
		task:                     tsk,
		interval:                 time.Second,
		collectors:               []metricCollector{mc},
		bufferTimedFlushInterval: time.Second,
		conn:                     conn,
	})
	require.NoError(t, err)

	require.NoError(t, coll.Start(ctx))

	timer := time.NewTimer(3 * time.Second)
	select {
	case <-timer.C:
		require.NoError(t, coll.Close())
	}

	assert.Equal(t, tsk.Id, srv.Create.Info.TaskId)
	assert.Equal(t, tsk.Project, srv.Create.Info.Project)
	assert.Equal(t, tsk.Version, srv.Create.Info.Version)
	assert.Equal(t, tsk.BuildVariant, srv.Create.Info.Variant)
	assert.NotEmpty(t, srv.Data)
	sentData := srv.Data[mc.name()]
	assert.NotZero(t, len(sentData))
	for _, data := range sentData {
		assert.Equal(t, mc.name(), data.Type)
		assert.Equal(t, mc.format(), systemmetrics.DataFormat(data.Format))
		assert.NotEmpty(t, data.Data)
	}
}
