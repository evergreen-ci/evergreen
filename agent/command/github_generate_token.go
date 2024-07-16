package command

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/v52/github"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	githubGenerateTokenAttribute = "evergreen.command.github_generate_token"
)

var (
	githubGenerateTokenOwnerAttribute         = fmt.Sprintf("%s.owner", githubGenerateTokenAttribute)
	githubGenerateTokenRepoAttribute          = fmt.Sprintf("%s.repo", githubGenerateTokenAttribute)
	githubGenerateTokenAllPermissionAttribute = fmt.Sprintf("%s.all_permissions", githubGenerateTokenAttribute)
)

type githubGenerateToken struct {
	// Owner of the repository. If not provided, the owner of the project is used.
	Owner string `mapstructure:"owner" plugin:"expand"`

	// Repo name. If not provided, the repo of the project is used
	Repo string `mapstructure:"repo" plugin:"expand"`

	// ExpansionName is what the generated token will be saved as.
	ExpansionName string `mapstructure:"expansion_name"`

	// Permissions to grant the token. If not provided, set to nil to grant all permissions.
	// The command can never specify to restrict all permissions- as it would
	// be the same as not using a token.
	Permissions *github.InstallationPermissions `mapstructure:"permissions"`

	base
}

func githubGenerateTokenFactory() Command   { return &githubGenerateToken{} }
func (r *githubGenerateToken) Name() string { return "github.generate_token" }

func (r *githubGenerateToken) ParseParams(params map[string]interface{}) error {
	// Extract permissions and remove it before decoding.
	permissions := params["permissions"]
	delete(params, "permissions")

	// Decode all parameters except permissions.
	if err := mapstructure.Decode(params, r); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	// We decode the permissions separately since GitHub only adds json struct tags.
	if permissions != nil {
		r.Permissions = &github.InstallationPermissions{}
		metadata := mapstructure.Metadata{}
		jsonDecoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:      r.Permissions,
			TagName:     "json",
			ErrorUnused: true,
			Metadata:    &metadata,
		})
		if err != nil {
			return errors.Wrap(err, "creating json decoder")
		}
		if err := jsonDecoder.Decode(permissions); err != nil {
			return errors.Wrap(err, "decoding permissions")
		}
		// If no keys were decoded, we assume all permissions should be granted.
		// And we set the permissions back to nil.
		if len(metadata.Keys) == 0 {
			r.Permissions = nil
		}
	}

	return r.validate()
}

func (r *githubGenerateToken) validate() error {
	if r.ExpansionName == "" {
		return errors.New("must specify expansion name")
	}
	return nil
}

func (r *githubGenerateToken) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(r, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}
	if r.Owner == "" {
		r.Owner = conf.ProjectRef.Owner
	}
	if r.Repo == "" {
		r.Repo = conf.ProjectRef.Repo
	}

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String(githubGenerateTokenOwnerAttribute, r.Owner),
		attribute.String(githubGenerateTokenRepoAttribute, r.Repo),
		attribute.Bool(githubGenerateTokenAllPermissionAttribute, r.Permissions == nil),
	)
	createTokenCtx, span := getTracer().Start(ctx, "create_token")
	token, err := comm.CreateGitHubDynamicAccessToken(createTokenCtx, td, r.Owner, r.Repo, r.Permissions)
	span.End()
	if err != nil {
		return errors.Wrap(err, "creating github dynamic access token")
	}

	// We write or overwrite the expansion with the new token.
	conf.NewExpansions.PutAndRedact(r.ExpansionName, token)

	conf.AddCommandCleanup(r.FullDisplayName(), func(ctx context.Context) error {
		// We remove the expansion and revoke the token. We do not restore
		// the expansion to any previous value as overwriting the token
		// reduces the scope of the token.
		conf.NewExpansions.Remove(r.ExpansionName)

		// This span bundles the attributes again because this will not be a child span of the
		// spans above.
		revokeTokenCtx, span := getTracer().Start(ctx, "revoke_token", trace.WithAttributes(
			attribute.String(githubGenerateTokenOwnerAttribute, r.Owner),
			attribute.String(githubGenerateTokenRepoAttribute, r.Repo),
			attribute.Bool(githubGenerateTokenAllPermissionAttribute, r.Permissions == nil),
		))
		defer span.End()
		return errors.Wrap(comm.RevokeGitHubDynamicAccessToken(revokeTokenCtx, td, token), "revoking token")
	})

	return nil
}
