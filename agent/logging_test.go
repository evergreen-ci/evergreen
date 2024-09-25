package agent

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSenderLocal(t *testing.T) {
	assert := assert.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := (&Agent{}).GetSender(ctx, globals.LogOutputStdout, "agent", "task_id", 2)
	assert.NoError(err)
}

func TestStartLogging(t *testing.T) {
	tmpDirName := t.TempDir()
	agt := &Agent{
		opts: Options{
			HostID:           "host",
			HostSecret:       "secret",
			StatusPort:       2286,
			LogOutput:        globals.LogOutputStdout,
			LogPrefix:        "agent",
			WorkingDirectory: tmpDirName,
			HomeDirectory:    tmpDirName,
		},
		comm: client.NewMock("url"),
	}
	tc := &taskContext{
		task: client.TaskData{
			ID:     "logging",
			Secret: "task_secret",
		},
		oomTracker: &mock.OOMTracker{},
	}

	ctx := context.Background()
	config, err := agt.makeTaskConfig(ctx, tc)
	require.NoError(t, err)
	tc.taskConfig = config

	require.NoError(t, agt.startLogging(ctx, tc))
	tc.logger.Execution().Info("foo")
	assert.NoError(t, tc.logger.Close())
	lines := agt.comm.(*client.Mock).GetTaskLogs(tc.taskConfig.Task.Id)
	require.Len(t, lines, 1)
	assert.Equal(t, "foo", lines[0].Data)
}
