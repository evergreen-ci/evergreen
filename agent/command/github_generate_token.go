package command

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/v70/github"
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
	ExpansionName string `mapstructure:"expansion_name" plugin:"expand"`

	// Permissions to grant the token. If not provided, set to nil to grant all permissions.
	// The command can never specify to restrict all permissions- as it would
	// be the same as not using a token.
	Permissions *github.InstallationPermissions `mapstructure:"permissions"`

	base
}

func githubGenerateTokenFactory() Command   { return &githubGenerateToken{} }
func (r *githubGenerateToken) Name() string { return "github.generate_token" }

func (r *githubGenerateToken) ParseParams(params map[string]any) error {
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

	logger.Task().Infof("Requesting a GitHub dynamic access token with owner:%s, repository:%s, permissions:%s", r.Owner, r.Repo, permissionsToString(r.Permissions))
	token, permissions, err := comm.CreateGitHubDynamicAccessToken(ctx, td, r.Owner, r.Repo, r.Permissions)
	if err != nil {
		return errors.Wrap(err, "creating github dynamic access token")
	}

	logger.Task().Infof("Created a GitHub dynamic access token. The token has the following permissions: %s", permissionsToString(permissions))

	// We write or overwrite the expansion with the new token.
	conf.NewExpansions.PutAndRedact(r.ExpansionName, token)

	conf.AddCommandCleanup(r.Name(), func(ctx context.Context) error {
		// We remove the expansion and revoke the token. We do not restore
		// the expansion to any previous value as overwriting the token
		// reduces the scope of the token.
		conf.NewExpansions.Remove(r.ExpansionName)

		trace.SpanFromContext(ctx).SetAttributes(
			attribute.String(githubGenerateTokenOwnerAttribute, r.Owner),
			attribute.String(githubGenerateTokenRepoAttribute, r.Repo),
			attribute.Bool(githubGenerateTokenAllPermissionAttribute, r.Permissions == nil),
		)
		return errors.Wrap(comm.RevokeGitHubDynamicAccessToken(ctx, td, token), "revoking token")
	})

	return nil
}

// permissionsToString converts the permissions struct to a string in the format [key:value, key:value].
func permissionsToString(permissions *github.InstallationPermissions) string {
	if permissions == nil {
		return "[]"
	}
	val := reflect.ValueOf(permissions).Elem()
	var p []string
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		if field.Kind() != reflect.Ptr || field.Elem().Kind() != reflect.String {
			continue
		}

		if !field.IsNil() {
			p = append(p, fmt.Sprintf("%s:%v", val.Type().Field(i).Name, *field.Interface().(*string)))
		}
	}

	return "[" + strings.Join(p, ", ") + "]"
}
