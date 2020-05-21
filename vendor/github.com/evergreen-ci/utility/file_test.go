package utility

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFile(t *testing.T) {
	t.Run("FileExistence", func(t *testing.T) {
		defer func() { assert.NoError(t, os.RemoveAll("foo")) }()
		require.False(t, FileExists("foo"))
		require.NoError(t, WriteFile("foo", "hello world"))
		require.True(t, FileExists("foo"))
	})
	t.Run("WriteFile", func(t *testing.T) {
		defer func() { assert.NoError(t, os.RemoveAll("foo")) }()
		require.Error(t, WriteFile("/usr/local", "hi"))
		require.NoError(t, WriteFile("foo", "hello world"))
	})
}

func TestBuildFileList(t *testing.T) {
	t.Run("DoesNotPanicForMissingRoot", func(t *testing.T) {
		assert.NotPanics(t, func() {
			list, err := BuildFileList("this/path/does/not/exist", "*")
			require.Error(t, err)
			assert.Empty(t, list)
		})
	})

	wd, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err, "error getting working directory")
	defer func() {
		assert.NoError(t, os.RemoveAll(wd))
	}()

	fnames := []string{
		"testFile1",
		"testFile2",
		"testFile.go",
		"testFile2.go",
		"testFile3.yml",
		"built.yml",
		"built.go",
		"built.cpp",
	}
	dirNames := []string{
		"dir1",
		"dir2",
	}
	// create all the files in the current directory
	for _, fname := range fnames {
		f, err := os.Create(filepath.Join(wd, fname))
		require.NoError(t, err, "error creating test file")
		require.NoError(t, f.Close(), "error closing test file")
	}

	// create all the files in the sub directories
	for _, dirName := range dirNames {
		err := os.Mkdir(filepath.Join(wd, dirName), 0777)
		require.NoError(t, err, "error creating test directory")
		for _, fname := range fnames {
			path := filepath.Join(wd, dirName, fname)
			f, err := os.Create(path)
			require.NoError(t, err, "error creating test file")
			require.NoError(t, f.Close(), "error closing test file")
		}
	}
	defer func() {
		require.NoError(t, os.RemoveAll(wd), "error removing test directory")
	}()

	t.Run("MatchesOneFileName", func(t *testing.T) {
		files, err := BuildFileList(wd, fnames[0])
		require.NoError(t, err)
		assert.Contains(t, files[0], fnames[0])
		for i := 1; i < len(fnames); i++ {
			assert.NotContains(t, files, fnames[i])
		}
	})
	t.Run("MatchesAllFilesWithPrefix", func(t *testing.T) {
		files, err := BuildFileList(wd, "/testFile*")
		require.NoError(t, err)
		for i, fname := range fnames {
			if i <= 4 {
				assert.Contains(t, files, fname)
			} else {
				assert.NotContains(t, files, fname)
			}
		}
	})
	t.Run("MatchesAllFilesWithSuffix", func(t *testing.T) {
		files, err := BuildFileList(wd, "/*.go")
		require.NoError(t, err)
		for i, fname := range fnames {
			if i == 2 || i == 3 || i == 6 {
				assert.Contains(t, files, fname)
			} else {
				assert.NotContains(t, files, fname)
			}
		}
	})
	t.Run("MatchesAllFilesInSubdirectoryWithSuffix", func(t *testing.T) {
		files, err := BuildFileList(wd, "/dir1/*.go")
		require.NoError(t, err)
		for i, fname := range fnames {
			path := filepath.Join("dir1", fname)
			if i == 2 || i == 3 || i == 6 {
				assert.Contains(t, files, path)
				assert.NotContains(t, files, fname)
			} else {
				assert.NotContains(t, files, path)
				assert.NotContains(t, files, fname)
			}
		}
	})
	t.Run("MatchesAllFilesInWildcardSubdirectoryWithSuffix", func(t *testing.T) {
		files, err := BuildFileList(wd, "/*/*.go")
		require.NoError(t, err)
		for i, fname := range fnames {
			for _, dirName := range dirNames {
				path := filepath.Join(dirName, fname)
				if i == 2 || i == 3 || i == 6 {
					assert.Contains(t, files, path)
					assert.NotContains(t, files, fname)
				} else {
					assert.NotContains(t, files, path)
					assert.NotContains(t, files, fname)
				}
			}
		}
	})
}
