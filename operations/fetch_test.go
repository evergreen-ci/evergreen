package operations

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestClone(t *testing.T) {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestClone")

	type testCase struct {
		opts      cloneOptions
		isPassing bool
	}

	testCases := map[string]testCase{
		"SimpleHTTPS": {isPassing: true, opts: cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "main",
		}},
		"InvalidRepo": {isPassing: false, opts: cloneOptions{
			owner:      "evergreen-ci",
			repository: "foo",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "main",
		}},
		"InvalidRevision": {isPassing: false, opts: cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "9999999999999999999999999999999999999999",
			branch:     "main",
		}},
		"InvalidToken": {isPassing: false, opts: cloneOptions{
			owner:      "10gen",
			repository: "kernel-tools",
			revision:   "cabca3defc4b251c8a0be268969606717e01f906",
			branch:     "main",
			token:      "foo",
		}},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			runCloneTest(t, test.opts, test.isPassing)
		})
	}
}

func runCloneTest(t *testing.T, opts cloneOptions, pass bool) {
	opts.rootDir = t.TempDir()
	if !pass {
		assert.Error(t, clone(opts))
		return
	}
	assert.NoError(t, clone(opts))
}

func TestTruncateName(t *testing.T) {
	// Test with .tar in filename
	fileName := strings.Repeat("a", 300) + ".tar.gz"
	newName := truncateFilename(fileName)
	assert.NotEqual(t, newName, fileName)
	assert.Len(t, newName, 250)
	assert.Equal(t, strings.Repeat("a", 243)+".tar.gz", newName)

	// Test with 3 dots in filename
	fileName = strings.Repeat("a", 243) + "_v4.4.4.txt"
	newName = truncateFilename(fileName)
	assert.NotEqual(t, newName, fileName)
	assert.Len(t, newName, 250)
	assert.Equal(t, strings.Repeat("a", 243)+"_v4.txt", newName)

	// Test filename at max length
	fileName = strings.Repeat("a", 247) + ".js"
	newName = truncateFilename(fileName)
	assert.Equal(t, fileName, newName)

	// Test "extension" significantly longer than name
	fileName = "a." + strings.Repeat("b", 300)
	newName = truncateFilename(fileName)
	assert.Equal(t, fileName, newName)

	// Test no extension
	fileName = strings.Repeat("a", 300)
	newName = truncateFilename(fileName)
	assert.Len(t, newName, 250)
	assert.Equal(t, strings.Repeat("a", 250), newName)
}

func TestFileNameWithIndex(t *testing.T) {
	t.Run("JustFilename", func(t *testing.T) {
		assert.Equal(t, "file_(4).txt", fileNameWithIndex("file.txt", 5))
	})
	t.Run("DirectoryAndFilename", func(t *testing.T) {
		assert.Equal(t, filepath.Join("path", "to", "file_(4).txt"), fileNameWithIndex(filepath.Join("path", "to", "file.txt"), 5))
	})
	t.Run("FilenameWithoutExtensions", func(t *testing.T) {
		assert.Equal(t, "file_(4)", fileNameWithIndex("file", 5))
	})
	t.Run("DirectoryAndFilenameWithoutExtensions", func(t *testing.T) {
		assert.Equal(t, filepath.Join("path", "to", "file_(4)"), fileNameWithIndex(filepath.Join("path", "to", "file"), 5))
	})
	t.Run("DirectoryAndFilenameWithMultipleExtensions", func(t *testing.T) {
		assert.Equal(t, filepath.Join("path", "to", "file_(4).tar.gz"), fileNameWithIndex(filepath.Join("path", "to", "file.tar.gz"), 5))
	})
	t.Run("DirectoryWithPeriodsAndFilenameWithExtension", func(t *testing.T) {
		assert.Equal(t, filepath.Join("path.with.dots", "to", "file_(4).tar.gz"), fileNameWithIndex(filepath.Join("path.with.dots", "to", "file.tar.gz"), 5))
	})
}
