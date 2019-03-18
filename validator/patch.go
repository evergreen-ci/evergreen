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
func GetPatchedProject(ctx context.Context, p *patch.Patch, githubOauthToken string) ([]byte, error) {
	if p.Version != "" {
		return nil, errors.Errorf("Patch %v already finalized", p.Version)
	}

	// if the patched config exists, use that as the project file bytes.
	if p.PatchedConfig != "" {
		return []byte(p.PatchedConfig), nil
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
	if p.IsGithubPRPatch() {
		hash = p.GithubPatchData.HeadHash
	}
	if p.IsPRMergePatch() {
		hash = p.GithubPatchData.MergeCommitSHA
	}

	githubFile, err := thirdparty.GetGithubFile(ctx, githubOauthToken, projectRef.Owner,
		projectRef.Repo, projectRef.RemotePath, hash)
	if err != nil {
		// if the project file doesn't exist, but our patch includes a project file,
		// we try to apply the diff and proceed.
		if !(p.ConfigChanged(projectRef.RemotePath) && thirdparty.IsFileNotFound(err)) {
			// return an error if the github error is network/auth-related or we aren't patching the config
			return nil, errors.Wrapf(err, "Could not get github file at '%s/%s'@%s: %s", projectRef.Owner,
				projectRef.Repo, projectRef.RemotePath, hash)
		}
	} else {
		// we successfully got the project file in base64, so we decode it
		projectFileBytes, err = base64.StdEncoding.DecodeString(*githubFile.Content)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not decode github file at '%s/%s'@%s: %s", projectRef.Owner,
				projectRef.Repo, projectRef.RemotePath, hash)
		}
	}

	// apply remote configuration patch if needed
	if !(p.IsGithubPRPatch() || p.IsPRMergePatch()) && p.ConfigChanged(projectRef.RemotePath) {
		projectFileBytes, err = model.MakePatchedConfig(ctx, p, projectRef.RemotePath, string(projectFileBytes))
		if err != nil {
			return nil, errors.Wrapf(err, "Could not patch remote configuration file")
		}
	}

	return projectFileBytes, nil
}

func ValidateProjectPatch(data []byte, projectID string) (*model.Project, bool, error) {
	project := &model.Project{}
	if err := model.LoadProjectInto(data, projectID, project); err != nil {
		return nil, true, errors.WithStack(err)
	}

	var verrs ValidationErrors
	verrs, err := CheckProjectSyntax(project)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}

	if len(verrs) != 0 {
		var message string
		for _, err := range verrs {
			message += fmt.Sprintf("\n\t=> %+v", err)
		}
		return nil, true, errors.New(message)
	}

	return project, false, nil
}
