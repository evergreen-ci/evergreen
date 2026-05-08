package client

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvergreenCommunicatorConstructor(t *testing.T) {
	t.Run("FailsWithoutServerURL", func(t *testing.T) {
		client, err := NewCommunicator("")
		assert.Error(t, err)
		assert.Zero(t, client)
	})
	client, err := NewCommunicator("url")
	require.NoError(t, err)
	defer client.Close()

	c, ok := client.(*communicatorImpl)
	assert.True(t, ok, true)
	assert.Empty(t, c.apiUser)
	assert.Empty(t, c.apiKey)
	assert.Equal(t, defaultMaxAttempts, c.maxAttempts)
	assert.Equal(t, defaultTimeoutStart, c.timeoutStart)
	assert.Equal(t, defaultTimeoutMax, c.timeoutMax)

	client.SetAPIUser("apiUser")
	client.SetAPIKey("apiKey")
	assert.Equal(t, "apiUser", c.apiUser)
	assert.Equal(t, "apiKey", c.apiKey)
}

// TestRemoveStaleOAuthLockFile verifies lock files are only removed when the owning process is dead.
func TestRemoveStaleOAuthLockFile(t *testing.T) {
	t.Run("PreservesActiveLock", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockFilePath := filepath.Join(tmpDir, "token.json.lock")
		require.NoError(t, os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", os.Getpid())), 0600))

		require.NoError(t, removeStaleOAuthLockFile(lockFilePath))

		_, err := os.Stat(lockFilePath)
		assert.NoError(t, err, "Lock owned by live process should not be removed")
	})

	t.Run("RemovesStaleLock", func(t *testing.T) {
		cmd := exec.Command("true")
		require.NoError(t, cmd.Run())
		deadPID := cmd.Process.Pid

		tmpDir := t.TempDir()
		lockFilePath := filepath.Join(tmpDir, "token.json.lock")
		require.NoError(t, os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", deadPID)), 0600))

		require.NoError(t, removeStaleOAuthLockFile(lockFilePath))

		_, err := os.Stat(lockFilePath)
		assert.True(t, os.IsNotExist(err), "Lock owned by dead process should be removed")
	})

	t.Run("NoErrorWhenFileDoesNotExist", func(t *testing.T) {
		assert.NoError(t, removeStaleOAuthLockFile("/nonexistent/path"))
	})
}
