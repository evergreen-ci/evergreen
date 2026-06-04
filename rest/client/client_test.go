package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kanopy-platform/kanopy-oidc-lib/pkg/dex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
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

func TestIsOAuthLockClaimedError(t *testing.T) {
	t.Run("NilErrorShouldReturnFalse", func(t *testing.T) {
		assert.False(t, isOAuthLockClaimedError(nil))
	})

	t.Run("WrappedDexLockErrorShouldReturnTrue", func(t *testing.T) {
		assert.True(t, isOAuthLockClaimedError(fmt.Errorf("lock claimed by process 123: %w", dex.ErrLockClaimed)))
	})

	t.Run("DexLockMessageShouldReturnTrue", func(t *testing.T) {
		assert.True(t, isOAuthLockClaimedError(fmt.Errorf("lock claimed by process 123: lock file claimed by another process")))
	})

	t.Run("UnrelatedErrorShouldReturnFalse", func(t *testing.T) {
		assert.False(t, isOAuthLockClaimedError(fmt.Errorf("refresh token expired")))
	})
}

func TestRemoveInvalidOAuthTokenCacheIfUnlocked(t *testing.T) {
	t.Run("MissingTokenShouldReturnImmediately", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")

		require.NoError(t, removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, time.Second))
	})

	t.Run("ValidTokenShouldRemain", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")
		tokenData, err := json.Marshal(&oauth2.Token{
			AccessToken:  "access",
			RefreshToken: "refresh",
			Expiry:       time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(tokenFilePath, tokenData, 0600))

		require.NoError(t, removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, time.Second))

		_, err = os.Stat(tokenFilePath)
		assert.NoError(t, err)
	})

	t.Run("ZeroExpiryTokenShouldBeRemoved", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")
		tokenData, err := json.Marshal(&oauth2.Token{
			AccessToken:  "access",
			RefreshToken: "refresh",
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(tokenFilePath, tokenData, 0600))

		require.NoError(t, removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, time.Second))

		_, err = os.Stat(tokenFilePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("ExpiredTokenWithExpiryShouldRemain", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")
		tokenData, err := json.Marshal(&oauth2.Token{
			AccessToken:  "access",
			RefreshToken: "refresh",
			Expiry:       time.Now().Add(-time.Hour),
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(tokenFilePath, tokenData, 0600))

		require.NoError(t, removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, time.Second))

		_, err = os.Stat(tokenFilePath)
		assert.NoError(t, err)
	})

	t.Run("InvalidTokenShouldBeRemoved", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")
		require.NoError(t, os.WriteFile(tokenFilePath, []byte("{"), 0600))

		require.NoError(t, removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, time.Second))

		_, err := os.Stat(tokenFilePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("StaleLockShouldAllowInvalidTokenRemoval", func(t *testing.T) {
		cmd := exec.Command("true")
		require.NoError(t, cmd.Run())

		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")
		lockFilePath := tokenFilePath + ".lock"
		require.NoError(t, os.WriteFile(tokenFilePath, []byte("{"), 0600))
		require.NoError(t, os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0600))

		require.NoError(t, removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, time.Second))

		_, tokenErr := os.Stat(tokenFilePath)
		assert.True(t, os.IsNotExist(tokenErr))
		_, lockErr := os.Stat(lockFilePath)
		assert.True(t, os.IsNotExist(lockErr))
	})

	t.Run("ActiveLockShouldNotRemoveInvalidToken", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFilePath := filepath.Join(tmpDir, "token.json")
		lockFilePath := tokenFilePath + ".lock"
		require.NoError(t, os.WriteFile(tokenFilePath, []byte("{"), 0600))
		require.NoError(t, os.WriteFile(lockFilePath, []byte(fmt.Sprintf("%d", os.Getpid())), 0600))

		err := removeInvalidOAuthTokenCacheIfUnlocked(t.Context(), tokenFilePath, 300*time.Millisecond)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")

		_, tokenErr := os.Stat(tokenFilePath)
		assert.NoError(t, tokenErr)
		_, lockErr := os.Stat(lockFilePath)
		assert.NoError(t, lockErr)
	})
}

func TestValidateOAuthToken(t *testing.T) {
	t.Run("ValidTokenShouldPass", func(t *testing.T) {
		err := validateOAuthToken(&oauth2.Token{
			AccessToken: "access",
			Expiry:      time.Now().Add(time.Hour),
		})
		assert.NoError(t, err)
	})

	t.Run("NilTokenShouldError", func(t *testing.T) {
		err := validateOAuthToken(nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, errInvalidOAuthToken))
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("EmptyAccessTokenShouldError", func(t *testing.T) {
		err := validateOAuthToken(&oauth2.Token{
			Expiry: time.Now().Add(time.Hour),
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, errInvalidOAuthToken))
		assert.Contains(t, err.Error(), "access token")
	})

	t.Run("ZeroExpiryShouldError", func(t *testing.T) {
		err := validateOAuthToken(&oauth2.Token{
			AccessToken: "access",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, errInvalidOAuthToken))
		assert.Contains(t, err.Error(), "expiry")
	})

	t.Run("ExpiredTokenShouldError", func(t *testing.T) {
		err := validateOAuthToken(&oauth2.Token{
			AccessToken: "access",
			Expiry:      time.Now().Add(-time.Hour),
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, errInvalidOAuthToken))
		assert.Contains(t, err.Error(), "expired")
	})
}

func TestRemoveOAuthTokenFile(t *testing.T) {
	t.Run("MissingPathShouldNoop", func(t *testing.T) {
		assert.NoError(t, removeOAuthTokenFile(""))
	})

	t.Run("MissingFileShouldNoop", func(t *testing.T) {
		tokenFilePath := filepath.Join(t.TempDir(), "token.json")
		assert.NoError(t, removeOAuthTokenFile(tokenFilePath))
	})

	t.Run("ExistingFileShouldBeRemoved", func(t *testing.T) {
		tokenFilePath := filepath.Join(t.TempDir(), "token.json")
		require.NoError(t, os.WriteFile(tokenFilePath, []byte("{}"), 0600))

		require.NoError(t, removeOAuthTokenFile(tokenFilePath))

		_, err := os.Stat(tokenFilePath)
		assert.True(t, os.IsNotExist(err))
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
