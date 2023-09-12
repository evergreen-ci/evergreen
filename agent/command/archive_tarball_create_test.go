package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/jasper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTarGzPackParseParams(t *testing.T) {
	Convey("With a targz pack command", t, func() {
		var cmd *tarballCreate

		Convey("when parsing params into the command", func() {

			cmd = &tarballCreate{}

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

func TestTarGzCommandMakeArchive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{Expansions: util.Expansions{}, Task: task.Task{}, Project: model.Project{}}
	logger, _ := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)

	Convey("With a targz pack command", t, func() {
		testDataDir := filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "archive")
		var cmd *tarballCreate

		Convey("when building an archive", func() {

			cmd = &tarballCreate{}

			Convey("the correct files should be included and excluded", func() {

				target, err := os.CreateTemp("", "target.tgz")
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, os.RemoveAll(target.Name()))
				}()
				require.NoError(t, target.Close())
				outputDir := t.TempDir()

				params := map[string]interface{}{
					"target":        target.Name(),
					"source_dir":    testDataDir,
					"include":       []string{"targz_me/dir1/**"},
					"exclude_files": []string{"*.pdb"},
				}

				So(cmd.ParseParams(params), ShouldBeNil)
				numFound, err := cmd.makeArchive(ctx, logger.Task())
				So(err, ShouldBeNil)
				So(numFound, ShouldEqual, 1)

				exists := utility.FileExists(target.Name())
				So(exists, ShouldBeTrue)

				var targetPath string
				if runtime.GOOS == "windows" {
					// On Windows, the tar command is provided by Cygwin, which
					// requires that you pass Unix-style Cygwin paths to it.
					cygpath, err := exec.LookPath("cygpath")
					require.NoError(t, err)

					output := util.NewMBCappedWriter()

					require.NoError(t, jasper.NewCommand().Add([]string{cygpath, "-u", target.Name()}).SetCombinedWriter(output).Run(ctx))
					targetPath = strings.TrimSpace(output.String())
				} else {
					targetPath = target.Name()
				}

				So(os.MkdirAll(outputDir, 0755), ShouldBeNil)
				// untar the file
				untarCmd := jasper.BuildCommand("extract test", level.Info, []string{"tar", "-zxvf", targetPath}, outputDir, nil)
				So(untarCmd.Run(context.TODO()), ShouldBeNil)

				// make sure that the correct files were included
				exists = utility.FileExists(filepath.Join(outputDir, "targz_me/dir1/dir2/testfile.txt"))
				So(exists, ShouldBeTrue)

				exists = utility.FileExists(filepath.Join(outputDir, "targz_me/dir1/dir2/test.pdb"))
				So(exists, ShouldBeFalse)
			})
		})
	})
}
