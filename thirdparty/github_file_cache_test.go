package thirdparty

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/pail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testCacheOwner = "test-owner"
	testCacheRepo  = "test-repo"
	testCacheFile  = "config.yml"
	testCacheBody  = "include:\n  - other.yml\n"
)

// createTestGitRepo creates a local git repo containing one file, standing in
// for the clone that GetGitHubFileFromGit would normally fetch from GitHub.
func createTestGitRepo(t *testing.T, filename, content string) string {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("init", "--initial-branch=main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
	run("add", filename)
	run("commit", "-m", "initial commit")
	return dir
}

// newTestCacheBucket returns a local filesystem bucket standing in for the S3
// cache bucket.
func newTestCacheBucket(t *testing.T) pail.Bucket {
	bucket, err := pail.NewLocalBucket(pail.LocalOptions{
		Path:     t.TempDir(),
		UseSlash: true,
	})
	require.NoError(t, err)
	return bucket
}

// setTestCacheBucket injects bucket for the duration of the test and restores
// the previous bucket afterwards.
func setTestCacheBucket(t *testing.T, bucket pail.Bucket) {
	orig := githubFileCacheBucketFn
	githubFileCacheBucketFn = func(context.Context) pail.Bucket { return bucket }
	t.Cleanup(func() { githubFileCacheBucketFn = orig })
}

// cacheObjectCount counts the objects in the bucket.
func cacheObjectCount(t *testing.T, bucket pail.Bucket) int {
	it, err := bucket.List(t.Context(), "")
	require.NoError(t, err)
	count := 0
	for it.Next(t.Context()) {
		count++
	}
	require.NoError(t, it.Err())
	return count
}

// getCacheObject returns the bytes stored under key.
func getCacheObject(t *testing.T, bucket pail.Bucket, key string) []byte {
	r, err := bucket.Get(t.Context(), key)
	require.NoError(t, err)
	defer r.Close()
	content, err := io.ReadAll(r)
	require.NoError(t, err)
	return content
}

func TestGetGitHubFileFromGitWithCache(t *testing.T) {
	t.Run("MissFetchesAndPopulatesCacheThenSecondCallHits", func(t *testing.T) {
		dir := createTestGitRepo(t, testCacheFile, testCacheBody)
		bucket := newTestCacheBucket(t)
		setTestCacheBucket(t, bucket)

		first, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Equal(t, testCacheBody, string(first))
		assert.Equal(t, 1, cacheObjectCount(t, bucket))

		second, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Equal(t, testCacheBody, string(second), "a hit should return the exact bytes GitRestoreFile would have returned")
		assert.Equal(t, 1, cacheObjectCount(t, bucket))
	})

	t.Run("ContentNotMatchingBlobSHAIsTreatedAsMissAndOverwritten", func(t *testing.T) {
		dir := createTestGitRepo(t, testCacheFile, testCacheBody)
		bucket := newTestCacheBucket(t)
		setTestCacheBucket(t, bucket)

		blobSHA, err := gitRevParseBlobSHA(t.Context(), dir, testCacheFile)
		require.NoError(t, err)
		key := githubFileCacheKey(testCacheOwner, testCacheRepo, blobSHA)
		require.NoError(t, bucket.Put(t.Context(), key, strings.NewReader("tampered")))

		content, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Equal(t, testCacheBody, string(content))
		assert.Equal(t, testCacheBody, string(getCacheObject(t, bucket, key)))
	})

	t.Run("S3GetErrorFallsBackToGitRestoreFile", func(t *testing.T) {
		dir := createTestGitRepo(t, testCacheFile, testCacheBody)
		setTestCacheBucket(t, errorBucket{
			Bucket: newTestCacheBucket(t),
			getErr: func() error { return assert.AnError },
		})

		content, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Equal(t, testCacheBody, string(content))
	})

	t.Run("S3PutErrorStillReturnsContent", func(t *testing.T) {
		dir := createTestGitRepo(t, testCacheFile, testCacheBody)
		setTestCacheBucket(t, errorBucket{
			Bucket: newTestCacheBucket(t),
			putErr: func() error { return assert.AnError },
		})

		content, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Equal(t, testCacheBody, string(content))
	})

	t.Run("MissingFileReturnsFileNotFoundAndWritesNothing", func(t *testing.T) {
		dir := createTestGitRepo(t, "other.yml", testCacheBody)
		bucket := newTestCacheBucket(t)
		setTestCacheBucket(t, bucket)

		_, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		assert.True(t, IsFileNotFound(err))
		assert.Equal(t, 0, cacheObjectCount(t, bucket))
	})

	t.Run("UnconfiguredBucketSkipsCache", func(t *testing.T) {
		dir := createTestGitRepo(t, testCacheFile, testCacheBody)
		setTestCacheBucket(t, nil)

		content, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Equal(t, testCacheBody, string(content))
	})

	t.Run("EmptyFileRoundTripsAsHit", func(t *testing.T) {
		dir := createTestGitRepo(t, testCacheFile, "")
		bucket := newTestCacheBucket(t)
		setTestCacheBucket(t, bucket)

		first, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Empty(t, first)

		blobSHA, err := gitRevParseBlobSHA(t.Context(), dir, testCacheFile)
		require.NoError(t, err)
		key := githubFileCacheKey(testCacheOwner, testCacheRepo, blobSHA)
		assert.Equal(t, "", string(getCacheObject(t, bucket, key)))

		second, err := GetGitHubFileFromGit(t.Context(), testCacheOwner, testCacheRepo, "main", testCacheFile, dir)
		require.NoError(t, err)
		assert.Empty(t, second)
		assert.Equal(t, 1, cacheObjectCount(t, bucket))
	})
}

func TestVerifyGitBlobContent(t *testing.T) {
	blobSHA, err := gitRevParseBlobSHA(t.Context(), createTestGitRepo(t, testCacheFile, testCacheBody), testCacheFile)
	require.NoError(t, err)

	assert.True(t, verifyGitBlobContent([]byte(testCacheBody), blobSHA))
	assert.False(t, verifyGitBlobContent([]byte("tampered"), blobSHA))
}

func TestValidGitHubFileCacheComponents(t *testing.T) {
	assert.True(t, validGitHubFileCacheComponents("evergreen-ci", "sample"))
	assert.False(t, validGitHubFileCacheComponents(".", "sample"))
	assert.False(t, validGitHubFileCacheComponents("evergreen-ci", ".."))
	assert.False(t, validGitHubFileCacheComponents("evergreen ci", "sample"))
}

// errorBucket is a pail.Bucket whose Get and Put fail with the configured
// errors. A nil error function delegates to the underlying bucket.
type errorBucket struct {
	pail.Bucket
	getErr func() error
	putErr func() error
}

func (b errorBucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if b.getErr != nil {
		return nil, b.getErr()
	}
	return b.Bucket.Get(ctx, key)
}

func (b errorBucket) Put(ctx context.Context, key string, r io.Reader) error {
	if b.putErr != nil {
		return b.putErr()
	}
	return b.Bucket.Put(ctx, key, r)
}
