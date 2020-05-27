package utility

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFile(t *testing.T) {
	t.Run("FileExistance", func(t *testing.T) {
		defer func() { assert.NoError(t, os.RemoveAll("foo")) }()
		require.False(t, FileExists("foo"))
		require.NoError(t, WriteFile("foo", "hello world"))
		require.True(t, FileExists("foo"))
	})
	t.Run("WriteFile", func(t *testing.T) {
		defer func() { assert.NoError(t, os.RemoveAll("foo")) }()
		require.Error(t, WriteFile("/usr/local", "hi"))
		require.NoError(t, WriteFile("foo", "hello world"))
	})
}
