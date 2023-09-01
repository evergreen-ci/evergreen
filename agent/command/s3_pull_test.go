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

func TestS3PullParseParams(t *testing.T) {
	for testName, testCase := range map[string]func(*testing.T, *s3Pull){
		"SetsValues": func(t *testing.T, c *s3Pull) {
			params := map[string]interface{}{
				"exclude":           "exclude_pattern",
				"max_retries":       uint(5),
				"task":              "task_name",
				"working_directory": "working_dir",
				"delete_on_sync":    true,
			}
			require.NoError(t, c.ParseParams(params))
			assert.Equal(t, params["exclude"], c.ExcludeFilter)
			assert.Equal(t, params["max_retries"], c.MaxRetries)
			assert.Equal(t, params["task"], c.Task)
			assert.Equal(t, params["working_directory"], c.WorkingDir)
			assert.Equal(t, params["delete_on_sync"], c.DeleteOnSync)
		},
		"RequiresWorkingDirectory": func(t *testing.T, c *s3Pull) {
			assert.Error(t, c.ParseParams(map[string]interface{}{}))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			c := &s3Pull{}
			testCase(t, c)
		})
	}
}

func TestS3PullExecute(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string){
		"PullsTaskDirectoryFromS3": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			taskDir := filepath.Join(bucketDir, conf.Task.S3Path(c.FromBuildVariant, c.Task))
			require.NoError(t, os.MkdirAll(taskDir, 0777))
			tmpFile, err := os.CreateTemp(taskDir, "file")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()
			fileContent := []byte("foobar")
			_, err = tmpFile.Write(fileContent)
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)

			c.WorkingDir = t.TempDir()

			require.NoError(t, c.Execute(ctx, comm, logger, conf))
			pulledContent, err := os.ReadFile(filepath.Join(c.WorkingDir, filepath.Base(tmpFile.Name())))
			require.NoError(t, err)
			assert.Equal(t, pulledContent, fileContent)
		},
		"IgnoresFilesExcludedByFilter": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			taskDir := filepath.Join(bucketDir, conf.Task.S3Path(c.FromBuildVariant, c.Task))
			require.NoError(t, os.MkdirAll(taskDir, 0777))
			tmpFile, err := os.CreateTemp(taskDir, "file")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()
			_, err = tmpFile.Write([]byte("foobar"))
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)

			c.WorkingDir = t.TempDir()

			c.ExcludeFilter = ".*"
			require.NoError(t, c.Execute(ctx, comm, logger, conf))

			files, err := os.ReadDir(c.WorkingDir)
			require.NoError(t, err)
			assert.Empty(t, files)
		},
		"ExpandsParameters": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			taskDir := filepath.Join(bucketDir, conf.Task.S3Path(c.FromBuildVariant, c.Task))
			require.NoError(t, os.MkdirAll(taskDir, 0777))

			require.NoError(t, os.WriteFile(filepath.Join(taskDir, "foo"), []byte("bar"), 0777))

			c.WorkingDir = t.TempDir()

			c.ExcludeFilter = "${exclude_filter}"
			excludeFilterExpansion := "expanded_exclude_filter"
			conf.Expansions = *util.NewExpansions(map[string]string{
				"exclude_filter": excludeFilterExpansion,
			})
			assert.NoError(t, c.Execute(ctx, comm, logger, conf))
			assert.Equal(t, excludeFilterExpansion, c.ExcludeFilter)
		},
		"FailsWithoutS3Key": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			c.bucket = nil
			conf.TaskSync.Key = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithoutS3Secret": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			c.bucket = nil
			conf.TaskSync.Secret = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithoutS3BucketName": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			c.bucket = nil
			conf.TaskSync.Bucket = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithNoContentsToPull": func(ctx context.Context, t *testing.T, c *s3Pull, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig, bucketDir string) {
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf := &internal.TaskConfig{
				Task: task.Task{
					Id:           "id",
					Project:      "project",
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
					Key:    "key",
					Secret: "secret",
					Bucket: "bucket",
				},
			}
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{
				ID:     conf.Task.Id,
				Secret: conf.Task.Secret,
			}, nil)
			require.NoError(t, err)
			tmpDir := t.TempDir()
			c := &s3Pull{Task: "task", FromBuildVariant: "from_build_variant"}
			c.bucket, err = pail.NewLocalBucket(pail.LocalOptions{
				Path: tmpDir,
			})
			require.NoError(t, err)
			testCase(ctx, t, c, comm, logger, conf, tmpDir)
		})
	}
}
