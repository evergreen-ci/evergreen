package data

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBAliasConnector is a struct that implements the Alias related methods
// from the Connector through interactions with the backing database.
type DBAliasConnector struct{}

// FindProjectAliases queries the database to find all aliases.
// If the repoId is given, we default to repo aliases if there are no project aliases.
// If aliasesToAdd are given, then we fold those aliases in and remove any that are marked as deleted.
func (d *DBAliasConnector) FindProjectAliases(projectId, repoId string, aliasesToAdd []restModel.APIProjectAlias) ([]restModel.APIProjectAlias, error) {
	var err error
	var aliases model.ProjectAliases
	// should this logic just be folded into FindProjectAliases?
	aliasesToDelete := []string{}
	aliasModels := []restModel.APIProjectAlias{}
	for _, a := range aliasesToAdd {
		if a.Delete {
			aliasesToDelete = append(aliasesToDelete, utility.FromStringPtr(a.ID))
		} else {
			aliasModels = append(aliasModels, a)
		}
	}
	if projectId != "" {
		aliases, err = model.FindAliasesForProjectFromDb(projectId)
		if err != nil {
			return nil, err
		}
	}

	if len(aliases) == 0 && repoId != "" {
		aliases, err = model.FindAliasesForRepo(repoId)
		if err != nil {
			return nil, errors.Wrapf(err, "error finding aliases for repo '%s'", repoId)
		}
	}
	if aliases == nil {
		return nil, nil
	}
	for _, alias := range aliases {
		if utility.StringSliceContains(aliasesToDelete, alias.ID.Hex()) {
			continue
		}
		aliasModel := restModel.APIProjectAlias{}
		if err := aliasModel.BuildFromService(alias); err != nil {
			return nil, err
		}
		aliasModels = append(aliasModels, aliasModel)
	}

	return aliasModels, nil
}

// CopyProjectAliases finds the aliases for a given project and inserts them for the new project.
func (d *DBAliasConnector) CopyProjectAliases(oldProjectId, newProjectId string) error {
	aliases, err := model.FindAliasesForProjectFromDb(oldProjectId)
	if err != nil {
		return errors.Wrapf(err, "error finding aliases for project '%s'", oldProjectId)
	}
	if aliases != nil {
		if err = model.UpsertAliasesForProject(aliases, newProjectId); err != nil {
			return errors.Wrapf(err, "error inserting aliases for project '%s'", newProjectId)
		}
	}
	return nil
}

func (d *DBAliasConnector) UpdateProjectAliases(projectId string, aliases []restModel.APIProjectAlias) error {
	aliasesToUpsert := []model.ProjectAlias{}
	aliasesToDelete := []string{}
	catcher := grip.NewBasicCatcher()
	for _, aliasModel := range aliases {
		if aliasModel.Delete {
			aliasesToDelete = append(aliasesToDelete, utility.FromStringPtr(aliasModel.ID))
		} else {
			v, err := aliasModel.ToService()
			catcher.Add(errors.Wrap(err, "problem converting to project variable model"))

			alias, ok := v.(model.ProjectAlias)
			if !ok {
				catcher.Add(errors.New("problem converting to project alias"))
			}
			alias.ProjectID = projectId
			aliasesToUpsert = append(aliasesToUpsert, alias)
		}
	}
	errStrs := model.ValidateProjectAliases(aliasesToUpsert, "All Aliases")
	for _, err := range errStrs {
		catcher.Add(errors.Errorf("error validating project alias: %s", err))
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	if err := model.UpsertAliasesForProject(aliasesToUpsert, projectId); err != nil {
		return errors.Wrap(err, "problem upserting aliases")
	}
	for _, aliasId := range aliasesToDelete {
		catcher.Add(model.RemoveProjectAlias(aliasId))
	}
	return catcher.Resolve()
}

func (pc *DBAliasConnector) UpdateAliasesForSection(projectId string, updatedAliases []restModel.APIProjectAlias,
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
	if err := pc.UpdateProjectAliases(projectId, aliasesToUpdate); err != nil {
		return false, errors.Wrap(err, "error updating project aliases")
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

func shouldSkipAliasForSection(section model.ProjectPageSection, alias string) bool {
	// if we're updating internal aliases, skip non-internal aliases
	if section == model.ProjectPageGithubAndCQSection && !utility.StringSliceContains(evergreen.InternalAliases, alias) {
		return true
	}
	// if we're updating patch aliases, skip internal aliases
	if section == model.ProjectPagePatchAliasSection && utility.StringSliceContains(evergreen.InternalAliases, alias) {
		return true
	}
	return false
}

//  GetMatchingGitTagAliasesForProject returns matching git tag aliases that match the given git tag
func (d *DBAliasConnector) HasMatchingGitTagAliasAndRemotePath(projectId, tag string) (bool, string, error) {
	aliases, err := model.FindMatchingGitTagAliasesInProject(projectId, tag)
	if err != nil {
		return false, "", err
	}

	if len(aliases) == 1 && aliases[0].RemotePath != "" {
		return true, aliases[0].RemotePath, nil
	}
	return len(aliases) > 0, "", nil
}

// MockAliasConnector is a struct that implements mock versions of
// Alias-related methods for testing.
type MockAliasConnector struct {
	Aliases []restModel.APIProjectAlias
}

// FindAllAliases is a mock implementation for testing.
func (d *MockAliasConnector) FindProjectAliases(projectId, repoId string, aliasesToAdd []restModel.APIProjectAlias) ([]restModel.APIProjectAlias, error) {
	return append(d.Aliases, aliasesToAdd...), nil
}

func (d *MockAliasConnector) CopyProjectAliases(oldProjectId, newProjectId string) error {
	return nil
}

func (d *MockAliasConnector) UpdateProjectAliases(projectId string, aliases []restModel.APIProjectAlias) error {
	return nil
}

func (pc *MockAliasConnector) UpdateAliasesForSection(projectId string, updatedAliases []restModel.APIProjectAlias,
	originalAliases []model.ProjectAlias, section model.ProjectPageSection) (bool, error) {
	return false, nil
}

func (d *MockAliasConnector) HasMatchingGitTagAliasAndRemotePath(projectId, tag string) (bool, string, error) {
	if len(d.Aliases) == 1 && utility.FromStringPtr(d.Aliases[0].RemotePath) != "" {
		return true, utility.FromStringPtr(d.Aliases[0].RemotePath), nil
	}
	return len(d.Aliases) > 0, "", nil
}
