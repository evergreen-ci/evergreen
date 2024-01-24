package command

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mholt/archiver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveAutoPackParseParams(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, cmd Command){
		"FailsWithNilParams": func(t *testing.T, cmd Command) {
			assert.Error(t, cmd.ParseParams(nil))
		},
		"FailsWithEmptyParams": func(t *testing.T, cmd Command) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{}))
		},
		"FailsWithInvalidParamTypes": func(t *testing.T, cmd Command) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"target":        1,
				"source_dir":    2,
				"include":       3,
				"exclude_files": 4,
			}))
		},
		"SucceedsWithValidParams": func(t *testing.T, cmd Command) {
			assert.NoError(t, cmd.ParseParams(map[string]interface{}{
				"target":     "some_target",
				"source_dir": "some_source_dir",
			}))
			assert.NoError(t, cmd.ParseParams(map[string]interface{}{
				"target":        "some_target",
				"source_dir":    "some_source_dir",
				"include":       []string{"included_file"},
				"exclude_files": []string{"excluded_file"},
			}))
		},
		"FailsWithoutSource": func(t *testing.T, cmd Command) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"target":  "some_target",
				"include": []string{"included_file"},
				"exclude": []string{"excluded_file"},
			}))
		},
		"FailsWithoutTarget": func(t *testing.T, cmd Command) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"source_dir": "some_source_dir",
				"include":    []string{"included_file"},
				"exclude":    []string{"excluded_file"},
			}))
		},
		"FailsWithExcludedFilesButNoIncludedFiles": func(t *testing.T, cmd Command) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"target":        "some_target",
				"source_dir":    "some_source_dir",
				"exclude_files": []string{"excluded_file"},
			}))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, autoArchiveCreateFactory())
		})
	}
}

func TestArchiveAutoPackExecute(t *testing.T) {
	thisDir := testutil.GetDirectoryOfFile()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FailsWithMalformedExpansion": func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.Target = "${badexpansion"
			cmd.SourceDir = thisDir
			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"FailsWithNonexistentSourceDir": func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.SourceDir = "nonexistent"
			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"FailsWithoutTargetArchiveExtension": func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.Target = "some_file"
			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"SucceedsAndCreatesArchive": func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			expectedEntriesInArchive := map[string]bool{}
			require.NoError(t, filepath.Walk(cmd.SourceDir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// When compressing a whole directory, expect paths that are
				// prefixed by the source directory (i.e. command).
				pathRelativeToSource, err := filepath.Rel(filepath.Dir(cmd.SourceDir), path)
				require.NoError(t, err)
				expectedEntriesInArchive[pathRelativeToSource] = false

				return nil
			}), "should not error while searching for files to archive")

			assert.NoError(t, cmd.Execute(ctx, client, logger, conf))

			unarchiveDir := t.TempDir()
			require.NoError(t, archiver.NewTarGz().Unarchive(cmd.Target, unarchiveDir))

			require.NoError(t, filepath.Walk(unarchiveDir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				pathRelativeToUnarchiveDir, err := filepath.Rel(unarchiveDir, path)
				require.NoError(t, err)
				if pathRelativeToUnarchiveDir == "." {
					return nil
				}

				_, ok := expectedEntriesInArchive[pathRelativeToUnarchiveDir]
				if assert.True(t, ok, "unexpected file '%s' found in archive", pathRelativeToUnarchiveDir) {
					expectedEntriesInArchive[pathRelativeToUnarchiveDir] = true
				}

				return nil
			}), "should not error while searching unarchived files")

			for filename, found := range expectedEntriesInArchive {
				assert.True(t, found, "expected to find file '%s' in archive", filename)
			}
		},
		"SucceedsAndCreatesArchiveWithIncludedFiles": func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.Include = []string{"*.go"}

			entries, err := os.ReadDir(cmd.SourceDir)
			require.NoError(t, err)
			expectedEntriesInArchive := map[string]bool{}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".go") {
					// When including individual files in the source directory,
					// expect the files to not be prefixed by the source
					// directory (i.e. command).
					require.NoError(t, err)
					expectedEntriesInArchive[entry.Name()] = true
				}
			}

			assert.NoError(t, cmd.Execute(ctx, client, logger, conf))

			unarchiveDir := t.TempDir()
			require.NoError(t, archiver.NewTarGz().Unarchive(cmd.Target, unarchiveDir))

			require.NoError(t, filepath.Walk(unarchiveDir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				pathRelativeToUnarchiveDir, err := filepath.Rel(unarchiveDir, path)
				require.NoError(t, err)
				if pathRelativeToUnarchiveDir == "." {
					return nil
				}

				_, ok := expectedEntriesInArchive[pathRelativeToUnarchiveDir]
				if assert.True(t, ok, "unexpected file '%s' found in archive", pathRelativeToUnarchiveDir) {
					expectedEntriesInArchive[pathRelativeToUnarchiveDir] = true
				}

				return nil
			}), "should not error while searching unarchived files")

			for filename, found := range expectedEntriesInArchive {
				assert.True(t, found, "expected to find file '%s' in archive", filename)
			}
		},
		"SucceedsAndCreatesArchiveWithIncludedFilesAndExcludedFiles": func(ctx context.Context, t *testing.T, cmd *autoArchiveCreate, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			_, thisFile, _, _ := runtime.Caller(0)
			excludedFile := filepath.Base(thisFile)
			cmd.Include = []string{"*.go"}
			// TODO (DEVPROD-2664): File exclusion is currently working in a
			// weird way - exclude_files will only exclude files based on the
			// absolute path, not the path relative to source_dir.
			// Fix this test so that exclude_files allows patterns relative to
			// the source_dir. In other words, this test ought to pass
			// excludedFile (the path relative to the source dir) here instead
			// of filepath.FromSlash(thisFile) (the absolute file path with
			// native file separators).
			cmd.ExcludeFiles = []string{filepath.FromSlash(thisFile)}

			entries, err := os.ReadDir(cmd.SourceDir)
			require.NoError(t, err)
			expectedEntriesInArchive := map[string]bool{}
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".go") && entry.Name() != excludedFile {
					// When including individual files in the source directory,
					// expect the files to not be prefixed by the source
					// directory (i.e. command).
					require.NoError(t, err)
					expectedEntriesInArchive[entry.Name()] = true
				}
			}

			assert.NoError(t, cmd.Execute(ctx, client, logger, conf))

			unarchiveDir := t.TempDir()
			require.NoError(t, archiver.NewTarGz().Unarchive(cmd.Target, unarchiveDir))

			require.NoError(t, filepath.Walk(unarchiveDir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				pathRelativeToUnarchiveDir, err := filepath.Rel(unarchiveDir, path)
				require.NoError(t, err)
				if pathRelativeToUnarchiveDir == "." {
					return nil
				}

				_, ok := expectedEntriesInArchive[pathRelativeToUnarchiveDir]
				if assert.True(t, ok, "unexpected file '%s' found in archive", pathRelativeToUnarchiveDir) {
					expectedEntriesInArchive[pathRelativeToUnarchiveDir] = true
				}

				return nil
			}), "should not error while searching unarchived files")

			for filename, found := range expectedEntriesInArchive {
				assert.True(t, found, "expected to find file '%s' in archive", filename)
			}
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf, err := internal.NewTaskConfig(t.TempDir(),
				&apimodels.DistroView{},
				&model.Project{BuildVariants: []model.BuildVariant{{Name: "bv"}}},
				&task.Task{BuildVariant: "bv"},
				&model.ProjectRef{},
				&patch.Patch{},
				&apimodels.ExpansionsAndVars{},
			)
			require.NoError(t, err)
			comm := client.NewMock("url")
			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			tCase(ctx, t, &autoArchiveCreate{
				SourceDir: thisDir,
				Target:    filepath.Join(t.TempDir(), "archive.tgz"),
			}, comm, logger, conf)
		})
	}
}
