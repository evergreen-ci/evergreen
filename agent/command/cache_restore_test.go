package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheRestoreParseParams(t *testing.T) {
	validParams := func() map[string]any {
		return map[string]any{
			"name":           "go-modules",
			"bucket":         "my-cache-bucket",
			"remote_path":    "project/caches",
			"key_expansions": []string{"${project_id}"},
			"aws_key":        "key",
			"aws_secret":     "secret",
		}
	}

	t.Run("ValidParamsAccepted", func(t *testing.T) {
		c := &cacheRestore{}
		require.NoError(t, c.ParseParams(validParams()))
	})

	t.Run("EmptyKeyExpansionsRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "key_expansions")
		c := &cacheRestore{}
		err := c.ParseParams(params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key_expansions")
	})

	t.Run("MissingCredentialsRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "aws_key")
		delete(params, "aws_secret")
		c := &cacheRestore{}
		require.Error(t, c.ParseParams(params))
	})

	t.Run("RoleARNWithStaticCredentialsRejected", func(t *testing.T) {
		params := validParams()
		params["role_arn"] = "arn:aws:iam::123456789012:role/cache"
		c := &cacheRestore{}
		require.Error(t, c.ParseParams(params))
	})

	t.Run("RoleARNWithoutStaticCredentialsAccepted", func(t *testing.T) {
		params := validParams()
		delete(params, "aws_key")
		delete(params, "aws_secret")
		params["role_arn"] = "arn:aws:iam::123456789012:role/cache"
		c := &cacheRestore{}
		require.NoError(t, c.ParseParams(params))
	})

	t.Run("MissingNameRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "name")
		c := &cacheRestore{}
		require.Error(t, c.ParseParams(params))
	})

	t.Run("InvalidNameRejected", func(t *testing.T) {
		params := validParams()
		params["name"] = "has spaces"
		c := &cacheRestore{}
		err := c.ParseParams(params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("MissingRemotePathRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "remote_path")
		c := &cacheRestore{}
		require.Error(t, c.ParseParams(params))
	})

	t.Run("DashesInNameAllowed", func(t *testing.T) {
		params := validParams()
		params["name"] = "mise-and-go"
		c := &cacheRestore{}
		require.NoError(t, c.ParseParams(params))
		assert.Equal(t, "mise_and_go_cache_hit", cacheHitExpansionName(c.CacheName))
	})
}
