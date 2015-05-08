package validator

import (
	"encoding/base64"
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/shelman/angier"
)

func ValidateAndFinalize(p *patch.Patch, settings *evergreen.Settings) (*version.Version, error) {
	if p.Version != "" {
		return nil, fmt.Errorf("Patch %v already finalized", p.Version)
	}
	projectRef, err := model.FindOneProjectRef(p.Project)
	if err != nil {
		return nil, err
	}

	gitCommit, err := thirdparty.GetCommitEvent(
		settings.Credentials["github"],
		projectRef.Owner, projectRef.Repo, p.Githash,
	)
	if err != nil {
		return nil, fmt.Errorf("Couldn't fetch commit information: %v", err)
	}
	if gitCommit == nil {
		return nil, fmt.Errorf("Couldn't fetch commit information: git commit" +
			" doesn't exist?")
	}

	// TODO: MCI-1938
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

	var project *model.Project

	if err = model.LoadProjectInto(projectFileBytes, project); err != nil {
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
		if err = angier.TransferByFieldNames(projectRef, project); err != nil {
			return nil, fmt.Errorf("Could not merge project Ref ref into project: %v", err)
		}
		errs := CheckProjectSyntax(project)
		if len(errs) != 0 {
			var message string
			for _, err := range errs {
				message += fmt.Sprintf("\n\t=> %v", err)
			}
			return nil, fmt.Errorf(message)
		}
	}
	return model.FinalizePatch(p, gitCommit, settings, project)
}
