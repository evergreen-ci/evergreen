package archive_test

import (
	. "github.com/evergreen-ci/evergreen/plugin/builtin/archive"
	"github.com/evergreen-ci/evergreen/plugin/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"path/filepath"
	"testing"
)

func TestTarGzUnpackParseParams(t *testing.T) {

	Convey("With a targz unpack command", t, func() {

		var cmd *TarGzUnpackCommand

		Convey("when parsing params into the command", func() {

			cmd = &TarGzUnpackCommand{}

			Convey("a missing source should cause an error", func() {

				params := map[string]interface{}{
					"dest_dir": "dest",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)

			})

			Convey("a missing dest_dir should cause an error", func() {

				params := map[string]interface{}{
					"source": "s",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)

			})

			Convey("a valid set of params should be parsed into the"+
				" corresponding fields of the targz pack command", func() {

				params := map[string]interface{}{
					"source":   "a.tgz",
					"dest_dir": "dest",
				}
				So(cmd.ParseParams(params), ShouldBeNil)

				So(cmd.Source, ShouldEqual, params["source"])
				So(cmd.DestDir, ShouldEqual, params["dest_dir"])

			})

		})
	})
}

func TestTarGzCommandUnpackArchive(t *testing.T) {

	Convey("With a targz unpack command", t, func() {

		var cmd *TarGzUnpackCommand

		Convey("when unpacking an archive", func() {

			cmd = &TarGzUnpackCommand{}

			Convey("the archive's contents should be expanded into the"+
				" specified target directory", func() {

				target := filepath.Join(testDataDir, "target.tgz")
				output := filepath.Join(testDataDir, "output")

				util.HandleTestingErr(os.RemoveAll(target), t,
					"Error removing tgz file")
				util.HandleTestingErr(os.RemoveAll(output), t,
					"Error removing output directory")

				// create the output directory
				util.HandleTestingErr(os.MkdirAll(output, 0755), t,
					"Error creating output directory")

				// use the tar gz pack command to create a tarball
				tarPackCmd := &TarGzPackCommand{}
				tarPackParams := map[string]interface{}{
					"target":        target,
					"source_dir":    testDataDir,
					"include":       []string{"targz_me/dir1/**"},
					"exclude_files": []string{},
				}

				So(tarPackCmd.ParseParams(tarPackParams), ShouldBeNil)
				So(tarPackCmd.BuildArchive("", &testutil.MockLogger{}), ShouldBeNil)

				// make sure it was built
				exists, err := util.FileExists(target)
				util.HandleTestingErr(err, t, "Error checking for file"+
					" existence")
				So(exists, ShouldBeTrue)

				// now, use a tar gz unpacking command to untar the tarball
				tarUnpackCmd := &TarGzUnpackCommand{}
				tarUnpackParams := map[string]interface{}{
					"source":   target,
					"dest_dir": output,
				}

				So(tarUnpackCmd.ParseParams(tarUnpackParams), ShouldBeNil)
				So(tarUnpackCmd.UnpackArchive(), ShouldBeNil)

				// make sure the tarball was unpacked successfully
				exists, err = util.FileExists(
					filepath.Join(output, "targz_me/dir1/dir2/test.pdb"))
				util.HandleTestingErr(err, t, "Error checking file existence")
				So(exists, ShouldBeTrue)
				exists, err = util.FileExists(
					filepath.Join(output, "targz_me/dir1/dir2/testfile.txt"))
				util.HandleTestingErr(err, t, "Error checking file existence")
				So(exists, ShouldBeTrue)

			})

		})
	})
}
