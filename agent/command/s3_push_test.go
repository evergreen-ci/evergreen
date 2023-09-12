package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3PushParseParams(t *testing.T) {
	for testName, testCase := range map[string]func(*testing.T, *s3Push){
		"SetsValues": func(t *testing.T, c *s3Push) {
			params := map[string]interface{}{
				"exclude":     "exclude_pattern",
				"max_retries": uint(5),
			}
			require.NoError(t, c.ParseParams(params))
			assert.Equal(t, params["exclude"], c.ExcludeFilter)
			assert.Equal(t, params["max_retries"], c.MaxRetries)
		},
		"SucceedsWithNoParameters": func(t *testing.T, c *s3Push) {
			assert.NoError(t, c.ParseParams(map[string]interface{}{}))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			c := &s3Push{}
			testCase(t, c)
		})
	}
}

func TestS3PushExecute(t *testing.T) {
	for testName, testCase := range map[string]func(context.Context, *testing.T, *s3Push, *client.Mock, client.LoggerProducer, *internal.TaskConfig){
		"PushesTaskDirectoryToS3": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			tmpDir := t.TempDir()
			workDir := filepath.Join(tmpDir, "work_dir")
			require.NoError(t, os.MkdirAll(workDir, 0777))
			tmpFile, err := os.CreateTemp(workDir, "file")
			require.NoError(t, err)
			fileContent := []byte("foobar")
			_, err = tmpFile.Write(fileContent)
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)
			conf.WorkDir = tmpDir

			require.NoError(t, c.Execute(ctx, comm, logger, conf))

			pullDir := filepath.Join(tmpDir, "pull_dir")
			require.NoError(t, os.MkdirAll(pullDir, 0777))

			s3Path := filepath.ToSlash(filepath.Join(conf.Task.S3Path(conf.Task.BuildVariant, conf.Task.DisplayName), filepath.Base(workDir)))
			require.NoError(t, c.bucket.Pull(ctx, pail.SyncOptions{
				Local:  pullDir,
				Remote: filepath.Join(s3Path),
			}))

			pulledContent, err := os.ReadFile(filepath.Join(pullDir, filepath.Base(tmpFile.Name())))
			require.NoError(t, err)
			assert.Equal(t, fileContent, pulledContent)
		},
		"IgnoresFilesExcludedByFilter": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			tmpDir := t.TempDir()
			workDir := filepath.Join(tmpDir, "work_dir")
			require.NoError(t, os.MkdirAll(workDir, 0777))
			tmpFile, err := os.CreateTemp(workDir, "file")
			require.NoError(t, err)
			_, err = tmpFile.Write([]byte("foobar"))
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)
			conf.WorkDir = workDir

			c.ExcludeFilter = ".*"
			require.NoError(t, c.Execute(ctx, comm, logger, conf))

			pullDir := filepath.Join(tmpDir, "pull_dir")
			require.NoError(t, os.MkdirAll(pullDir, 0777))

			s3Path := conf.Task.S3Path(conf.Task.BuildVariant, conf.Task.DisplayName)
			require.NoError(t, c.bucket.Pull(ctx, pail.SyncOptions{
				Local:  pullDir,
				Remote: s3Path,
			}))

			files, err := os.ReadDir(pullDir)
			require.NoError(t, err)
			assert.Empty(t, files)
		},
		"ExpandsParameters": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			conf.WorkDir = t.TempDir()

			c.ExcludeFilter = "${exclude_filter}"
			excludeFilterExpansion := "expanded_exclude_filter"
			conf.Expansions = *util.NewExpansions(map[string]string{
				"exclude_filter": excludeFilterExpansion,
			})
			assert.NoError(t, c.Execute(ctx, comm, logger, conf))
			assert.Equal(t, excludeFilterExpansion, c.ExcludeFilter)
		},
		"FailsWithoutS3Key": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c.bucket = nil
			conf.TaskSync.Key = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithoutS3Secret": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c.bucket = nil
			conf.TaskSync.Secret = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithoutS3BucketName": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c.bucket = nil
			conf.TaskSync.Bucket = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf := &internal.TaskConfig{
				Task: task.Task{
					Id:           "id",
					Secret:       "secret",
					Version:      "version",
					BuildVariant: "build_variant",
					DisplayName:  "display_name",
				},
				BuildVariant: model.BuildVariant{
					Name: "build_variant",
				},
				ProjectRef: model.ProjectRef{
					Id: "project_identifier",
					TaskSync: model.TaskSyncOptions{
						ConfigEnabled: utility.TruePtr(),
					},
				},
				TaskSync: evergreen.S3Credentials{
					Key:    "task_sync_key",
					Secret: "task_sync_secret",
					Bucket: "task_sync_bucket",
				},
			}
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{
				ID:     conf.Task.Id,
				Secret: conf.Task.Secret,
			}, nil)
			require.NoError(t, err)
			c := &s3Push{}
			c.bucket, err = pail.NewLocalBucket(pail.LocalOptions{
				Path: t.TempDir(),
			})
			require.NoError(t, err)
			testCase(ctx, t, c, comm, logger, conf)
		})
	}
}
