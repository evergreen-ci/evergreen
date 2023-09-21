package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3CopyExecute(t *testing.T) {
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *s3copy, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FailsWithNoContentsToPull": func(ctx context.Context, t *testing.T, c *s3copy, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
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

			c := &s3copy{S3CopyFiles: []*s3CopyFile{
				{
					Source: s3Loc{
						Bucket: "bucket",
						Path:   "path",
					},
				},
			}}
			require.NoError(t, err)
			testCase(ctx, t, c, comm, logger, conf)
		})
	}
}
