package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	t.Run("MissingTaskID", func(t *testing.T) {
		cfg := Config{
			Image:   "ubuntu:22.04",
			WorkDir: "/tmp/work",
		}
		assert.Error(t, cfg.Validate())
	})
}

func TestContainerName(t *testing.T) {
	cfg := Config{TaskID: "abc123_def456_24_08_01_12_00_00"}
	name := cfg.containerName()
	assert.Equal(t, "evergreen-task-abc123_def456_24_08_01_12_00_00", name)
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
