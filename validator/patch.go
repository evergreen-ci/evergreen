package validator

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/pkg/errors"
)

// GetPatchedProject creates and validates a project created by fetching latest commit information from GitHub
// and applying the patch to the latest remote configuration. The error returned can be a validation error.
func GetPatchedProject(ctx context.Context, p *patch.Patch, githubOauthToken string) (*model.Project, error) {
	if p.Version != "" {
		return nil, errors.Errorf("Patch %v already finalized", p.Version)
	}
	projectRef, err := model.FindOneProjectRef(p.Project)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// try to get the remote project file data at the requested revision
	var projectFileBytes []byte
	hash := p.Githash
	path := projectRef.RemotePath
	if p.IsGithubPRPatch() {
		hash = p.GithubPatchData.HeadHash
	}
	if p.IsPRMergePatch() {
		hash = p.GithubPatchData.MergeCommitSHA
		path = projectRef.CommitQueueConfigFile
	}

	githubFile, err := thirdparty.GetGithubFile(ctx, githubOauthToken, projectRef.Owner,
		projectRef.Repo, path, hash)
	if err != nil {
		// if the project file doesn't exist, but our patch includes a project file,
		// we try to apply the diff and proceed.
		if !(p.ConfigChanged(path) && thirdparty.IsFileNotFound(err)) {
			// return an error if the github error is network/auth-related or we aren't patching the config
			return nil, errors.Wrapf(err, "Could not get github file at '%s/%s'@%s: %s", projectRef.Owner,
				projectRef.Repo, path, hash)
		}
	} else {
		// we successfully got the project file in base64, so we decode it
		projectFileBytes, err = base64.StdEncoding.DecodeString(*githubFile.Content)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not decode github file at '%s/%s'@%s: %s", projectRef.Owner,
				projectRef.Repo, path, hash)
		}
	}

	project := &model.Project{}

	// if the patched config exists, use that as the project file bytes.
	if p.PatchedConfig != "" {
		projectFileBytes = []byte(p.PatchedConfig)
	}

	// apply remote configuration patch if needed
	if !p.IsGithubPRPatch() && p.ConfigChanged(path) && p.PatchedConfig == "" {
		project, err = model.MakePatchedConfig(ctx, p, path, string(projectFileBytes))
		if err != nil {
			return nil, errors.Wrapf(err, "Could not patch remote configuration file")
		}
		// overwrite project fields with the project ref to disallow tracking a
		// different project or doing other crazy things via config patches
		var verrs ValidationErrors
		verrs, err = CheckProjectSyntax(project)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		if len(verrs) != 0 {
			var message string
			for _, err := range verrs {
				message += fmt.Sprintf("\n\t=> %+v", err)
			}
			return nil, errors.New(message)
		}
	} else {
		// configuration is not patched
		if err = model.LoadProjectInto(projectFileBytes, projectRef.Identifier, project); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return project, nil
}
