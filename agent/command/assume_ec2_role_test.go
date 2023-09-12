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

func TestEc2AssumeRoleExecute(t *testing.T) {
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
		EC2Keys: []evergreen.EC2Key{
			evergreen.EC2Key{
				Key:    "aaaaaaaaaa",
				Secret: "bbbbbbbbbbb",
			},
		},
	}
	comm := client.NewMock("localhost")
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{
		ID:     conf.Task.Id,
		Secret: conf.Task.Secret,
	}, nil)
	require.NoError(t, err)
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FailsWithNoARN": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c := &ec2AssumeRole{}
			err := c.Execute(ctx, comm, logger, conf)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "must specify role ARN")
		},
		"FailsWithInvalidDuration": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: -10,
			}
			err := c.Execute(ctx, comm, logger, conf)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "cannot specify a non-positive duration")
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(ctx, t, comm, logger, conf)
		})
	}
}
