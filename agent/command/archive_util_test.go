package command

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/klauspost/pgzip"
	"github.com/mongodb/grip/logging"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindContentsToArchive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	thisDir := testutil.GetDirectoryOfFile()
	t.Run("FindsFiles", func(t *testing.T) {
		entries, err := os.ReadDir(thisDir)
		require.NoError(t, err)

		expectedFiles := map[string]bool{}
		expectedFileSize := 0
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".go") {
				expectedFiles[entry.Name()] = false
				info, err := entry.Info()
				require.NoError(t, err)
				expectedFileSize += int(info.Size())
			}
		}
		assert.NotEmpty(t, expectedFiles)

		foundFiles, totalSize, err := findContentsToArchive(ctx, thisDir, []string{"*.go"}, nil)
		require.NoError(t, err)
		assert.Equal(t, expectedFileSize, totalSize)
		assert.Equal(t, len(foundFiles), len(expectedFiles))

		for _, foundFile := range foundFiles {
			pathRelativeToDir, err := filepath.Rel(thisDir, foundFile.path)
			require.NoError(t, err)
			_, ok := expectedFiles[pathRelativeToDir]
			if assert.True(t, ok, "unexpected file '%s' found", pathRelativeToDir) {
				expectedFiles[pathRelativeToDir] = true
			}
		}

		for filename, found := range expectedFiles {
			assert.True(t, found, "expected file '%s' not found", filename)
		}
	})
	t.Run("DeduplicatesFiles", func(t *testing.T) {
		entries, err := os.ReadDir(thisDir)
		require.NoError(t, err)

		expectedFiles := map[string]bool{}
		expectedFileSize := 0
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".go") {
				expectedFiles[entry.Name()] = false
				info, err := entry.Info()
				require.NoError(t, err)
				expectedFileSize += int(info.Size())
			}
		}
		assert.NotEmpty(t, expectedFiles)

		foundFiles, totalSize, err := findContentsToArchive(ctx, thisDir, []string{"*.go", "*.go"}, nil)
		require.NoError(t, err)
		assert.Equal(t, expectedFileSize, totalSize)

		for _, foundFile := range foundFiles {
			pathRelativeToDir, err := filepath.Rel(thisDir, foundFile.path)
			require.NoError(t, err)
			found, ok := expectedFiles[pathRelativeToDir]
			if assert.True(t, ok, "unexpected file '%s' found", pathRelativeToDir) {
				expectedFiles[pathRelativeToDir] = true
			}
			assert.False(t, found, "found file '%s' multiple times", pathRelativeToDir)
		}

		for filename, found := range expectedFiles {
			assert.True(t, found, "expected file '%s' not found", filename)
		}
	})
}

// getDirectoryOfFile returns the directory of the file of the calling function.
func getDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

// tarGzReader returns a file, gzip reader, and tar reader for the given path.
// The tar reader wraps the gzip reader, which wraps the file.
func tarGzReader(path string) (f, gz io.ReadCloser, tarReader *tar.Reader, err error) {
	f, err = os.Open(path)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "opening file '%s'", path)
	}
	gz, err = pgzip.NewReader(f)
	if err != nil {
		defer f.Close()
		return nil, nil, nil, errors.Wrap(err, "initializing gzip reader")
	}
	tarReader = tar.NewReader(gz)
	return f, gz, tarReader, nil
}

func TestArchiveExtract(t *testing.T) {
	Convey("After extracting a tarball", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		testDir := getDirectoryOfFile()
		outputDir := t.TempDir()

		f, gz, tarReader, err := tarGzReader(filepath.Join(testDir, "testdata", "archive", "artifacts.tar.gz"))
		require.NoError(t, err)
		defer f.Close()
		defer gz.Close()

		err = extractTarballArchive(ctx, tarReader, outputDir, []string{})
		So(err, ShouldBeNil)

		Convey("extracted data should match the archive contents", func() {
			f, err := os.Open(filepath.Join(outputDir, "artifacts", "dir1", "dir2", "testfile.txt"))
			So(err, ShouldBeNil)
			defer f.Close()
			data, err := io.ReadAll(f)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "test\n")
		})
	})
}

func TestBuildArchive(t *testing.T) {
	Convey("Making an archive should not return an error", t, func() {
		testDir := getDirectoryOfFile()
		logger := logging.NewGrip("test.archive")

		for testCase, makeTarGzWriter := range map[string]func(outputFile string) (f io.WriteCloser, gz io.WriteCloser, tarWriter *tar.Writer, err error){
			"Gzip": func(outputFile string) (f io.WriteCloser, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
				return tarGzWriter(outputFile, false)
			},
			"ParallelGzip": func(outputFile string) (f io.WriteCloser, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
				return tarGzWriter(outputFile, true)
			},
		} {
			Convey(fmt.Sprintf("with %s implementation", testCase), func() {
				outputFile, err := os.CreateTemp("", "artifacts_test_out.tar.gz")
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, os.RemoveAll(outputFile.Name()))
				}()
				require.NoError(t, outputFile.Close())

				f, gz, tarWriter, err := makeTarGzWriter(outputFile.Name())
				require.NoError(t, err)
				defer f.Close()
				defer gz.Close()
				defer tarWriter.Close()
				includes := []string{"artifacts/dir1/**"}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				rootPath := filepath.Join(testDir, "testdata", "archive", "artifacts_in")
				pathsToAdd, _, err := findContentsToArchive(ctx, rootPath, includes, []string{})
				require.NoError(t, err)
				excludes := []string{"*.pdb"}
				_, err = buildArchive(ctx, tarWriter, rootPath, pathsToAdd, excludes, logger, false)
				So(err, ShouldBeNil)
			})
		}
	})
}

func TestBuildArchiveVerbose(t *testing.T) {
	testDir := getDirectoryOfFile()
	logger := logging.NewGrip("test.archive")

	outputFile, err := os.CreateTemp("", "artifacts_test_verbose.tar.gz")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(outputFile.Name()))
	}()
	require.NoError(t, outputFile.Close())

	f, gz, tarWriter, err := tarGzWriter(outputFile.Name(), false)
	require.NoError(t, err)
	defer f.Close()
	defer gz.Close()
	defer tarWriter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rootPath := filepath.Join(testDir, "testdata", "archive", "artifacts_in")
	pathsToAdd, _, err := findContentsToArchive(ctx, rootPath, []string{"dir1/**"}, []string{})
	require.NoError(t, err)

	numFiles, err := buildArchive(ctx, tarWriter, rootPath, pathsToAdd, []string{"*.pdb"}, logger, true)
	require.NoError(t, err)
	assert.Greater(t, numFiles, 0)
}

func TestBuildArchiveRoundTrip(t *testing.T) {
	Convey("After building archive with include/exclude filters", t, func() {
		testDir := getDirectoryOfFile()
		logger := logging.NewGrip("test.archive")

		for testCase, makeTarGzWriter := range map[string]func(outputFile string) (f io.WriteCloser, gz io.WriteCloser, tarWriter *tar.Writer, err error){
			"Gzip": func(outputFile string) (f io.WriteCloser, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
				return tarGzWriter(outputFile, false)
			},
			"ParallelGzip": func(outputFile string) (f io.WriteCloser, gz io.WriteCloser, tarWriter *tar.Writer, err error) {
				return tarGzWriter(outputFile, true)
			},
		} {
			Convey(fmt.Sprintf("with %s implementation", testCase), func() {
				outputFile, err := os.CreateTemp("", "artifacts_test_out.tar.gz")
				require.NoError(t, err)
				require.NoError(t, outputFile.Close())
				defer func() {
					assert.NoError(t, os.RemoveAll(outputFile.Name()))
				}()

				f, gz, tarWriter, err := makeTarGzWriter(outputFile.Name())
				require.NoError(t, err)
				includes := []string{"dir1/**"}
				excludes := []string{"*.pdb"}
				var found int
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				rootPath := filepath.Join(testDir, "testdata", "archive", "artifacts_in")
				pathsToAdd, _, err := findContentsToArchive(ctx, rootPath, includes, []string{})
				require.NoError(t, err)
				found, err = buildArchive(ctx, tarWriter, rootPath, pathsToAdd, excludes, logger, false)
				So(err, ShouldBeNil)
				So(found, ShouldEqual, 4)
				So(tarWriter.Close(), ShouldBeNil)
				So(gz.Close(), ShouldBeNil)
				So(f.Close(), ShouldBeNil)

				outputDir := t.TempDir()
				f2, gz2, tarReader, err := tarGzReader(outputFile.Name())
				require.NoError(t, err)
				err = extractTarballArchive(context.Background(), tarReader, outputDir, []string{})
				defer f2.Close()
				defer gz2.Close()
				So(err, ShouldBeNil)
				exists := utility.FileExists(outputDir)
				So(exists, ShouldBeTrue)
				exists = utility.FileExists(filepath.Join(outputDir, "dir1", "dir2", "test.pdb"))
				So(exists, ShouldBeFalse)

				// Dereference symlinks
				exists = utility.FileExists(filepath.Join(outputDir, "dir1", "dir2", "my_symlink.txt"))
				So(exists, ShouldBeTrue)
				contents, err := os.ReadFile(filepath.Join(outputDir, "dir1", "dir2", "my_symlink.txt"))
				So(err, ShouldBeNil)
				So(strings.Trim(string(contents), "\r\n\t "), ShouldEqual, "Hello, World")
			})
		}
	})
}

func TestFindArchiveContentsSymLink(t *testing.T) {
	testDir := getDirectoryOfFile()
	root := filepath.Join(testDir, "testdata", "archive", "symlink")

	// create a invalid.txt symlink that points outside of the root
	invalidLinkPath := filepath.Join(root, "invalid.txt")
	require.NoError(t, os.Symlink("/etc/passwd", invalidLinkPath))
	t.Cleanup(func() {
		require.NoError(t, os.Remove(invalidLinkPath))
	})

	t.Run("SymLinksStayUnresolved", func(t *testing.T) {
		files, size, err := findArchiveContents(t.Context(), root, []string{"*.txt"}, []string{})
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, "invalid.txt", filepath.Base(files[0].path))
		assert.Equal(t, "valid.txt", filepath.Base(files[1].path))
		assert.Equal(t, 5, size)
	})
}

func TestGlobPatternBehavior(t *testing.T) {
	testDir := getDirectoryOfFile()

	// dir1/dir2/my_symlink.txt
	// dir1/dir2/test.pdb
	// dir1/dir2/testfile.txt

	rootPath := filepath.Join(testDir, "testdata", "archive", "artifacts_in")

	// Test directory glob patterns with different behaviors
	testCases := map[string][]string{
		"dir1":    {"dir1"},                                                                                          // directory itself
		"dir1/":   {},                                                                                                // nothing (trailing slash)
		"dir1/*":  {"dir1/dir2"},                                                                                     // direct children only
		"dir1/**": {"dir1", "dir1/dir2", "dir1/dir2/my_symlink.txt", "dir1/dir2/test.pdb", "dir1/dir2/testfile.txt"}, // everything recursively
	}

	for pattern, expectedPaths := range testCases {
		t.Run(pattern, func(t *testing.T) {
			files, _, err := findContentsToArchive(t.Context(), rootPath, []string{pattern}, []string{})
			require.NoError(t, err)
			assert.Equal(t, len(expectedPaths), len(files), "pattern '%s' should find %d files", pattern, len(expectedPaths))

			var actualPaths []string
			for _, f := range files {
				rel, err := filepath.Rel(rootPath, f.path)
				require.NoError(t, err)
				actualPaths = append(actualPaths, filepath.ToSlash(rel))
			}
			assert.ElementsMatch(t, expectedPaths, actualPaths, "pattern '%s' should find expected paths", pattern)
		})
	}
}

func TestGlobPatternCombinations(t *testing.T) {
	testDir := getDirectoryOfFile()
	rootPath := filepath.Join(testDir, "testdata", "glob_combinations")

	// Test cases with combinations of include and exclude patterns
	testCases := []struct {
		name          string
		includes      []string
		excludes      []string
		expectedPaths []string
	}{
		{
			// Should recursively find all java files in the fixture folder
			name:     "AllJavaFiles",
			includes: []string{"./**.java"},
			excludes: []string{},
			expectedPaths: []string{
				"src/main.java",
				"src/util.java",
				"src/subdir/helper.java",
				"src/subdir/test.java",
			},
		},
		{
			// Should recursively find all json and md files in the fixture folder
			name:     "MultipleIncludes",
			includes: []string{"**.json", "**.md"},
			excludes: []string{},
			expectedPaths: []string{
				"config/local.json",
				"config/settings.json",
				"docs/guide.md",
			},
		},
		{
			// Should recursively find all files in the config directory, and include the directory itself
			name:     "SpecificDirectory",
			includes: []string{"config/**"},
			expectedPaths: []string{
				"config",
				"config/local.json",
				"config/settings.json",
			},
		},
		{
			// Should include every folder and file one level under src, but not recursively.
			name:     "DirectChildrenOnly",
			includes: []string{"src/*"},
			excludes: []string{},
			expectedPaths: []string{
				"src/main.java",
				"src/subdir",
				"src/util.java",
			},
		},
		{
			// Should include every folder under src recursively, and also just a specific file.
			name:     "SpecificFiles",
			includes: []string{"config/local.json", "src/**"},
			excludes: []string{},
			expectedPaths: []string{
				"config/local.json",
				"src",
				"src/main.java",
				"src/util.java",
				"src/subdir",
				"src/subdir/helper.java",
				"src/subdir/test.java",
			},
		},
		{
			// Should include both specified files only.
			name:     "SpecificFilesAndWildcard",
			includes: []string{"src/main.java", "src/util.java"},
			excludes: []string{},
			expectedPaths: []string{
				"src/main.java",
				"src/util.java",
			},
		},
		{
			// Tracking the legacy behavior, wildcards before the last slash don't return results due to how prefixes are matched.
			// This test exercises the code for "**" wildcard final path element.
			name:          "WildcardsBeforeEndDontWork",
			includes:      []string{"*/**"},
			excludes:      []string{},
			expectedPaths: []string{},
		},
		{
			// Tracking the legacy behavior, wildcards before the last slash don't return results due to how prefixes are matched.
			// This test exercises the code path without a "**" wildcard final path element.
			name:          "WildcardsBeforeEndDontWorkWithSpecificEnding",
			includes:      []string{"*/main.java"},
			excludes:      []string{},
			expectedPaths: []string{},
		},
		{
			// Tracking the legacy behavior, wildcards before the last slash don't return results due to how prefixes are matched.
			// This test exercises the code path with a "**.java" wildcard final path element, which is distinct from bare '**'.
			name:          "WildcardsBeforeEndDontWorkWithDoubleWildcardEnding",
			includes:      []string{"*/**.java"},
			excludes:      []string{},
			expectedPaths: []string{},
		},
		{
			// Tracking the legacy behavior, wildcards before the last slash don't return results due to how prefixes are matched.
			// This test exercises the code path with no double wildcard but a wildcard in the final path element.
			name:          "WildcardsBeforeEndDontWorkWithSingleWildcardEnding",
			includes:      []string{"*/*.java"},
			excludes:      []string{},
			expectedPaths: []string{},
		},
		{
			// This tracks the legacy behavior that never returns anything if the include pattern ends in a slash.
			name:          "EndingInSlashReturnsNone",
			includes:      []string{"src/"},
			excludes:      []string{},
			expectedPaths: []string{},
		},
		{
			// Tests using infrequently used "?" wildcard, which matches a single character.
			// This particular one should match a single file.
			name:     "TestQuestionMarkWildcard",
			includes: []string{"config/loc??.json"},
			excludes: []string{},
			expectedPaths: []string{
				"config/local.json",
			},
		},
		//
		// Test Exclusions. Note that we have to include the rootPath because the util function
		// currently requires the exclude math to match the full joined path, not just the suffix
		// to the root path provided.
		//
		{
			// The '*.java' doesn't actually exclude anything because filepath.Match is used on the whole path
			// This is preserving legacy behavior.
			name:     "FailToExcludeEverythingSingleStar",
			includes: []string{"**.java"},
			excludes: []string{"*.java"},
			expectedPaths: []string{
				"src/main.java",
				"src/util.java",
				"src/subdir/helper.java",
				"src/subdir/test.java",
			},
		},
		{
			// The '**' doesn't actually exclude anything because filepath.Match doesn't handle double wildcards.
			name:     "FailToExcludeEverythingDoubleStar",
			includes: []string{"**.java"},
			excludes: []string{"**"},
			expectedPaths: []string{
				"src/main.java",
				"src/util.java",
				"src/subdir/helper.java",
				"src/subdir/test.java",
			},
		},
		{
			// This excludes the only file explicitly included, resulting in no matches.
			name:          "SpecificFileAndExclude",
			includes:      []string{"src/main.java"},
			excludes:      []string{filepath.Join(rootPath, "src/main.java")},
			expectedPaths: []string{},
		},
		{
			// This excludes a wiledcard that matches the file explicitly included, resulting in no matches.
			name:          "SpecificFileAndExcludeWildcard",
			includes:      []string{"src/main.java"},
			excludes:      []string{filepath.Join(rootPath, "src/*.java")},
			expectedPaths: []string{},
		},
		{
			// This excludes one of the java files marked under src, leaving only the remaining one.
			name:     "SingleWildcardAndExcludeSpecific",
			includes: []string{"src/*.java"},
			excludes: []string{filepath.Join(rootPath, "src/main.java")},
			expectedPaths: []string{
				"src/util.java",
			},
		},
		{
			// This excludes one of the java files marked under src, leaving the remaining ones obtained recursively
			name:     "DoubleWildcardAndExcludeSpecific",
			includes: []string{"src/**.java"},
			excludes: []string{filepath.Join(rootPath, "src/main.java")},
			expectedPaths: []string{
				"src/subdir/helper.java",
				"src/subdir/test.java",
				"src/util.java",
			},
		},
		{
			// This excludes all the direct subfiles of src, leaving only the deeper recursive files.
			name:     "DoubleWildcardAndExcludeWildcard",
			includes: []string{"src/**.java"},
			excludes: []string{filepath.Join(rootPath, "src/*.java")},
			expectedPaths: []string{
				"src/subdir/helper.java",
				"src/subdir/test.java",
			},
		},
		{
			// Combines the exclusion of direct subfiles of src, and one specific subdirectory file.
			name:     "DoubleWildcardAndExcludeWildcardAndSpecificFile",
			includes: []string{"src/**.java"},
			excludes: []string{filepath.Join(rootPath, "src/*.java"), filepath.Join(rootPath, "src/subdir/test.java")},
			expectedPaths: []string{
				"src/subdir/helper.java",
			},
		},
		{
			// Combines the exclusion of direct subfiles of src, and one specific subdirectory file, but uses "**"
			// which includes returning the directories themselves as well.
			name:     "DoubleWildcardAllAndExcludeWildcardAndSpecificFile",
			includes: []string{"src/**"},
			excludes: []string{filepath.Join(rootPath, "src/*.java"), filepath.Join(rootPath, "src/subdir/test.java")},
			expectedPaths: []string{
				"src",
				"src/subdir",
				"src/subdir/helper.java",
			},
		},
		{
			// Tests that excluding a directory using a trailing slash works, despite
			// this not working for includes. This preserves legacy behavior.
			name:     "ExcludeDirectoriesFromDoubleWildcard",
			includes: []string{"config/**"},
			excludes: []string{filepath.Join(rootPath, "config/")},
			expectedPaths: []string{
				"config/local.json",
				"config/settings.json",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files, _, err := findContentsToArchive(t.Context(), rootPath, tc.includes, tc.excludes)
			require.NoError(t, err)

			var actualPaths []string
			for _, f := range files {
				rel, err := filepath.Rel(rootPath, f.path)
				require.NoError(t, err)
				actualPaths = append(actualPaths, filepath.ToSlash(rel))
			}

			assert.ElementsMatch(t, tc.expectedPaths, actualPaths,
				"includes: %v, excludes: %v should find expected paths", tc.includes, tc.excludes)
		})
	}
}
