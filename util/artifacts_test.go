package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/logging"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
	"golang.org/x/net/context"
)

func init() {
	reporting.QuietMode()
}

func TestArchiveExtract(t *testing.T) {
	Convey("After extracting a tarball", t, func() {
		testDir := testutil.GetDirectoryOfFile()
		//Remove the test output dir, in case it was left over from prior test
		err := os.RemoveAll(filepath.Join(testDir, "testdata", "artifacts_test"))
		testutil.HandleTestingErr(err, t, "Couldn't remove test dir")

		f, gz, tarReader, err := TarGzReader(filepath.Join(testDir, "testdata", "artifacts.tar.gz"))
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		defer f.Close()
		defer gz.Close()

		err = Extract(context.Background(), tarReader, filepath.Join(testDir, "testdata", "artifacts_test"))
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
		testDir := testutil.GetDirectoryOfFile()
		logger := logging.NewGrip("test.archive")

		err := os.RemoveAll(filepath.Join(testDir, "testdata", "artifacts_out.tar.gz"))
		testutil.HandleTestingErr(err, t, "Couldn't delete test tarball")

		f, gz, tarWriter, err := TarGzWriter(filepath.Join(testDir, "testdata", "artifacts_out.tar.gz"))
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
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
		testDir := testutil.GetDirectoryOfFile()
		logger := logging.NewGrip("test.archive")

		err := os.RemoveAll(filepath.Join(testDir, "testdata", "artifacts_out.tar.gz"))
		testutil.HandleTestingErr(err, t, "Couldn't remove test tarball")

		err = os.RemoveAll(filepath.Join(testDir, "testdata", "artifacts_out"))
		testutil.HandleTestingErr(err, t, "Couldn't remove test tarball")

		f, gz, tarWriter, err := TarGzWriter(filepath.Join(testDir, "testdata", "artifacts_out.tar.gz"))
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		includes := []string{"dir1/**"}
		excludes := []string{"*.pdb"}
		var found int
		found, err = BuildArchive(context.Background(), tarWriter, filepath.Join(testDir, "testdata", "artifacts_in"), includes, excludes, logger)
		So(err, ShouldBeNil)
		So(found, ShouldEqual, 2)
		So(tarWriter.Close(), ShouldBeNil)
		So(gz.Close(), ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		f2, gz2, tarReader, err := TarGzReader(filepath.Join(testDir, "testdata", "artifacts_out.tar.gz"))
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		err = Extract(context.Background(), tarReader, filepath.Join(testDir, "testdata", "artifacts_out"))
		defer f2.Close()
		defer gz2.Close()
		So(err, ShouldBeNil)
		exists, err := FileExists(filepath.Join(testDir, "testdata", "artifacts_out"))
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)
		exists, err = FileExists(filepath.Join(testDir, "testdata", "artifacts_out", "dir1", "dir2", "test.pdb"))
		So(err, ShouldBeNil)
		So(exists, ShouldBeFalse)

		// Dereference symlinks
		exists, err = FileExists(filepath.Join(testDir, "testdata", "artifacts_out", "dir1", "dir2", "my_symlink.txt"))
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)
		contents, err := ioutil.ReadFile(filepath.Join(testDir, "testdata", "artifacts_out", "dir1", "dir2", "my_symlink.txt"))
		So(err, ShouldBeNil)
		So(strings.Trim(string(contents), "\r\n\t "), ShouldEqual, "Hello, World")
	})
}
