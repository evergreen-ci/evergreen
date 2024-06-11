package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/v52/github"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type githubGenerateToken struct {
	// Owner of the repository. If not provided, the owner of the project is used.
	Owner string `mapstructure:"owner" plugin:"expand"`

	// Repo name. If not provided, the repo of the project is used
	Repo string `mapstructure:"repo" plugin:"expand"`

	// ExpansionName is what the generated token will be saved as.
	ExpansionName string `mapstructure:"expansion_name"`

	// Permissions to grant the token. If not provided, set to nil to grant all permissions.
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
		if len(metadata.Keys) == 0 {
			r.Permissions = nil
		}
	} else {
		// If no permissions were provided, we assume all permissions should be granted.len(metadata.Keys) == 0
		r.Permissions = nil
	}

	return r.validate()
}

func (r *githubGenerateToken) validate() error {
	catcher := grip.NewSimpleCatcher()

	if r.ExpansionName == "" {
		catcher.New("must specify expansion name")
	}

	return catcher.Resolve()
}

func (r *githubGenerateToken) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(r, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}
	// Re-validate the command here, in case an expansion is not defined.
	if err := r.validate(); err != nil {
		return errors.WithStack(err)
	}

	token, err := comm.CreateGitHubDynamicAccessToken(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, r.Owner, r.Repo, r.Permissions)
	if err != nil {
		return errors.Wrap(err, "creating github dynamic access token")
	}

	// TODO DEVPROD-5986: Tokens should be redacted at the end, expanisions (or some other mechanism) should
	// keep track of that and redact the token from GitHub.
	// They also need to be redacted from logs.
	conf.NewExpansions.Put(r.ExpansionName, token)

	return nil
}
