package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTarGzUnpackParseParams(t *testing.T) {

	Convey("With a targz unpack command", t, func() {

		var cmd *tarballExtract

		Convey("when parsing params into the command", func() {

			cmd = &tarballExtract{}

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

	Convey("With a targz unpack command", t, func() {
		testDataDir := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "archive")

		Convey("when unpacking an archive", func() {
			Convey("the archive's contents should be expanded into the"+
				" specified target directory", func() {

				target := filepath.Join(testDataDir, "target.tgz")
				output := filepath.Join(testDataDir, "output")

				testutil.HandleTestingErr(os.RemoveAll(target), t,
					"Error removing tgz file")
				testutil.HandleTestingErr(os.RemoveAll(output), t,
					"Error removing output directory")

				// create the output directory
				testutil.HandleTestingErr(os.MkdirAll(output, 0755), t,
					"Error creating output directory")

				// use the tar gz pack command to create a tarball
				tarPackCmd := &tarballCreate{}
				tarPackParams := map[string]interface{}{
					"target":        target,
					"source_dir":    testDataDir,
					"include":       []string{"targz_me/dir1/**"},
					"exclude_files": []string{},
				}

				So(tarPackCmd.ParseParams(tarPackParams), ShouldBeNil)
				numFound, err := tarPackCmd.makeArchive(ctx, logger.Task())
				So(err, ShouldBeNil)
				So(numFound, ShouldEqual, 2)

				// make sure it was built
				exists, err := util.FileExists(target)
				testutil.HandleTestingErr(err, t, "Error checking for file"+
					" existence")
				So(exists, ShouldBeTrue)

				// now, use a tar gz unpacking command to untar the tarball
				tarUnpackCmd := &tarballExtract{}
				tarUnpackParams := map[string]interface{}{
					"source":   target,
					"dest_dir": output,
				}

				So(tarUnpackCmd.ParseParams(tarUnpackParams), ShouldBeNil)
				So(tarUnpackCmd.unpackArchive(ctx), ShouldBeNil)

				// make sure the tarball was unpacked successfully
				exists, err = util.FileExists(
					filepath.Join(output, "targz_me/dir1/dir2/test.pdb"))
				testutil.HandleTestingErr(err, t, "Error checking file existence")
				So(exists, ShouldBeTrue)
				exists, err = util.FileExists(
					filepath.Join(output, "targz_me/dir1/dir2/testfile.txt"))
				testutil.HandleTestingErr(err, t, "Error checking file existence")
				So(exists, ShouldBeTrue)

			})

		})
	})
}
