package command

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3PushParseParams(t *testing.T) {
	for testName, testCase := range map[string]func(*testing.T, *s3Push){
		"SetsDefaults": func(t *testing.T, c *s3Push) {
			require.NoError(t, c.ParseParams(map[string]interface{}{}))
			assert.Equal(t, (defaultS3MaxRetries), c.MaxRetries)
		},
		"SetsValues": func(t *testing.T, c *s3Push) {
			params := map[string]interface{}{
				"exclude_filter": "exclude_pattern",
				"build_variants": []string{"some_build_variant"},
				"max_retries":    uint(5),
			}
			require.NoError(t, c.ParseParams(params))
			assert.Equal(t, params["exclude_filter"], c.ExcludeFilter)
			assert.Equal(t, params["build_variants"], c.BuildVariants)
			assert.Equal(t, params["max_retries"], c.MaxRetries)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			c := &s3Push{}
			testCase(t, c)
		})
	}
}

func TestS3PushExecute(t *testing.T) {
	for testName, testCase := range map[string]func(context.Context, *testing.T, *s3Push, *client.Mock, client.LoggerProducer, *model.TaskConfig){
		"PushesTaskDirectoryToS3": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			tmpDir, err := ioutil.TempDir("", "s3-push")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpDir))
			}()
			tmpFile, err := ioutil.TempFile(tmpDir, "s3-push-file")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()
			fileContent := []byte("foobar")
			_, err = tmpFile.Write(fileContent)
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)
			conf.WorkDir = tmpDir

			require.NoError(t, c.Execute(ctx, comm, logger, conf))
			iter, err := c.bucket.List(ctx, conf.Task.S3Path(conf.Task.DisplayName))
			require.NoError(t, err)
			require.True(t, iter.Next(ctx))
			item := iter.Item()
			require.NotNil(t, item)
			assert.Equal(t, filepath.Base(tmpFile.Name()), filepath.Base(item.Name()))
			assert.Equal(t, conf.Task.S3Path(conf.Task.DisplayName), filepath.Dir(item.Name()))
			r, err := item.Get(ctx)
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, r.Close())
			}()
			pulledContent, err := ioutil.ReadAll(r)
			require.NoError(t, err)
			assert.Equal(t, pulledContent, fileContent)

			assert.False(t, iter.Next(ctx))
		},
		"IgnoresFilesExcludedByFilter": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			tmpDir, err := ioutil.TempDir("", "s3-push")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpDir))
			}()
			tmpFile, err := ioutil.TempFile(tmpDir, "s3-push-file")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()
			_, err = tmpFile.Write([]byte("foobar"))
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)
			conf.WorkDir = tmpDir

			c.ExcludeFilter = ".*"
			require.NoError(t, c.Execute(ctx, comm, logger, conf))
			iter, err := c.bucket.List(ctx, conf.Task.S3Path(conf.Task.DisplayName))
			require.NoError(t, err)
			assert.False(t, iter.Next(ctx))
		},
		"NoopsIfIgnoringBuildVariant": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			tmpDir, err := ioutil.TempDir("", "s3-push")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpDir))
			}()
			tmpFile, err := ioutil.TempFile(tmpDir, "s3-push-file")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()
			_, err = tmpFile.Write([]byte("foobar"))
			assert.NoError(t, tmpFile.Close())
			require.NoError(t, err)
			conf.WorkDir = tmpDir

			c.BuildVariants = []string{"other_build_variant"}
			require.NoError(t, c.Execute(ctx, comm, logger, conf))
			iter, err := c.bucket.List(ctx, conf.Task.S3Path(conf.Task.DisplayName))
			require.NoError(t, err)
			assert.False(t, iter.Next(ctx))
		},
		"ExpandsParameters": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			tmpDir, err := ioutil.TempDir("", "s3-push")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpDir))
			}()
			conf.WorkDir = tmpDir

			c.ExcludeFilter = "${exclude_filter}"
			excludeFilterExpansion := "expanded_exclude_filter"
			conf.Expansions = util.NewExpansions(map[string]string{
				"exclude_filter": excludeFilterExpansion,
			})
			assert.NoError(t, c.Execute(ctx, comm, logger, conf))
			assert.Equal(t, excludeFilterExpansion, c.ExcludeFilter)
		},
		"FailsWithoutS3Key": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			c.bucket = nil
			conf.S3Data.Key = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithoutS3Secret": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			c.bucket = nil
			conf.S3Data.Secret = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
		"FailsWithoutS3BucketName": func(ctx context.Context, t *testing.T, c *s3Push, comm *client.Mock, logger client.LoggerProducer, conf *model.TaskConfig) {
			c.bucket = nil
			conf.S3Data.Bucket = ""
			assert.Error(t, c.Execute(ctx, comm, logger, conf))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conf := &model.TaskConfig{
				Task: &task.Task{
					Id:           "id",
					Secret:       "secret",
					Version:      "version",
					BuildVariant: "build_variant",
					DisplayName:  "display_name",
				},
				BuildVariant: &model.BuildVariant{
					Name: "build_variant",
				},
				ProjectRef: &model.ProjectRef{
					Identifier: "project_identifier",
				},
				S3Data: apimodels.S3TaskSetupData{
					Key:    "s3_key",
					Secret: "s3_secret",
					Bucket: "s3_bucket",
				},
			}
			comm := client.NewMock("localhost")
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{
				ID:     conf.Task.Id,
				Secret: conf.Task.Secret,
			}, nil)
			require.NoError(t, err)
			tmpDir, err := ioutil.TempDir("", "s3-push-bucket")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpDir))
			}()
			c := &s3Push{}
			c.bucket, err = pail.NewLocalBucket(pail.LocalOptions{
				Path: tmpDir,
			})
			require.NoError(t, err)
			testCase(ctx, t, c, comm, logger, conf)
		})
	}
}
