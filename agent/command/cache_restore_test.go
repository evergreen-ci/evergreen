package command

import (
	"testing"

	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/pail"
	"github.com/pkg/errors"
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

	t.Run("PreserveSymlinksDecodedAndDefaultsToFalse", func(t *testing.T) {
		c := &cacheRestore{}
		require.NoError(t, c.ParseParams(validParams()))
		assert.False(t, c.PreserveSymlinks)

		params := validParams()
		params["preserve_symlinks"] = true
		c = &cacheRestore{}
		require.NoError(t, c.ParseParams(params))
		assert.True(t, c.PreserveSymlinks)
	})

	t.Run("ExpandableNameDeferredToExpansion", func(t *testing.T) {
		// A templated name passes parse-time validation; the resolved value is
		// re-validated after expansions are applied.
		params := validParams()
		params["name"] = "${cache_name}"
		c := &cacheRestore{}
		require.NoError(t, c.ParseParams(params))
	})
}

func TestClassifyCacheDownloadErr(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		expected cacheDownloadOutcome
	}{
		{
			name:     "KeyNotFoundIsMiss",
			err:      pail.NewKeyNotFoundError("no such key"),
			expected: cacheDownloadMiss,
		},
		{
			name:     "AccessDeniedIsMaybeMiss",
			err:      &smithy.GenericAPIError{Code: "AccessDenied", Fault: smithy.FaultClient},
			expected: cacheDownloadMaybeMiss,
		},
		{
			name:     "WrappedAccessDeniedIsMaybeMiss",
			err:      errors.Wrap(&smithy.GenericAPIError{Code: "AccessDenied", Fault: smithy.FaultClient}, "downloading"),
			expected: cacheDownloadMaybeMiss,
		},
		{
			name:     "OtherClientErrorIsFatal",
			err:      &smithy.GenericAPIError{Code: "InvalidRequest", Fault: smithy.FaultClient},
			expected: cacheDownloadFatal,
		},
		{
			name:     "ServerErrorIsRetried",
			err:      &smithy.GenericAPIError{Code: "InternalError", Fault: smithy.FaultServer},
			expected: cacheDownloadRetry,
		},
		{
			name:     "NonAPIErrorIsRetried",
			err:      errors.New("connection reset by peer"),
			expected: cacheDownloadRetry,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, classifyCacheDownloadErr(tc.err))
		})
	}
}
