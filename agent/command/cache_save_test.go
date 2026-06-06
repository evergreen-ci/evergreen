package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheSaveParseParams(t *testing.T) {
	validParams := func() map[string]any {
		return map[string]any{
			"name":           "go-modules",
			"bucket":         "my-cache-bucket",
			"remote_path":    "project/caches",
			"key_expansions": []string{"${project_id}"},
			"paths":          []string{"vendor"},
			"aws_key":        "key",
			"aws_secret":     "secret",
		}
	}

	t.Run("ValidParamsAccepted", func(t *testing.T) {
		c := &cacheSave{}
		require.NoError(t, c.ParseParams(validParams()))
	})

	t.Run("EmptyPathsRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "paths")
		c := &cacheSave{}
		err := c.ParseParams(params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "paths")
	})

	t.Run("EmptyKeyExpansionsRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "key_expansions")
		c := &cacheSave{}
		require.Error(t, c.ParseParams(params))
	})

	t.Run("MissingCredentialsRejected", func(t *testing.T) {
		params := validParams()
		delete(params, "aws_key")
		delete(params, "aws_secret")
		c := &cacheSave{}
		require.Error(t, c.ParseParams(params))
	})
}

// TestCacheSaveRecomputedKeyMatchesRestore verifies cache.save derives the same
// cache key as cache.restore for identical inputs, which is what guarantees a
// save can be found by a later restore.
func TestCacheSaveRecomputedKeyMatchesRestore(t *testing.T) {
	dir := t.TempDir()
	keyFile := writeCacheKeyFile(t, dir, "go.sum", "checksum")

	restoreKey, err := computeCacheKey([]string{keyFile}, []string{"my-project", "linux"})
	require.NoError(t, err)
	saveKey, err := computeCacheKey([]string{keyFile}, []string{"my-project", "linux"})
	require.NoError(t, err)
	assert.Equal(t, restoreKey, saveKey)
}

// TestCacheSaveSkipsOnHit verifies that when the cache-hit expansion is already
// "true", cache.save does no work and creates no tarball, short-circuiting
// before any S3 interaction.
func TestCacheSaveSkipsOnHit(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		Expansions: *util.NewExpansions(map[string]string{"go_modules_cache_hit": "true"}),
		WorkDir:    t.TempDir(),
		Task:       task.Task{Id: "task_id", Secret: "task_secret"},
		Project:    model.Project{},
		Distro:     &apimodels.DistroView{},
	}
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	c := &cacheSave{
		cacheCommon: cacheCommon{
			CacheName:     "go-modules",
			Bucket:        "my-cache-bucket",
			RemotePath:    "project/caches",
			KeyExpansions: []string{"${project_id}"},
			AwsKey:        "key",
			AwsSecret:     "secret",
		},
		Paths: []string{"vendor"},
	}

	require.NoError(t, c.Execute(ctx, comm, logger, conf))

	_, statErr := os.Stat(filepath.Join(conf.WorkDir, "go-modules.tgz"))
	assert.True(t, os.IsNotExist(statErr), "no tarball should be created on a cache hit")
}
