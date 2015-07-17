package validator

import (
	"encoding/base64"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
)

// GetPatchedProject creates and validates a project created by fetching latest commit information from GitHub
// and applying the patch to the latest remote configuration. The error returned can be a validation error.
func GetPatchedProject(p *patch.Patch, settings *evergreen.Settings) (*model.Project, error) {
	if p.Version != "" {
		return nil, fmt.Errorf("Patch %v already finalized", p.Version)
	}
	projectRef, err := model.FindOneProjectRef(p.Project)
	if err != nil {
		return nil, err
	}

	// get the remote file at the requested revision
	projectFileURL := thirdparty.GetGithubFileURL(
		projectRef.Owner,
		projectRef.Repo,
		projectRef.RemotePath,
		p.Githash,
	)

	githubFile, err := thirdparty.GetGithubFile(
		settings.Credentials["github"],
		projectFileURL,
	)
	if err != nil {
		return nil, fmt.Errorf("Could not get github file at %v: %v", projectFileURL, err)
	}

	projectFileBytes, err := base64.StdEncoding.DecodeString(githubFile.Content)
	if err != nil {
		return nil, fmt.Errorf("Could not decode github file at %v: %v", projectFileURL, err)
	}

	project := &model.Project{}

	if err = model.LoadProjectInto(projectFileBytes, projectRef.Identifier, project); err != nil {
		return nil, err
	}
	// apply remote configuration patch if needed
	if p.ConfigChanged(projectRef.RemotePath) {
		project, err = model.MakePatchedConfig(p, projectRef.RemotePath, string(projectFileBytes))
		if err != nil {
			return nil, fmt.Errorf("Could not patch remote configuration file: %v", err)
		}
		// overwrite project fields with the project ref to disallow tracking a
		// different project or doing other crazy things via config patches
		errs := CheckProjectSyntax(project)
		if len(errs) != 0 {
			var message string
			for _, err := range errs {
				message += fmt.Sprintf("\n\t=> %v", err)
			}
			return nil, fmt.Errorf(message)
		}
	}
	return project, nil
}
