package client

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

func TestWaitForOAuthLockRelease(t *testing.T) {
	t.Run("MissingLockShouldReturnImmediately", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockFilePath := filepath.Join(tmpDir, "token.json.lock")

		err := waitForOAuthLockRelease(t.Context(), lockFilePath, time.Second)
		require.NoError(t, err)
	})

	t.Run("StaleLockShouldBeRemovedAndReturn", func(t *testing.T) {
		cmd := exec.Command("true")
		require.NoError(t, cmd.Run())

		tmpDir := t.TempDir()
		lockFilePath := filepath.Join(tmpDir, "token.json.lock")
		require.NoError(t, os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0600))

		err := waitForOAuthLockRelease(t.Context(), lockFilePath, time.Second)
		require.NoError(t, err)

		_, statErr := os.Stat(lockFilePath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("ActiveLockShouldTimeout", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockFilePath := filepath.Join(tmpDir, "token.json.lock")
		require.NoError(t, os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", os.Getpid())), 0600))

		start := time.Now()
		err := waitForOAuthLockRelease(t.Context(), lockFilePath, 300*time.Millisecond)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond)
	})
}

func TestIsPortBindError(t *testing.T) {
	t.Run("NilErrorShouldReturnFalse", func(t *testing.T) {
		assert.False(t, isPortBindError(nil))
	})

	t.Run("PortBindErrorShouldReturnTrue", func(t *testing.T) {
		assert.True(t, isPortBindError(fmt.Errorf("listen tcp 127.0.0.1:8888: bind: address already in use")))
	})

	t.Run("UnrelatedErrorShouldReturnFalse", func(t *testing.T) {
		assert.False(t, isPortBindError(fmt.Errorf("refresh token expired")))
	})
}

func TestCallbackPortAvailable(t *testing.T) {
	t.Run("FreePortShouldReturnTrue", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := listener.Addr().(*net.TCPAddr).Port
		require.NoError(t, listener.Close())

		assert.True(t, callbackPortAvailable(fmt.Sprintf("%d", port)))
	})

	t.Run("UsedPortShouldReturnFalse", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		port := listener.Addr().(*net.TCPAddr).Port
		t.Cleanup(func() {
			assert.NoError(t, listener.Close())
		})

		assert.False(t, callbackPortAvailable(fmt.Sprintf("%d", port)))
	})
}
