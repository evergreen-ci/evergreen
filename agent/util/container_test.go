package util

import (
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapWithContainer(t *testing.T) {
	baseArgs := []string{"/bin/bash", "-c", "echo hello"}

	makeOpts := func() *options.Create {
		return &options.Create{Args: append([]string{}, baseArgs...)}
	}

	t.Run("EmptyContainerID", func(t *testing.T) {
		opts := makeOpts()
		WrapWithContainer(opts, "")
		assert.Equal(t, baseArgs, opts.Args)
	})

	t.Run("WithContainerID", func(t *testing.T) {
		opts := makeOpts()
		WrapWithContainer(opts, "abc123def")
		require.GreaterOrEqual(t, len(opts.Args), len(baseArgs)+3)
		assert.Equal(t, "docker", opts.Args[0])
		assert.Equal(t, "exec", opts.Args[1])
		assert.Equal(t, "abc123def", opts.Args[2])
		assert.Equal(t, baseArgs, opts.Args[len(opts.Args)-len(baseArgs):])
	})

	t.Run("PreservesOriginalArgs", func(t *testing.T) {
		opts := makeOpts()
		original := append([]string{}, opts.Args...)
		WrapWithContainer(opts, "xyz789")
		assert.Equal(t, original, opts.Args[3:])
	})
}
