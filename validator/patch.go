package validator

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/model/patch"
	"10gen.com/mci/model/version"
	"10gen.com/mci/thirdparty"
	"encoding/base64"
	"fmt"
	"github.com/shelman/angier"
)

func ValidateAndFinalize(p *patch.Patch, mciSettings *mci.MCISettings) (*version.Version, error) {
	if p.Version != "" {
		return nil, fmt.Errorf("Patch %v already finalized", p.Version)
	}

	project, err := model.FindProject("", p.Project, mciSettings.ConfigDir)
	if err != nil {
		return nil, err
	}

	gitCommit, err := thirdparty.GetCommitEvent(
		mciSettings.Credentials[project.RepoKind],
		project.Owner, project.Repo, p.Githash,
	)
	if err != nil {
		return nil, fmt.Errorf("Couldn't fetch commit information: %v", err)
	}
	if gitCommit == nil {
		return nil, fmt.Errorf("Couldn't fetch commit information: git commit" +
			" doesn't exist?")
	}
	// get the current project ref
	ref, err := model.FindOneProjectRef(project.Identifier)
	if err != nil || ref == nil {
		return nil, fmt.Errorf("Couldn't find project ref for %v: %v",
			project.Identifier, err)
	}
	// apply remote configuration patch if needed
	if project.Remote && p.ConfigChanged(project.RemotePath) {
		// TODO: MCI-1938
		// get the remote file at the requested revision
		projectFileURL := thirdparty.GetGithubFileURL(
			project.Owner,
			project.Repo,
			project.RemotePath,
			p.Githash,
		)
		githubFile, err := thirdparty.GetGithubFile(
			mciSettings.Credentials[project.RepoKind],
			projectFileURL,
		)
		if err != nil {
			return nil, fmt.Errorf("Could not get github file at %v: %v",
				projectFileURL, err)
		}
		projectFileBytes, err := base64.StdEncoding.DecodeString(githubFile.Content)
		if err != nil {
			return nil, fmt.Errorf("Could not decode github file at %v: %v",
				projectFileURL, err)
		}
		project, err = model.MakePatchedConfig(p, project.RemotePath, string(projectFileBytes))
		if err != nil {
			return nil, fmt.Errorf("Could not patch remote configuration "+
				"file: %v", err)
		}
		// overwrite project fields with the project ref to disallow tracking a
		// different project or doing other crazy things via config patches
		if err = angier.TransferByFieldNames(ref, project); err != nil {
			return nil, fmt.Errorf("Could not merge ref into project: %v", err)
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
	return model.FinalizePatch(p, gitCommit, mciSettings, project)
}
