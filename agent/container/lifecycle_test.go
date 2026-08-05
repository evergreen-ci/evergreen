package container

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigValidation(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		cfg := Config{
			Image:   "ubuntu:22.04",
			WorkDir: "/tmp/work",
			TaskID:  "task123",
		}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("MissingImage", func(t *testing.T) {
		cfg := Config{
			WorkDir: "/tmp/work",
			TaskID:  "task123",
		}
		assert.Error(t, cfg.Validate())
	})

	t.Run("MissingWorkDir", func(t *testing.T) {
		cfg := Config{
			Image:  "ubuntu:22.04",
			TaskID: "task123",
		}
		assert.Error(t, cfg.Validate())
	})

	t.Run("RelativeWorkDirRejected", func(t *testing.T) {
		cfg := Config{
			Image:   "ubuntu:22.04",
			WorkDir: "relative/path",
			TaskID:  "task123",
		}
		assert.ErrorContains(t, cfg.Validate(), "work directory must be absolute")
	})

	t.Run("MissingTaskID", func(t *testing.T) {
		cfg := Config{
			Image:   "ubuntu:22.04",
			WorkDir: "/tmp/work",
		}
		assert.Error(t, cfg.Validate())
	})

	t.Run("NegativeMemoryRejected", func(t *testing.T) {
		cfg := Config{
			Image:    "ubuntu:22.04",
			WorkDir:  "/tmp/work",
			TaskID:   "task123",
			MemoryMB: -1,
		}
		assert.ErrorContains(t, cfg.Validate(), "memory limit cannot be negative")
	})

	t.Run("NegativeCPUsRejected", func(t *testing.T) {
		cfg := Config{
			Image:   "ubuntu:22.04",
			WorkDir: "/tmp/work",
			TaskID:  "task123",
			CPUs:    -1,
		}
		assert.ErrorContains(t, cfg.Validate(), "CPU limit cannot be negative")
	})
}

func TestContainerName(t *testing.T) {
	cfg := Config{TaskID: "abc123_def456_24_08_01_12_00_00"}
	name := cfg.containerName()
	assert.Equal(t, "evergreen-task-abc123_def456_24_08_01_12_00_00", name)
}

func TestEnvHostDir(t *testing.T) {
	dir := envHostDir("abc123_def456_24_08_01_12_00_00")
	assert.Equal(t, filepath.Join("/var/run/evergreen-env", "abc123_def456_24_08_01_12_00_00"), dir)
	assert.True(t, filepath.IsAbs(dir), "envHostDir should return an absolute path")
}

func TestExtraMountsValidation(t *testing.T) {
	base := Config{
		Image:   "ubuntu:22.04",
		WorkDir: "/tmp/work",
		TaskID:  "task123",
	}

	t.Run("EmptyExtraMounts", func(t *testing.T) {
		cfg := base
		assert.NoError(t, cfg.Validate())
	})

	t.Run("AbsoluteReadOnlyMount", func(t *testing.T) {
		cfg := base
		cfg.ExtraMounts = []Mount{{Source: "/opt", Target: "/opt", ReadOnly: true}}
		assert.NoError(t, cfg.Validate())
	})

	t.Run("RelativeSourceRejected", func(t *testing.T) {
		cfg := base
		cfg.ExtraMounts = []Mount{{Source: "opt", Target: "/opt"}}
		assert.ErrorContains(t, cfg.Validate(), "source must be absolute")
	})

	t.Run("RelativeTargetRejected", func(t *testing.T) {
		cfg := base
		cfg.ExtraMounts = []Mount{{Source: "/opt", Target: "opt"}}
		assert.ErrorContains(t, cfg.Validate(), "target must be absolute")
	})
}

type startRemoveClientMock struct {
	startErr     error
	removeCalled bool
	removeCtxErr error
}

func (c *startRemoveClientMock) ContainerStart(context.Context, string, dockercontainer.StartOptions) error {
	return c.startErr
}

func (c *startRemoveClientMock) ContainerRemove(ctx context.Context, _ string, _ dockercontainer.RemoveOptions) error {
	c.removeCalled = true
	c.removeCtxErr = ctx.Err()
	return nil
}

func TestStartContainerUsesDetachedContextForCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cli := &startRemoveClientMock{startErr: errors.New("start error")}

	err := startContainer(ctx, cli, "container-id")
	require.Error(t, err)
	assert.True(t, cli.removeCalled)
	assert.NoError(t, cli.removeCtxErr)
}
