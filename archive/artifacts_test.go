package archive

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

var logger *slogger.Logger

func init() {
	logger = &slogger.Logger{
		Prefix:    "test",
		Appenders: []slogger.Appender{slogger.StdOutAppender()},
	}
}

func TestArchiveExtract(t *testing.T) {
	Convey("After extracting a tarball", t, func() {

		//Remove the test output dir, in case it was left over from prior test
		err := os.RemoveAll("testdata/artifacts_test")
		testutil.HandleTestingErr(err, t, "Couldn't remove test dir")

		f, gz, tarReader, err := TarGzReader("testdata/artifacts.tar.gz")
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		defer f.Close()
		defer gz.Close()

		err = Extract(tarReader, "testdata/artifacts_test")
		So(err, ShouldBeNil)

		Convey("extracted data should match the archive contents", func() {
			f, err := os.Open("testdata/artifacts_test/artifacts/dir1/dir2/testfile.txt")
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
		err := os.RemoveAll("testdata/artifacts_out.tar.gz")
		testutil.HandleTestingErr(err, t, "Couldn't delete test tarball")

		f, gz, tarWriter, err := TarGzWriter("testdata/artifacts_out.tar.gz")
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		defer f.Close()
		defer gz.Close()
		defer tarWriter.Close()
		includes := []string{"artifacts/dir1/**"}
		excludes := []string{"*.pdb"}
		_, err = BuildArchive(tarWriter, "testdata/artifacts_in", includes, excludes, logger)
		So(err, ShouldBeNil)
	})
}

func TestArchiveRoundTrip(t *testing.T) {
	Convey("After building archive with include/exclude filters", t, func() {
		err := os.RemoveAll("testdata/artifacts_out.tar.gz")
		testutil.HandleTestingErr(err, t, "Couldn't remove test tarball")

		err = os.RemoveAll("testdata/artifacts_out")
		testutil.HandleTestingErr(err, t, "Couldn't remove test tarball")

		f, gz, tarWriter, err := TarGzWriter("testdata/artifacts_out.tar.gz")
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		includes := []string{"dir1/**"}
		excludes := []string{"*.pdb"}
		var found int
		found, err = BuildArchive(tarWriter, "testdata/artifacts_in", includes, excludes, logger)
		So(err, ShouldBeNil)
		So(found, ShouldEqual, 2)
		tarWriter.Close()
		gz.Close()
		f.Close()

		f2, gz2, tarReader, err := TarGzReader("testdata/artifacts_out.tar.gz")
		testutil.HandleTestingErr(err, t, "Couldn't open test tarball")
		err = Extract(tarReader, "testdata/artifacts_out")
		defer f2.Close()
		defer gz2.Close()
		So(err, ShouldBeNil)
		exists, err := util.FileExists("testdata/artifacts_out")
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)
		exists, err = util.FileExists("testdata/artifacts_out/dir1/dir2/test.pdb")
		So(err, ShouldBeNil)
		So(exists, ShouldBeFalse)

		// Dereference symlinks
		exists, err = util.FileExists("testdata/artifacts_out/dir1/dir2/my_symlink.txt")
		So(err, ShouldBeNil)
		So(exists, ShouldBeTrue)
		contents, err := ioutil.ReadFile("testdata/artifacts_out/dir1/dir2/my_symlink.txt")
		So(err, ShouldBeNil)
		So(string(contents), ShouldEqual, "Hello, World\n")
	})
}
