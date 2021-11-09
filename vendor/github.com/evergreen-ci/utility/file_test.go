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

func TestFileListBuilder(t *testing.T) {
	t.Run("DoesNotPanicForMissingRoot", func(t *testing.T) {
		assert.NotPanics(t, func() {
			dir := "this/path/does/not/exist"
			m, err := NewGitignoreFileMatcher(dir, "*")
			require.NoError(t, err)
			b := FileListBuilder{
				WorkingDir: dir,
				Include:    m,
			}
			files, err := b.Build()
			require.Error(t, err)
			assert.Empty(t, files)
		})
	})

	tmpDir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpDir))
	}()

	fileNames := []string{
		"testFile1",
		"testFile2",
		"testFile1.go",
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

	var allFiles []string

	for _, fileName := range fileNames {
		f, err := os.Create(filepath.Join(tmpDir, fileName))
		require.NoError(t, err, "error creating test file")
		require.NoError(t, f.Close(), "error closing test file")
		allFiles = append(allFiles, fileName)
	}

	for _, dirName := range dirNames {
		err := os.Mkdir(filepath.Join(tmpDir, dirName), 0777)
		require.NoError(t, err, "error creating test directory")
		for _, fileName := range fileNames {
			path := filepath.Join(tmpDir, dirName, fileName)
			f, err := os.Create(path)
			require.NoError(t, err, "error creating test file")
			require.NoError(t, f.Close(), "error closing test file")
			allFiles = append(allFiles, filepath.Join(dirName, fileName))
		}
	}

	t.Run("GitignoreFileBuilder", func(t *testing.T) {
		for testName, testCase := range map[string]struct {
			includeExprs []string
			excludeExprs []string
			expected     []string
		}{
			"MatchesOneFileName": {
				includeExprs: []string{"testFile1"},
				expected: []string{
					"testFile1",
					filepath.Join("dir1", "testFile1"),
					filepath.Join("dir2", "testFile1"),
				},
			},
			"MatchesOneFileWhenSubdirectoriesExcluded": {
				includeExprs: []string{"testFile1"},
				excludeExprs: []string{"/*/testFile1"},
				expected:     []string{"testFile1"},
			},
			"MatchesAllFilesWithPrefix": {
				includeExprs: []string{"/testFile*"},
				expected: []string{
					"testFile1",
					"testFile2",
					"testFile1.go",
					"testFile2.go",
					"testFile3.yml",
				},
			},
			"MatchesAllFilesWithPrefixExcludingSuffix": {
				includeExprs: []string{"/testFile*"},
				excludeExprs: []string{"/*.go"},
				expected: []string{
					"testFile1",
					"testFile2",
					"testFile3.yml",
				},
			},
			"MatchesAllFilesWithSuffix": {
				includeExprs: []string{"/*.go"},
				expected: []string{
					"testFile1.go",
					"testFile2.go",
					"built.go",
				},
			},
			"MatchesAllFilesWithSuffixExcludingPrefix": {
				includeExprs: []string{"/*.go"},
				excludeExprs: []string{"/built*"},
				expected: []string{
					"testFile1.go",
					"testFile2.go",
				},
			},
			"MatchesAllFilesInSubdirectoryWithSuffix": {
				includeExprs: []string{"/dir1/*.go"},
				expected: []string{
					filepath.Join("dir1", "testFile1.go"),
					filepath.Join("dir1", "testFile2.go"),
					filepath.Join("dir1", "built.go"),
				},
			},
			"MatchesAllFilesInWildcardSubdirectoryWithSuffix": {
				includeExprs: []string{"/*/*.go"},
				expected: []string{
					filepath.Join("dir1", "testFile1.go"),
					filepath.Join("dir1", "testFile2.go"),
					filepath.Join("dir1", "built.go"),
					filepath.Join("dir2", "testFile1.go"),
					filepath.Join("dir2", "testFile2.go"),
					filepath.Join("dir2", "built.go"),
				},
			},
			"MatchesAllFilesInWildcardSubdirectoryWithSuffixExcludingPrefix": {
				includeExprs: []string{"/*/*.go"},
				excludeExprs: []string{"/*/built*"},
				expected: []string{
					filepath.Join("dir1", "testFile1.go"),
					filepath.Join("dir1", "testFile2.go"),
					filepath.Join("dir2", "testFile1.go"),
					filepath.Join("dir2", "testFile2.go"),
				},
			},
		} {
			t.Run(testName, func(t *testing.T) {
				include, err := NewGitignoreFileMatcher(tmpDir, testCase.includeExprs...)
				require.NoError(t, err)
				var exclude FileMatcher
				if len(testCase.excludeExprs) != 0 {
					exclude, err = NewGitignoreFileMatcher(tmpDir, testCase.excludeExprs...)
					require.NoError(t, err)
				}
				b := FileListBuilder{
					WorkingDir: tmpDir,
					Include:    include,
					Exclude:    exclude,
				}

				files, err := b.Build()
				require.NoError(t, err)
				assert.ElementsMatch(t, testCase.expected, files)
			})
		}
	})

	t.Run("NewFileListBuilder", func(t *testing.T) {
		t.Run("MatchesAllFiles", func(t *testing.T) {
			b := NewFileListBuilder(tmpDir)
			files, err := b.Build()
			require.NoError(t, err)
			assert.ElementsMatch(t, allFiles, files)
		})
	})
	t.Run("AlwaysMatch", func(t *testing.T) {
		t.Run("IncludesAllFilesAndDirectories", func(t *testing.T) {
			b := NewFileListBuilder(tmpDir)
			b.Include = AlwaysMatch{}
			files, err := b.Build()
			require.NoError(t, err)
			allFilesAndDirs := append(allFiles, dirNames...)
			allFilesAndDirs = append(allFilesAndDirs, "")
			assert.ElementsMatch(t, allFilesAndDirs, files)
		})
		t.Run("ExcludesAllFilesAndDirectories", func(t *testing.T) {
			b := NewFileListBuilder(tmpDir)
			b.Exclude = AlwaysMatch{}
			files, err := b.Build()
			require.NoError(t, err)
			assert.Empty(t, files)
		})
	})

	t.Run("FileAlwaysMatch", func(t *testing.T) {
		t.Run("IncludesAllFiles", func(t *testing.T) {
			b := FileListBuilder{
				Include:    FileAlwaysMatch{},
				WorkingDir: tmpDir,
			}
			files, err := b.Build()
			require.NoError(t, err)
			assert.ElementsMatch(t, allFiles, files)
		})
		t.Run("ExcludesAllFiles", func(t *testing.T) {
			b := FileListBuilder{
				Include:    AlwaysMatch{},
				Exclude:    FileAlwaysMatch{},
				WorkingDir: tmpDir,
			}
			files, err := b.Build()
			require.NoError(t, err)
			assert.ElementsMatch(t, append(dirNames, ""), files)
		})
	})

	t.Run("NeverMatch", func(t *testing.T) {
		t.Run("IncludesNoFiles", func(t *testing.T) {
			b := FileListBuilder{
				Include:    NeverMatch{},
				WorkingDir: tmpDir,
			}
			files, err := b.Build()
			require.NoError(t, err)
			assert.Empty(t, files)
		})
		t.Run("ExcludesNoFiles", func(t *testing.T) {
			b := FileListBuilder{
				Include:    FileAlwaysMatch{},
				Exclude:    NeverMatch{},
				WorkingDir: tmpDir,
			}
			files, err := b.Build()
			require.NoError(t, err)
			assert.ElementsMatch(t, allFiles, files)
		})
	})
}
