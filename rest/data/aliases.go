package data

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// FindMergedProjectAliases returns a merged list of aliases, with the order of precedence being:
// 1. aliases defined on the project page
// 2. aliases defined on the repo page
// 3. aliases defined in the project config YAML
// The includeProjectConfig flag determines whether to include aliases defined in the project config YAML.
// If the aliasesToAdd parameter is defined, we fold those aliases in and remove any that are marked as deleted.
func FindMergedProjectAliases(projectId, repoId string, aliasesToAdd []restModel.APIProjectAlias, includeProjectConfig bool) ([]restModel.APIProjectAlias, error) {
	projectRef, err := model.FindBranchProjectRef(projectId)
	if err != nil {
		return nil, errors.Wrapf(err, "finding project ref for project '%s'", projectId)
	}
	var projectConfig *model.ProjectConfig
	if includeProjectConfig {
		projectConfig, err = model.FindLastKnownGoodProjectConfig(projectId)
		if err != nil {
			return nil, errors.Wrapf(err, "finding project config for project '%s'", projectId)
		}
	}
	aliases, err := model.ConstructMergedAliasesByPrecedence(projectRef, projectConfig, repoId)
	if err != nil {
		return nil, errors.Wrapf(err, "finding merged aliases for project '%s'", projectId)
	}
	if aliases == nil {
		return nil, nil
	}
	aliasesToDelete := []string{}
	aliasModels := []restModel.APIProjectAlias{}
	for _, a := range aliasesToAdd {
		if a.Delete {
			aliasesToDelete = append(aliasesToDelete, utility.FromStringPtr(a.ID))
		} else {
			aliasModels = append(aliasModels, a)
		}
	}
	for _, alias := range aliases {
		if utility.StringSliceContains(aliasesToDelete, alias.ID.Hex()) {
			continue
		}
		aliasModel := restModel.APIProjectAlias{}
		aliasModel.BuildFromService(alias)
		aliasModels = append(aliasModels, aliasModel)
	}

	return aliasModels, nil
}

// UpdateProjectAliases upserts/deletes aliases for the given project
func UpdateProjectAliases(projectId string, aliases []restModel.APIProjectAlias) error {
	aliasesToUpsert := []model.ProjectAlias{}
	aliasesToDelete := []string{}
	catcher := grip.NewBasicCatcher()
	for _, aliasModel := range aliases {
		if aliasModel.Delete {
			aliasesToDelete = append(aliasesToDelete, utility.FromStringPtr(aliasModel.ID))
		} else {
			alias := aliasModel.ToService()
			alias.ProjectID = projectId
			aliasesToUpsert = append(aliasesToUpsert, alias)
		}
	}
	errStrs := model.ValidateProjectAliases(aliasesToUpsert, "All Aliases")
	for _, err := range errStrs {
		catcher.Wrap(errors.New(err), "invalid project alias")
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if err := model.UpsertAliasesForProject(aliasesToUpsert, projectId); err != nil {
		return errors.Wrap(err, "upserting project aliases")
	}
	for _, aliasId := range aliasesToDelete {
		catcher.Wrapf(model.RemoveProjectAlias(aliasId), "deleting project alias '%s'", aliasId)
	}
	return catcher.Resolve()
}

// updateAliasesForSection, given a project, a list of current aliases, a list of previous aliases, and a project page section,
// upserts any current aliases, and deletes any aliases that existed previously but not anymore (only
// considers the aliases that are relevant for the section). Returns if any aliases have been modified.
func updateAliasesForSection(projectId string, updatedAliases []restModel.APIProjectAlias,
	originalAliases []model.ProjectAlias, section model.ProjectPageSection) (bool, error) {
	aliasesIdMap := map[string]bool{}
	aliasesToUpdate := []restModel.APIProjectAlias{}

	for _, a := range updatedAliases {
		if shouldSkipAliasForSection(section, utility.FromStringPtr(a.Alias)) {
			continue
		}
		aliasesToUpdate = append(aliasesToUpdate, a)
		aliasesIdMap[utility.FromStringPtr(a.ID)] = true
	}
	if err := UpdateProjectAliases(projectId, aliasesToUpdate); err != nil {
		return false, errors.Wrap(err, "updating project aliases")
	}
	modified := len(aliasesToUpdate) > 0
	catcher := grip.NewBasicCatcher()
	// delete any aliasesToUpdate that were in the list before but are not now
	for _, originalAlias := range originalAliases {
		// only look at the relevant aliases to update
		if shouldSkipAliasForSection(section, originalAlias.Alias) {
			continue
		}
		id := originalAlias.ID.Hex()
		if _, ok := aliasesIdMap[id]; !ok {
			catcher.Add(model.RemoveProjectAlias(id))
			modified = true
		}
	}
	return modified, catcher.Resolve()
}

// validateFeaturesHaveAliases returns an error if project/repo aliases are not defined for a Github/CQ feature.
// Does not error if version control is enabled. To check for version control, we pass in the original project ref
// along with the newly changed project ref because the new project ref only contains github / CQ section data.
func validateFeaturesHaveAliases(originalProjectRef *model.ProjectRef, newProjectRef *model.ProjectRef, aliases []restModel.APIProjectAlias) error {
	if originalProjectRef.IsVersionControlEnabled() {
		return nil
	}

	aliasesMap := map[string]bool{}
	for _, a := range aliases {
		aliasesMap[utility.FromStringPtr(a.Alias)] = true
	}

	if newProjectRef.UseRepoSettings() {
		repoAliases, err := model.FindAliasesForRepo(newProjectRef.RepoRefId)
		if err != nil {
			return err
		}
		for _, a := range repoAliases {
			aliasesMap[a.Alias] = true
		}

	}

	msg := "%s cannot be enabled without aliases"
	catcher := grip.NewBasicCatcher()
	if newProjectRef.IsPRTestingEnabled() && !aliasesMap[evergreen.GithubPRAlias] {
		catcher.Errorf(msg, "PR testing")
	}
	if newProjectRef.CommitQueue.IsEnabled() && !aliasesMap[evergreen.CommitQueueAlias] {
		catcher.Errorf(msg, "Commit queue")
	}
	if newProjectRef.IsGitTagVersionsEnabled() && !aliasesMap[evergreen.GitTagAlias] {
		catcher.Errorf(msg, "Git tag versions")
	}
	if newProjectRef.IsGithubChecksEnabled() && !aliasesMap[evergreen.GithubChecksAlias] {
		catcher.Errorf(msg, "GitHub checks")
	}

	return catcher.Resolve()
}

func shouldSkipAliasForSection(section model.ProjectPageSection, alias string) bool {
	// if we're updating internal aliases, skip non-internal aliases
	if section == model.ProjectPageGithubAndCQSection && model.IsPatchAlias(alias) {
		return true
	}
	// if we're updating patch aliases, skip internal aliases
	if section == model.ProjectPagePatchAliasSection && !model.IsPatchAlias(alias) {
		return true
	}
	return false
}
