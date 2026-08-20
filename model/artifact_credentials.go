package model

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

// ArtifactCredentialSettings names where to find a project's current AWS artifact
// credentials, as the names of the project variables holding them.
type ArtifactCredentialSettings struct {
	// AWSKeyVarName is the name of the project variable holding the AWS access key ID.
	AWSKeyVarName string `bson:"aws_key_var_name,omitempty" json:"aws_key_var_name,omitempty" yaml:"aws_key_var_name,omitempty"`
	// AWSSecretVarName is the name of the project variable holding the AWS secret key.
	AWSSecretVarName string `bson:"aws_secret_var_name,omitempty" json:"aws_secret_var_name,omitempty" yaml:"aws_secret_var_name,omitempty"`
}

// Validate rejects settings that cannot resolve, such as half of the static pair.
func (s ArtifactCredentialSettings) Validate() error {
	if (s.AWSKeyVarName == "") != (s.AWSSecretVarName == "") {
		return errors.New("AWS key and secret variable names must be set together")
	}

	return nil
}

// artifactCredentialResolver resolves credentials for one task, caching its
// project lookups.
type artifactCredentialResolver struct {
	taskID string

	loaded      bool
	loadErr     error
	settings    ArtifactCredentialSettings
	projectVars map[string]string
}

// NewArtifactCredentialResolver returns a resolver for the given task's project.
// It caches, so it must not be shared across requests.
func NewArtifactCredentialResolver(taskID string) artifact.CredentialResolver {
	r := &artifactCredentialResolver{taskID: taskID}
	return r.resolve
}

func (r *artifactCredentialResolver) resolve(ctx context.Context, file artifact.File) (*artifact.Credentials, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}
	if flags.LiveArtifactCredentialsDisabled {
		return nil, nil
	}

	if err := r.load(ctx); err != nil {
		return nil, err
	}

	// Names on the artifact are more specific than the project-wide defaults.
	keyVar, secretVar := file.AWSKeyVarName, file.AWSSecretVarName
	if keyVar == "" || secretVar == "" {
		keyVar, secretVar = r.settings.AWSKeyVarName, r.settings.AWSSecretVarName
	}
	if keyVar == "" || secretVar == "" {
		return nil, nil
	}

	key, secret := r.projectVars[keyVar], r.projectVars[secretVar]
	if key == "" || secret == "" {
		return nil, errors.Errorf("project variables '%s' and '%s' for task '%s' did not both resolve to a value", keyVar, secretVar, r.taskID)
	}

	return &artifact.Credentials{AWSKey: key, AWSSecret: secret}, nil
}

// load fetches and caches the task's project settings and variables.
func (r *artifactCredentialResolver) load(ctx context.Context) error {
	if r.loaded {
		return r.loadErr
	}
	r.loaded = true
	r.loadErr = r.loadUncached(ctx)
	return r.loadErr
}

func (r *artifactCredentialResolver) loadUncached(ctx context.Context) error {
	t, err := task.FindOneIdWithFields(ctx, r.taskID, task.ProjectKey, task.VersionKey)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", r.taskID)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", r.taskID)
	}

	pRef, err := FindMergedProjectRef(ctx, t.Project, t.Version, false)
	if err != nil {
		return errors.Wrapf(err, "finding project ref for project '%s'", t.Project)
	}
	if pRef == nil {
		return errors.Errorf("project ref for project '%s' not found", t.Project)
	}
	r.settings = pRef.ArtifactCredentials

	vars, err := FindMergedProjectVars(ctx, t.Project)
	if err != nil {
		return errors.Wrapf(err, "finding project variables for project '%s'", t.Project)
	}
	if vars != nil {
		r.projectVars = vars.Vars
	}

	return nil
}
