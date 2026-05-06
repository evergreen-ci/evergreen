package client

import (
	"encoding/json"
	"fmt"
	"os"
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

// Verifies lock files are only removed when the owning process is dead.
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
		tmpDir := t.TempDir()
		lockFilePath := filepath.Join(tmpDir, "token.json.lock")
		require.NoError(t, os.WriteFile(lockFilePath, []byte("99999999"), 0600))

		require.NoError(t, removeStaleOAuthLockFile(lockFilePath))

		_, err := os.Stat(lockFilePath)
		assert.True(t, os.IsNotExist(err), "Lock owned by dead process should be removed")
	})

	t.Run("NoErrorWhenFileDoesNotExist", func(t *testing.T) {
		assert.NoError(t, removeStaleOAuthLockFile("/nonexistent/path"))
	})
}

// Verifies the no-write loader prevents overwriting the refresh token on disk.
func TestTokenLoaderNoWrite(t *testing.T) {
	tmpDir := t.TempDir()
	tokenFilePath := filepath.Join(tmpDir, "token.json")

	original := &oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "precious-refresh",
		Expiry:       time.Now().Add(10 * time.Minute),
		TokenType:    "Bearer",
	}
	data, err := json.Marshal(original)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenFilePath, data, 0600))

	loader := &tokenLoaderNoWrite{&dex.FileTokenLoader{}}

	loaded, err := loader.LoadToken(tokenFilePath)
	require.NoError(t, err)
	assert.Empty(t, loaded.RefreshToken, "LoadToken should clear RefreshToken")

	err = loader.SaveToken(tokenFilePath, &oauth2.Token{AccessToken: "new", TokenType: "Bearer"})
	require.NoError(t, err)

	fileData, err := os.ReadFile(tokenFilePath)
	require.NoError(t, err)
	var saved oauth2.Token
	require.NoError(t, json.Unmarshal(fileData, &saved))
	assert.Equal(t, "precious-refresh", saved.RefreshToken, "SaveToken should be a no-op")
}
