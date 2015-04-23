package archive_test

import (
	"10gen.com/mci/command"
	. "10gen.com/mci/plugin/builtin/archivePlugin"
	"10gen.com/mci/plugin/testutil"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

var (
	testDataDir = "testdata"
)

func TestTarGzPackParseParams(t *testing.T) {

	Convey("With a targz pack command", t, func() {

		var cmd *TarGzPackCommand

		Convey("when parsing params into the command", func() {

			cmd = &TarGzPackCommand{}

			Convey("a missing target should cause an error", func() {

				params := map[string]interface{}{
					"source_dir": "s",
					"include":    []string{"i"},
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)

			})

			Convey("a missing source_dir should cause an error", func() {

				params := map[string]interface{}{
					"target":  "t",
					"include": []string{"i"},
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)

			})

			Convey("an empty include field should cause an error", func() {

				params := map[string]interface{}{
					"target":     "t",
					"source_dir": "s",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)

			})

			Convey("a valid set of params should be parsed into the"+
				" corresponding fields of the targz pack command", func() {

				params := map[string]interface{}{
					"target":        "t",
					"source_dir":    "s",
					"include":       []string{"i", "j"},
					"exclude_files": []string{"e", "f"},
				}
				So(cmd.ParseParams(params), ShouldBeNil)

				So(cmd.Target, ShouldEqual, params["target"])
				So(cmd.SourceDir, ShouldEqual, params["source_dir"])
				So(cmd.Include, ShouldResemble, params["include"])
				So(cmd.ExcludeFiles, ShouldResemble, params["exclude_files"])

			})

		})
	})
}

func TestTarGzCommandBuildArchive(t *testing.T) {

	Convey("With a targz pack command", t, func() {

		var cmd *TarGzPackCommand

		Convey("when building an archive", func() {

			cmd = &TarGzPackCommand{}

			Convey("the correct files should be included and excluded", func() {

				target := filepath.Join(testDataDir, "target.tgz")
				outputDir := filepath.Join(testDataDir, "output")

				util.HandleTestingErr(os.RemoveAll(target), t,
					"Error removing tgz file")
				util.HandleTestingErr(os.RemoveAll(outputDir), t,
					"Error removing output dir")

				params := map[string]interface{}{
					"target":        target,
					"source_dir":    testDataDir,
					"include":       []string{"targz_me/dir1/**"},
					"exclude_files": []string{"*.pdb"},
				}

				So(cmd.ParseParams(params), ShouldBeNil)
				So(cmd.BuildArchive("", &testutil.MockPluginLogger{}), ShouldBeNil)

				exists, err := util.FileExists(target)
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)

				// untar the file
				So(os.MkdirAll(outputDir, 0755), ShouldBeNil)
				untarCmd := &command.LocalCommand{
					CmdString:        "tar xvf ../target.tgz",
					WorkingDirectory: outputDir,
					Stdout:           ioutil.Discard,
					Stderr:           ioutil.Discard,
				}
				So(untarCmd.Run(), ShouldBeNil)

				// make sure that the correct files were included
				exists, err = util.FileExists(
					filepath.Join(outputDir, "targz_me/dir1/dir2/testfile.txt"))
				So(err, ShouldBeNil)
				So(exists, ShouldBeTrue)

				exists, err = util.FileExists(
					filepath.Join(outputDir, "targz_me/dir1/dir2/test.pdb"),
				)
				So(err, ShouldBeNil)
				So(exists, ShouldBeFalse)

			})

		})
	})
}
