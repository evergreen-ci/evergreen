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
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig){
		"FailsWithMalformedExpansion": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			cmd.Owner = "${badexpansion"
			assert.Error(t, cmd.Execute(ctx, client, logger, conf))
		},
		"FailsIfCreatingFails": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			client.CreateGitHubDynamicAccessTokenFail = true
			assert.ErrorContains(t, cmd.Execute(ctx, client, logger, conf), "creating github dynamic access token: failed to create token")
		},
		"TokenRedactionSurfacesError": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			client.CreateGitHubDynamicAccessTokenResult = "token!"
			client.RevokeGitHubDynamicAccessTokenFail = true
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
			assert.Equal(t, client.CreateGitHubDynamicAccessTokenResult, conf.NewExpansions.Get(cmd.ExpansionName))

			require.Len(t, conf.CommandCleanups, 1)
			cleanup := conf.CommandCleanups[0]
			assert.Equal(t, "github.generate_token", cleanup.Command)
			assert.ErrorContains(t, cleanup.Run(ctx), "revoking token: failed to revoke token")
		},
		"SucceedsAndCreatesToken": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			client.CreateGitHubDynamicAccessTokenResult = "token!"
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
			assert.Equal(t, client.CreateGitHubDynamicAccessTokenResult, conf.NewExpansions.Get(cmd.ExpansionName))

			require.Len(t, conf.CommandCleanups, 1)
			cleanup := conf.CommandCleanups[0]
			assert.Equal(t, "github.generate_token", cleanup.Command)
			assert.Nil(t, cleanup.Run(ctx))
		},
		"SucceedsWithEmptyOwnerAndRepoAndCreatesToken": func(ctx context.Context, t *testing.T, cmd *githubGenerateToken, client *client.Mock, logger client.LoggerProducer, conf *internal.TaskConfig) {
			client.CreateGitHubDynamicAccessTokenResult = "token!"
			cmd.Owner = ""
			cmd.Repo = ""
			require.NoError(t, cmd.Execute(ctx, client, logger, conf))
			assert.Equal(t, "token!", conf.NewExpansions.Get(cmd.ExpansionName))
			assert.Equal(t, "new_owner", cmd.Owner)
			assert.Equal(t, "new_repo", cmd.Repo)

			require.Len(t, conf.CommandCleanups, 1)
			cleanup := conf.CommandCleanups[0]
			assert.Equal(t, "github.generate_token", cleanup.Command)
			assert.Nil(t, cleanup.Run(ctx))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conf := &internal.TaskConfig{NewExpansions: agentutil.NewDynamicExpansions(util.Expansions{}), ProjectRef: model.ProjectRef{Owner: "new_owner", Repo: "new_repo"}}
			comm := client.NewMock("url")
			logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
			require.NoError(t, err)

			cmd := &githubGenerateToken{
				Owner:         "owner",
				Repo:          "repo",
				ExpansionName: "expansion_name",
			}
			cmd.SetFullDisplayName("github.generate_token")

			tCase(ctx, t, cmd, comm, logger, conf)
		})
	}
}
