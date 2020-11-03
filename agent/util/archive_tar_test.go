package util

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/logging"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	reporting.QuietMode()
}

func getDirectoryOfFile() string {
	// returns the directory of the file of the calling
	// function. duplicated from testutil to avoid a cycle.
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

func TestArchiveExtract(t *testing.T) {
	Convey("After extracting a tarball", t, func() {
		testDir := getDirectoryOfFile()
		//Remove the test output dir, in case it was left over from prior test
		outputDir := filepath.Join(testDir, "testdata", "artifacts_test")
		err := os.RemoveAll(outputDir)
		require.NoError(t, err, "Couldn't remove test dir")
		defer func() {
			assert.NoError(t, os.RemoveAll(outputDir))
		}()

		f, gz, tarReader, err := TarGzReader(filepath.Join(testDir, "testdata", "artifacts.tar.gz"))
		require.NoError(t, err, "Couldn't open test tarball")
		defer f.Close()
		defer gz.Close()

		err = extractTarArchive(context.Background(), tarReader, filepath.Join(testDir, "testdata", "artifacts_test"), []string{})
		So(err, ShouldBeNil)

		Convey("extracted data should match the archive contents", func() {
			f, err := os.Open(filepath.Join(testDir, "testdata", "artifacts_test", "artifacts", "dir1", "dir2", "testfile.txt"))
			So(err, ShouldBeNil)
			defer f.Close()
			data, err := ioutil.ReadAll(f)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "test\n")
		})
	})
}

func TestMakeArchive(t *testing.T) {
	Convey("Making an archive should not return an error", t, func() {
		testDir := getDirectoryOfFile()
		logger := logging.NewGrip("test.archive")

		outputDir := filepath.Join(testDir, "testdata", "artifacts_out.tar.gz")
		err := os.RemoveAll(outputDir)
		require.NoError(t, err, "Couldn't delete test tarball")
		defer func() {
			assert.NoError(t, os.RemoveAll(outputDir))
		}()

		f, gz, tarWriter, err := TarGzWriter(outputDir)
		require.NoError(t, err, "Couldn't open test tarball")
		defer f.Close()
		defer gz.Close()
		defer tarWriter.Close()
		includes := []string{"artifacts/dir1/**"}
		excludes := []string{"*.pdb"}
		_, err = BuildArchive(context.Background(), tarWriter, filepath.Join(testDir, "testdata", "artifacts_in"), includes, excludes, logger)
		So(err, ShouldBeNil)
	})
}

func TestArchiveRoundTrip(t *testing.T) {
	Convey("After building archive with include/exclude filters", t, func() {
		testDir := getDirectoryOfFile()
		logger := logging.NewGrip("test.archive")

		outputFile := filepath.Join(testDir, "testdata", "artifacts_out.tar.gz")
		err := os.RemoveAll(outputFile)
		require.NoError(t, err, "Couldn't remove test tarball")
		defer func() {
			assert.NoError(t, os.RemoveAll(outputFile))
		}()

		err = os.RemoveAll(outputFile)
		require.NoError(t, err, "Couldn't remove test tarball")

		f, gz, tarWriter, err := TarGzWriter(outputFile)
		require.NoError(t, err, "Couldn't open test tarball")
		includes := []string{"dir1/**"}
		excludes := []string{"*.pdb"}
		var found int
		found, err = BuildArchive(context.Background(), tarWriter, filepath.Join(testDir, "testdata", "artifacts_in"), includes, excludes, logger)
		So(err, ShouldBeNil)
		So(found, ShouldEqual, 2)
		So(tarWriter.Close(), ShouldBeNil)
		So(gz.Close(), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		outputDir := filepath.Join(testDir, "testdata", "artifacts_out")
		defer func() {
			assert.NoError(t, os.RemoveAll(outputDir))
		}()
		f2, gz2, tarReader, err := TarGzReader(outputFile)
		require.NoError(t, err, "Couldn't open test tarball")
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
		contents, err := ioutil.ReadFile(filepath.Join(outputDir, "dir1", "dir2", "my_symlink.txt"))
		So(err, ShouldBeNil)
		So(strings.Trim(string(contents), "\r\n\t "), ShouldEqual, "Hello, World")
	})
}
