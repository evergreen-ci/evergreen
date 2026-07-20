package command

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type HostListSuite struct {
	cancel func()
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	ctx    context.Context

	cmd *listHosts
	suite.Suite
}

func TestHostListSuite(t *testing.T) {
	suite.Run(t, new(HostListSuite))
}

func (s *HostListSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var err error

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{Expansions: util.Expansions{"foo": "3"}, Task: task.Task{}, Project: model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, &s.conf.Task, nil)
	s.Require().NoError(err)
	s.cmd = listHostFactory().(*listHosts)
}

func (s *HostListSuite) TearDownTest() {
	s.cancel()
}

func (s *HostListSuite) TestCommandIsProperlyConstructed() {
	s.NotNil(s.cmd)
	s.Equal("host.list", s.cmd.Name())
	f, ok := GetCommandFactory("host.list")
	s.True(ok)
	s.Equal(f(), s.cmd)
}

func (s *HostListSuite) TestEarlyValidationAvoidsBlockingForNothing() {
	s.NoError(s.cmd.ParseParams(nil))
	s.cmd.Wait = true
	s.cmd.NumHosts = "0"
	s.Error(s.cmd.ParseParams(nil))
	s.cmd.NumHosts = "1"
	s.NoError(s.cmd.ParseParams(nil))
}

func (s *HostListSuite) TestEarlyValidationWithoutOutputIsAnError() {
	s.NoError(s.cmd.ParseParams(nil))
	s.cmd.Path = ""
	s.cmd.Silent = true
	s.cmd.Wait = false
	s.Error(s.cmd.ParseParams(nil))
	s.cmd.Path = "foo"
	s.NoError(s.cmd.ParseParams(nil))
}

func (s *HostListSuite) TestEarlyValidationTimeNonsense() {
	s.NoError(s.cmd.ParseParams(nil))
	s.cmd.TimeoutSecs = -1
	s.Error(s.cmd.ParseParams(nil))
	s.cmd.TimeoutSecs = 4
	s.NoError(s.cmd.ParseParams(nil))
}

func (s *HostListSuite) TestMockExecuteUnconfigured() {
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *HostListSuite) TestMockExecuteWithWait() {
	s.cmd.Wait = true
	s.cmd.NumHosts = "1"
	s.cmd.TimeoutSecs = 1
	s.Error(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *HostListSuite) TestExpansions() {
	s.NoError(s.cmd.ParseParams(
		map[string]any{
			"num_hosts": 2,
		},
	))
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Equal("2", s.cmd.NumHosts)

	s.NoError(s.cmd.ParseParams(
		map[string]any{
			"num_hosts": "${foo}",
		},
	))
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Equal("3", s.cmd.NumHosts)
}

func TestHostListSetsWorkdirBoundaryAttributeOnViolation(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, tp.Shutdown(ctx))
	})

	ctx, span := tp.Tracer("test").Start(t.Context(), "test")

	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		WorkDir:    "/data/mci/work",
		Expansions: util.Expansions{},
		Task:       task.Task{},
		Project:    model.Project{},
	}
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	cmd := listHostFactory().(*listHosts)
	cmd.Path = "/etc/passwd"
	cmd.Silent = true

	// Execute may return an error (e.g. cannot write to /etc/passwd), but
	// the workdir boundary attribute is set before any file I/O.
	_ = cmd.Execute(ctx, comm, logger, conf)
	span.End()

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)

	found := false
	for _, attr := range ended[0].Attributes() {
		if string(attr.Key) == workdirBoundaryViolationAttribute {
			assert.True(t, attr.Value.AsBool(), "workdir boundary violation attribute should be true")
			found = true
		}
	}
	assert.True(t, found, "workdir boundary violation attribute was not set on the span")
}

func TestHostListDoesNotSetWorkdirBoundaryAttributeForRelativePath(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, tp.Shutdown(ctx))
	})

	ctx, span := tp.Tracer("test").Start(t.Context(), "test")

	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		WorkDir:    "/data/mci/work",
		Expansions: util.Expansions{},
		Task:       task.Task{},
		Project:    model.Project{},
	}
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	cmd := listHostFactory().(*listHosts)
	cmd.Path = "hosts.json"
	cmd.Silent = true

	_ = cmd.Execute(ctx, comm, logger, conf)
	span.End()

	ended := spanRecorder.Ended()
	require.Len(t, ended, 1)

	for _, attr := range ended[0].Attributes() {
		if string(attr.Key) == workdirBoundaryViolationAttribute {
			t.Fatalf("workdir boundary violation attribute should not be set for a relative path, got value %v", attr.Value.AsBool())
		}
	}
}
