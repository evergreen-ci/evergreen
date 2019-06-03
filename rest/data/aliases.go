package data

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

// DBAliasConnector is a struct that implements the Alias related methods
// from the Connector through interactions with the backing database.
type DBAliasConnector struct{}

// FindProjectAliases queries the database to find all aliases.
func (d *DBAliasConnector) FindProjectAliases(projectId string) ([]model.ProjectAlias, error) {
	aliases, err := model.FindAliasesForProject(projectId)
	if err != nil {
		return nil, err
	}
	if aliases == nil {
		return nil, nil
	}
	return aliases, nil
}

// CopyProjectAliases finds the aliases for a given project and inserts them for the new project.
func (d *DBAliasConnector) CopyProjectAliases(oldProjectId, newProjectId string) error {
	aliases, err := d.FindProjectAliases(oldProjectId)
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

// MockAliasConnector is a struct that implements mock versions of
// Alias-related methods for testing.
type MockAliasConnector struct{}

// FindAllAliases is a mock implementation for testing.
func (d *MockAliasConnector) FindProjectAliases(projectId string) ([]model.ProjectAlias, error) {
	return nil, nil
}

func (d *MockAliasConnector) CopyProjectAliases(oldProjectId, newProjectId string) error {
	return nil
}
