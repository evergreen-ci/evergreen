package agent

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturingManager wraps a mock jasper manager and records the process
// options for every created process in a race-safe way, so tests can inspect
// the argv of processes created by the stats collector goroutine.
type capturingManager struct {
	*mock.Manager

	mu   sync.Mutex
	opts []*options.Create
}

func newCapturingManager() *capturingManager {
	return &capturingManager{Manager: &mock.Manager{}}
}

// CreateCommand routes every command's default process constructor through
// CreateProcess below so its options get captured. Constructors registered
// later by the caller (e.g. the ps command's container wrapping) take
// precedence over this one.
func (m *capturingManager) CreateCommand(ctx context.Context) *jasper.Command {
	cmd := m.Manager.CreateCommand(ctx)
	cmd.ProcConstructor(func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
		return m.CreateProcess(ctx, opts)
	})
	return cmd
}

func (m *capturingManager) CreateProcess(ctx context.Context, opts *options.Create) (jasper.Process, error) {
	m.mu.Lock()
	copied := *opts
	copied.Args = slices.Clone(opts.Args)
	m.opts = append(m.opts, &copied)
	m.mu.Unlock()
	return m.Manager.CreateProcess(ctx, opts)
}

func (m *capturingManager) snapshot() []*options.Create {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.opts)
}

// makeStatsTestLogger returns a LoggerProducer whose senders render messages
// eagerly, reproducing the conditions under which wrapping a nil error in a
// grip error composer panics.
func makeStatsTestLogger(t *testing.T) client.LoggerProducer {
	t.Helper()
	comm := client.NewMock("")
	logger, err := comm.GetLoggerProducer(t.Context(), &task.Task{Id: "task_id"}, nil)
	require.NoError(t, err)
	return logger
}

func TestStatsCollectorPSCommandRunsInsideContainer(t *testing.T) {
	jpm := newCapturingManager()
	logger := makeStatsTestLogger(t)

	conf := &internal.TaskConfig{
		ContainerID:    "container-id",
		WorkDir:        "/task/workdir",
		EnvFileHostDir: "/host/envdir",
		Distro:         &apimodels.DistroView{ExecUser: "task-user"},
	}

	collector := NewSimpleStatsCollector(logger, jpm, time.Hour, "uptime", "df -h")
	collector.setPSCommand("sh -c 'id > /tmp/proof'", conf)
	collector.logStats(t.Context(), util.Expansions{})

	var psProc *options.Create
	var allProcs []*options.Create
	assert.Eventually(t, func() bool {
		allProcs = jpm.snapshot()
		for _, opts := range allProcs {
			if len(opts.Args) > 0 && opts.Args[0] == "docker" {
				psProc = opts
			}
		}
		return psProc != nil
	}, 10*time.Second, 10*time.Millisecond, "expected the ps command to be wrapped for container execution")

	require.NotNil(t, psProc)
	assert.Equal(t, []string{"docker", "exec", "-i", "--workdir=/task/workdir", "--user=task-user", "container-id"}, psProc.Args[:6])
	assert.Equal(t, []string{"sh", "-c", "id > /tmp/proof"}, psProc.Args[len(psProc.Args)-3:])

	// The author-controlled payload must never be handed to the host manager
	// unwrapped: the only processes that may run outside the container are
	// the agent-fixed host diagnostics.
	for _, opts := range allProcs {
		switch opts.Args[0] {
		case "docker", "uptime", "df":
		default:
			assert.NotEqual(t, "sh", opts.Args[0], "ps payload must not execute on the host")
		}
	}
}

func TestStatsCollectorNoContainerRunsPSOnHost(t *testing.T) {
	for _, conf := range []*internal.TaskConfig{
		{},
		{Distro: &apimodels.DistroView{ExecUser: "task-user"}},
	} {
		jpm := newCapturingManager()
		logger := makeStatsTestLogger(t)

		collector := NewSimpleStatsCollector(logger, jpm, time.Hour)
		collector.setPSCommand("ps -o pid", conf)
		collector.logStats(t.Context(), util.Expansions{})

		var psProcs []*options.Create
		assert.Eventually(t, func() bool {
			for _, opts := range jpm.snapshot() {
				if len(opts.Args) > 0 && opts.Args[0] == "ps" {
					psProcs = append(psProcs, opts)
				}
			}
			return len(psProcs) > 0
		}, 10*time.Second, 10*time.Millisecond)

		require.Len(t, psProcs, 1)
		// Without a container, the ps command runs as the agent user with no
		// wrapping or sudo prefix, preserving historical behavior.
		assert.Equal(t, []string{"ps", "-o", "pid"}, psProcs[0].Args)
	}
}

func TestStatsCollectorHostCommandsStayOnHost(t *testing.T) {
	jpm := newCapturingManager()
	logger := makeStatsTestLogger(t)

	conf := &internal.TaskConfig{
		ContainerID:    "container-id",
		WorkDir:        "/task/workdir",
		EnvFileHostDir: "/host/envdir",
		Distro:         &apimodels.DistroView{ExecUser: "task-user"},
	}

	collector := NewSimpleStatsCollector(logger, jpm, time.Hour, "uptime", "df -h")
	collector.setPSCommand("ps -o pid", conf)
	collector.logStats(t.Context(), util.Expansions{})

	assert.Eventually(t, func() bool {
		for _, opts := range jpm.snapshot() {
			if len(opts.Args) > 0 && opts.Args[0] == "df" {
				// Host diagnostics are agent-fixed values and are never
				// wrapped into the container.
				assert.Equal(t, []string{"df", "-h"}, opts.Args)
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)
}

func TestStatsCollectorSuccessfulRunsDoNotKillCollector(t *testing.T) {
	jpm := newCapturingManager()
	logger := makeStatsTestLogger(t)

	conf := &internal.TaskConfig{ContainerID: "container-id", WorkDir: "/task/workdir"}

	collector := NewSimpleStatsCollector(logger, jpm, 50*time.Millisecond, "uptime")
	collector.setPSCommand("ps -o pid", conf)
	collector.logStats(t.Context(), util.Expansions{})

	// A successful collection must not panic the collector goroutine, so
	// iterations continue and processes keep getting created.
	assert.Eventually(t, func() bool {
		return len(jpm.snapshot()) >= 3
	}, 10*time.Second, 50*time.Millisecond, "expected the collector to keep running across iterations")
}
