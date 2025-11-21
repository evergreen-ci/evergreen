package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveTarballExtractParseParams(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, cmd *tarballExtract){
		"ArchivePathMustBeDefined": func(t *testing.T, cmd *tarballExtract) {
			assert.ErrorContains(t, cmd.ParseParams(map[string]any{
				"path":        "",
				"destination": "bar",
			}), "archive path")
		},
		"DestinationMustBeDefined": func(t *testing.T, cmd *tarballExtract) {
			assert.Error(t, cmd.ParseParams(map[string]any{
				"path":        "foo",
				"destination": "",
			}), "target directory")
		},
		"SucceedsWithValidParams": func(t *testing.T, cmd *tarballExtract) {
			params := map[string]any{
				"path":        "foo",
				"destination": "bar",
			}
			require.NoError(t, cmd.ParseParams(params))
			assert.Equal(t, params["path"], cmd.ArchivePath)
			assert.Equal(t, params["destination"], cmd.TargetDirectory)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			cmd, ok := tarballExtractFactory().(*tarballExtract)
			require.True(t, ok)

			tCase(t, cmd)
		})
	}
}

func TestArchiveTarballExtractExecute(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, cmd *tarballExtract, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FailsWithFileThatDoesNotExist": func(ctx context.Context, t *testing.T, cmd *tarballExtract, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
				"testdata", "archive", "nonexistent.tar.gz")

			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"FailsWithFileThatIsNotArchive": func(ctx context.Context, t *testing.T, cmd *tarballExtract, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			_, thisFile, _, _ := runtime.Caller(0)
			cmd.ArchivePath = thisFile

			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"SucceedsAndIsIdempotent": func(ctx context.Context, t *testing.T, cmd *tarballExtract, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))

			checkCommonExtractedArchiveContents(t, cmd.TargetDirectory)

			// Extracting the same archive contents multiple times to the same directory
			// results results in no error. The command simply ignores duplicate files.
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
		},
		"ExtractsEmptyDirectories": func(ctx context.Context, t *testing.T, cmd *tarballExtract, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			sourceDir := t.TempDir()
			rootToArchive := filepath.Join(sourceDir, "empty")
			require.NoError(t, os.MkdirAll(filepath.Join(rootToArchive, "logs", "nested"), 0755))

			targetArchive := filepath.Join(sourceDir, "empty_dirs.tgz")
			createCmd := &tarballCreate{
				Target:    targetArchive,
				SourceDir: rootToArchive,
				Include:   []string{"**"},
			}
			numFound, err := createCmd.makeArchive(ctx, logger.Task())
			require.NoError(t, err)
			require.Equal(t, 2, numFound)

			cmd.ArchivePath = targetArchive
			cmd.TargetDirectory = t.TempDir()

			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
			assert.DirExists(t, filepath.Join(cmd.TargetDirectory, "logs"))
			assert.DirExists(t, filepath.Join(cmd.TargetDirectory, "logs", "nested"))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf := &internal.TaskConfig{
				Expansions: util.Expansions{},
				Task:       task.Task{},
				Project:    model.Project{},
				WorkDir:    t.TempDir(),
			}
			comm := client.NewMock("url")
			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			cmd, ok := tarballExtractFactory().(*tarballExtract)
			require.True(t, ok)

			cmd.TargetDirectory = conf.WorkDir
			cmd.ArchivePath = filepath.Join(testutil.GetDirectoryOfFile(),
				"testdata", "archive", "artifacts.tar.gz")

			tCase(ctx, t, cmd, comm, logger, conf)
		})
	}
}
