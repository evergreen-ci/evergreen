package command

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCacheKeyFile(t *testing.T, dir, name, contents string) string {
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(contents), 0644))
	return path
}

func TestComputeCacheKey(t *testing.T) {
	dir := t.TempDir()
	fileA := writeCacheKeyFile(t, dir, "a.txt", "alpha")
	fileB := writeCacheKeyFile(t, dir, "b.txt", "beta")

	t.Run("IdenticalInputsProduceIdenticalKeys", func(t *testing.T) {
		first, err := computeCacheKey([]string{fileA, fileB}, []string{"linux", "go1.21"}, false)
		require.NoError(t, err)
		second, err := computeCacheKey([]string{fileA, fileB}, []string{"linux", "go1.21"}, false)
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})

	t.Run("ReorderingExpansionsChangesKey", func(t *testing.T) {
		first, err := computeCacheKey(nil, []string{"linux", "go1.21"}, false)
		require.NoError(t, err)
		second, err := computeCacheKey(nil, []string{"go1.21", "linux"}, false)
		require.NoError(t, err)
		assert.NotEqual(t, first, second)
	})

	t.Run("ReorderingKeyFilesChangesKey", func(t *testing.T) {
		first, err := computeCacheKey([]string{fileA, fileB}, []string{"linux"}, false)
		require.NoError(t, err)
		second, err := computeCacheKey([]string{fileB, fileA}, []string{"linux"}, false)
		require.NoError(t, err)
		assert.NotEqual(t, first, second)
	})

	t.Run("DifferentFileContentsChangeKey", func(t *testing.T) {
		changed := writeCacheKeyFile(t, dir, "a.txt", "alpha-changed")
		first, err := computeCacheKey([]string{fileB}, []string{"linux"}, false)
		require.NoError(t, err)
		second, err := computeCacheKey([]string{changed}, []string{"linux"}, false)
		require.NoError(t, err)
		assert.NotEqual(t, first, second)
	})

	t.Run("NoKeyFilesIsValid", func(t *testing.T) {
		key, err := computeCacheKey(nil, []string{"linux"}, false)
		require.NoError(t, err)
		assert.NotEmpty(t, key)
	})

	t.Run("MissingKeyFileErrors", func(t *testing.T) {
		_, err := computeCacheKey([]string{filepath.Join(dir, "does-not-exist.txt")}, []string{"linux"}, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does-not-exist.txt")
	})

	t.Run("KeyFileContentDoesNotCollideWithExpansions", func(t *testing.T) {
		// A key file containing "a" plus expansion "b" must not hash the same as
		// expansions ["a", "b"], which a naive concatenation would conflate.
		fileWithA := writeCacheKeyFile(t, dir, "collide.txt", "a")
		withFile, err := computeCacheKey([]string{fileWithA}, []string{"b"}, false)
		require.NoError(t, err)
		withoutFile, err := computeCacheKey(nil, []string{"a", "b"}, false)
		require.NoError(t, err)
		assert.NotEqual(t, withFile, withoutFile)
	})

	t.Run("PreserveSymlinksChangesKey", func(t *testing.T) {
		// Folding preserve_symlinks into the key partitions symlink-aware caches
		// from older dereferenced ones with otherwise-identical inputs.
		without, err := computeCacheKey([]string{fileA}, []string{"linux"}, false)
		require.NoError(t, err)
		with, err := computeCacheKey([]string{fileA}, []string{"linux"}, true)
		require.NoError(t, err)
		assert.NotEqual(t, without, with)
	})
}

func TestCacheHitExpansionName(t *testing.T) {
	assert.Equal(t, "go_cache_hit", cacheHitExpansionName("go"))
	assert.Equal(t, "mise_and_go_cache_hit", cacheHitExpansionName("mise-and-go"))
}

func TestCacheArchiveRoundTrip(t *testing.T) {
	logger := logging.MakeGrip(send.MakeInternalLogger())
	ctx := t.Context()

	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "top.txt"), []byte("top"), 0644))
	nested := filepath.Join(srcDir, "deps")
	require.NoError(t, os.MkdirAll(nested, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "lib.txt"), []byte("library"), 0644))

	archivePath := filepath.Join(t.TempDir(), "cache.tgz")
	require.NoError(t, makeCacheArchive(ctx, srcDir, []string{"top.txt", "deps"}, archivePath, logger, false))

	destDir := t.TempDir()
	archive, err := os.Open(archivePath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, archive.Close()) })
	require.NoError(t, extractTarball(ctx, archive, destDir, nil, false))

	top, err := os.ReadFile(filepath.Join(destDir, "top.txt"))
	require.NoError(t, err)
	assert.Equal(t, "top", string(top))

	lib, err := os.ReadFile(filepath.Join(destDir, "deps", "lib.txt"))
	require.NoError(t, err)
	assert.Equal(t, "library", string(lib))
}

func TestCacheArchiveRoundTripPreservesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink preservation is not supported on Windows")
	}
	logger := logging.MakeGrip(send.MakeInternalLogger())
	ctx := t.Context()

	srcDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "real.txt"), []byte("payload"), 0644))
	require.NoError(t, os.Symlink("real.txt", filepath.Join(srcDir, "link.txt")))

	archivePath := filepath.Join(t.TempDir(), "cache.tgz")
	require.NoError(t, makeCacheArchive(ctx, srcDir, []string{"real.txt", "link.txt"}, archivePath, logger, true))

	destDir := t.TempDir()
	archive, err := os.Open(archivePath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, archive.Close()) })
	require.NoError(t, extractTarball(ctx, archive, destDir, nil, true))

	info, err := os.Lstat(filepath.Join(destDir, "link.txt"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "restored link.txt should still be a symlink")

	target, err := os.Readlink(filepath.Join(destDir, "link.txt"))
	require.NoError(t, err)
	assert.Equal(t, "real.txt", target)
}

func TestMakeCacheArchiveMissingPathErrors(t *testing.T) {
	logger := logging.MakeGrip(send.MakeInternalLogger())
	srcDir := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "cache.tgz")

	err := makeCacheArchive(t.Context(), srcDir, []string{"missing-dir"}, archivePath, logger, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-dir")
}
