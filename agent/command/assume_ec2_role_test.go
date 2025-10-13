package command

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEC2AssumeRoleParse(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"FailsWithNoARN": func(t *testing.T) {
			c := &ec2AssumeRole{}
			assert.Error(t, c.ParseParams(map[string]any{}))
		},
		"FailsWithInvalidDuration": func(t *testing.T) {
			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: -10,
			}
			assert.Error(t, c.ParseParams(map[string]any{}))
		},
		"SucceedsWithValidParams": func(t *testing.T) {
			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: 10,
			}
			assert.NoError(t, c.ParseParams(map[string]any{}))
		},
	} {
		t.Run(tName, tCase)
	}
}

func TestEC2AssumeRoleExecute(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"BadAWSResponse": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			comm.AssumeRoleResponse = nil

			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: 10,
			}
			assert.EqualError(t, c.Execute(ctx, comm, logger, conf), "nil credentials returned")
		},
		"Success": func(ctx context.Context, t *testing.T, comm *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			expiration := time.Now()
			externalID := "ext_id"
			comm.AssumeRoleResponse = &apimodels.AWSCredentials{
				AccessKeyID:     "access_key_id",
				SecretAccessKey: "secret_access_key",
				SessionToken:    "session_token",
				Expiration:      expiration.Format(time.RFC3339),
				ExternalID:      externalID,
			}

			c := &ec2AssumeRole{
				RoleARN:         "randomRoleArn1234567890",
				DurationSeconds: 10,
			}
			require.NoError(t, c.Execute(ctx, comm, logger, conf))

			assert.Equal(t, "access_key_id", conf.NewExpansions.Get(globals.AWSAccessKeyId))
			assert.Equal(t, "secret_access_key", conf.NewExpansions.Get(globals.AWSSecretAccessKey))
			assert.Equal(t, "session_token", conf.NewExpansions.Get(globals.AWSSessionToken))
			assert.Equal(t, expiration.Format(time.RFC3339), conf.NewExpansions.Get(globals.AWSRoleExpiration))

			assert.Equal(t, c.RoleARN, conf.AssumeRoleInformation[comm.AssumeRoleResponse.SessionToken].RoleARN)
			assert.Equal(t, externalID, conf.AssumeRoleInformation[comm.AssumeRoleResponse.SessionToken].ExternalID)
			assert.WithinDuration(t, expiration, conf.AssumeRoleInformation[comm.AssumeRoleResponse.SessionToken].Expiration, time.Second)

			t.Run("KeysAreRedacted", func(t *testing.T) {
				hasAccessKey := false
				hasSecretAccessKey := false
				hasSessionToken := false
				hasExpiration := false

				for _, redacted := range conf.NewExpansions.GetRedacted() {
					switch redacted.Key {
					case globals.AWSAccessKeyId:
						hasAccessKey = true
					case globals.AWSSecretAccessKey:
						hasSecretAccessKey = true
					case globals.AWSSessionToken:
						hasSessionToken = true
					case globals.AWSRoleExpiration:
						hasExpiration = true
					}
				}

				assert.True(t, hasAccessKey)
				assert.True(t, hasSecretAccessKey)
				assert.True(t, hasSessionToken)
				// The expiration should not be redacted.
				assert.False(t, hasExpiration)
			})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			expansions := util.Expansions{}
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
				},
				Expansions:            expansions,
				NewExpansions:         agentutil.NewDynamicExpansions(expansions),
				AssumeRoleInformation: map[string]internal.AssumeRoleInformation{},
			}

			comm := client.NewMock("localhost")

			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			tCase(ctx, t, comm, logger, conf)
		})
	}
}
