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
	"github.com/mongodb/grip/logging"
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
		assert.NotZero(t, len(expectedFiles))

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
		assert.NotZero(t, len(expectedFiles))

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

func TestArchiveExtract(t *testing.T) {
	Convey("After extracting a tarball", t, func() {
		testDir := getDirectoryOfFile()
		outputDir := t.TempDir()

		f, gz, tarReader, err := tarGzReader(filepath.Join(testDir, "testdata", "archive", "artifacts.tar.gz"))
		require.NoError(t, err)
		defer f.Close()
		defer gz.Close()

		err = extractTarArchive(context.Background(), tarReader, outputDir, []string{})
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
				_, err = buildArchive(ctx, tarWriter, rootPath, pathsToAdd, excludes, logger)
				So(err, ShouldBeNil)
			})
		}
	})
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
				found, err = buildArchive(ctx, tarWriter, rootPath, pathsToAdd, excludes, logger)
				So(err, ShouldBeNil)
				So(found, ShouldEqual, 2)
				So(tarWriter.Close(), ShouldBeNil)
				So(gz.Close(), ShouldBeNil)
				So(f.Close(), ShouldBeNil)

				outputDir := t.TempDir()
				f2, gz2, tarReader, err := tarGzReader(outputFile.Name())
				require.NoError(t, err)
				err = extractTarArchive(context.Background(), tarReader, outputDir, []string{})
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
