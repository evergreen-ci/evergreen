package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEC2AssumeRoleParse(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"FailsWithNoARN": func(t *testing.T) {
			c := &ec2AssumeRole{}
			assert.Error(t, c.ParseParams(map[string]interface{}{}))
		},
		"FailsWithInvalidDuration": func(t *testing.T) {
			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: -10,
			}
			assert.Error(t, c.ParseParams(map[string]interface{}{}))
		},
		"SucceedsWithValidParams": func(t *testing.T) {
			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: 10,
			}
			assert.NoError(t, c.ParseParams(map[string]interface{}{}))
		},
	} {
		t.Run(tName, tCase)
	}
}

func TestEC2AssumeRoleExecute(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"TemporaryFeatureFlagDisabled": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: 10,
			}
			assert.EqualError(t, c.Execute(ctx, comm, logger, conf), "no EC2 keys in config")
		},
		"TemporaryFeatureFlagEnabled": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			// TODO (DEVPROD-9945): Migration to new implementation. That should include tests for the new implementations Execute method.
			c := &ec2AssumeRole{
				RoleARN:              "randomRoleArn1234567890",
				DurationSeconds:      10,
				TemporaryFeatureFlag: true,
			}
			assert.EqualError(t, c.Execute(ctx, comm, logger, conf), "temporary feature flag is enabled")
		},
	} {
		t.Run(tName, func(t *testing.T) {
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
			}

			comm := client.NewMock("localhost")

			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			tCase(ctx, t, comm, logger, conf)
		})
	}
}
