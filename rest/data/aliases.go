package data

import (
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DBAliasConnector is a struct that implements the Alias related methods
// from the Connector through interactions with the backing database.
type DBAliasConnector struct{}

// FindProjectAliases queries the database to find all aliases.
func (d *DBAliasConnector) FindProjectAliases(projectId string) ([]restModel.APIProjectAlias, error) {
	aliases, err := model.FindAliasesForProject(projectId)
	if err != nil {
		return nil, err
	}
	if aliases == nil {
		return nil, nil
	}
	aliasModels := []restModel.APIProjectAlias{}
	for _, alias := range aliases {
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
	aliases, err := model.FindAliasesForProject(oldProjectId)
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
			aliasesToDelete = append(aliasesToDelete, restModel.FromStringPtr(aliasModel.ID))
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

// MockAliasConnector is a struct that implements mock versions of
// Alias-related methods for testing.
type MockAliasConnector struct {
	Aliases []restModel.APIProjectAlias
}

// FindAllAliases is a mock implementation for testing.
func (d *MockAliasConnector) FindProjectAliases(projectId string) ([]restModel.APIProjectAlias, error) {
	return d.Aliases, nil
}

func (d *MockAliasConnector) CopyProjectAliases(oldProjectId, newProjectId string) error {
	return nil
}

func (d *MockAliasConnector) UpdateProjectAliases(projectId string, aliases []restModel.APIProjectAlias) error {
	return nil
}
