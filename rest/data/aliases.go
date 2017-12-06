package data

import (
	"github.com/evergreen-ci/evergreen/model"
)

// DBAliasConnector is a struct that implements the Alias related methods
// from the Connector through interactions with the backing database.
type DBAliasConnector struct{}

// FindProjectAliases queries the database to find all aliases.
func (d *DBAliasConnector) FindProjectAliases(projectId string) ([]model.PatchDefinition, error) {
	aliases, err := model.FindAllProjectAliases(projectId)
	if err != nil {
		return nil, err
	}
	if aliases == nil {
		return nil, nil
	}
	return aliases, nil
}

// MockAliasConnector is a struct that implements mock versions of
// Alias-related methods for testing.
type MockAliasConnector struct{}

// FindAllAliases is a mock implementation for testing.
func (d *MockAliasConnector) FindProjectAliases(projectId string) ([]model.PatchDefinition, error) {
	return nil, nil
}
