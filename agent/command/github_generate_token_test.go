package command

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubGenerateTokenParseParams(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, cmd *githubGenerateToken){
		"FailsWithNilParams": func(t *testing.T, cmd *githubGenerateToken) {
			assert.Error(t, cmd.ParseParams(nil))
		},
		"FailsWithEmptyParams": func(t *testing.T, cmd *githubGenerateToken) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{}))
		},
		"FailsWithInvalidParamTypes": func(t *testing.T, cmd *githubGenerateToken) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"owner": 1,
			}))
		},
		"FailsWithInvalidPermissions": func(t *testing.T, cmd *githubGenerateToken) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"expansion_name": "expansion_name",
				"permissions": map[string]interface{}{
					"invalid": "test",
				},
			}))
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"expansion_name": "expansion_name",
				"permissions": map[string]interface{}{
					"invalid": 1,
				},
			}))
		},
		"SucceedsWithValidParamsNoPermissions": func(t *testing.T, cmd *githubGenerateToken) {
			assert.NoError(t, cmd.ParseParams(map[string]interface{}{
				"owner":          "owner",
				"repo":           "repo",
				"expansion_name": "expansion_name",
			}))
			assert.Nil(t, cmd.Permissions)
		},
		"SucceedsWithValidParamsEmptyPermissions": func(t *testing.T, cmd *githubGenerateToken) {
			assert.NoError(t, cmd.ParseParams(map[string]interface{}{
				"owner":          "owner",
				"repo":           "repo",
				"expansion_name": "expansion_name",
				"permissions":    map[string]interface{}{},
			}))
			assert.Nil(t, cmd.Permissions)
		},
		"SucceedsWithValidParamsSomePermissions": func(t *testing.T, cmd *githubGenerateToken) {
			assert.NoError(t, cmd.ParseParams(map[string]interface{}{
				"owner":          "owner",
				"repo":           "repo",
				"expansion_name": "expansion_name",
				"permissions": map[string]interface{}{
					"actions": "actions",
					"checks":  "checks",
				},
			}))
			require.NotNil(t, cmd.Permissions)
			assert.Equal(t, "actions", utility.FromStringPtr(cmd.Permissions.Actions))
			assert.Equal(t, "checks", utility.FromStringPtr(cmd.Permissions.Checks))
			assert.Nil(t, cmd.Permissions.Administration)
		},
		"FailsWithoutExpansionName": func(t *testing.T, cmd *githubGenerateToken) {
			assert.Error(t, cmd.ParseParams(map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
			}))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			cmd, ok := githubGenerateTokenFactory().(*githubGenerateToken)
			require.True(t, ok)

			tCase(t, cmd)
		})
	}
}

func TestGitHubGenerateTokenExecute(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FailsWithMalformedExpansion": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.Owner = "${badexpansion"
			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"SucceedsAndCreatesToken": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
			assert.Equal(t, "token!", conf.NewExpansions.Get(cmd.ExpansionName))
		},
		"SucceedsWithEmptyOwnerAndRepoAndCreatesToken": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.Owner = ""
			cmd.Repo = ""
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
			assert.Equal(t, "token!", conf.NewExpansions.Get(cmd.ExpansionName))
			assert.Equal(t, "new_owner", cmd.Owner)
			assert.Equal(t, "new_repo", cmd.Repo)

			require.Len(t, conf.CommandCleanups, 1)
			cleanup := conf.CommandCleanups[0]
			assert.Equal(t, "revoking token", cleanup.Name)
			assert.Equal(t, "github.generate_token", cleanup.Command)
			// The cleanup function is expected to return an error mocked
			// in the test data.
			assert.EqualError(t, cleanup.Run(ctx), "revoked!")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf := &internal.TaskConfig{NewExpansions: agentutil.NewDynamicExpansions(util.Expansions{}), ProjectRef: model.ProjectRef{Owner: "new_owner", Repo: "new_repo"}}
			comm := client.NewMock("url")
			comm.CreateGitHubDynamicAccessTokenResult = "token!"
			comm.RevokeGitHubDynamicAccessTokenResult = "revoked!"
			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			tCase(ctx, t, &githubGenerateToken{
				Owner:         "owner",
				Repo:          "repo",
				ExpansionName: "expansion_name",
			}, comm, logger, conf)
		})
	}
}
