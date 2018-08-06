package data

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
)

// DBPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with the backing database.
type DBProjectConnector struct{}

// FindProjects queries the backing database for the specified projects
func (pc *DBProjectConnector) FindProjects(key string, limit int, sortDir int, isAuthenticated bool) ([]model.ProjectRef, error) {
	projects, err := model.FindProjectRefs(key, limit, sortDir, isAuthenticated)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching projects starting at project '%s'", key)
	}

	return projects, nil
}

// MockPatchConnector is a struct that implements the Patch related methods
// from the Connector through interactions with he backing database.
type MockProjectConnector struct {
	CachedProjects []model.ProjectRef
	CachedVars     []*model.ProjectVars
}

// FindProjects queries the cached projects slice for the matching projects.
// Assumes CachedProjects is sorted in alphabetical order of project identifier.
func (pc *MockProjectConnector) FindProjects(key string, limit int, sortDir int, isAuthenticated bool) ([]model.ProjectRef, error) {
	projects := []model.ProjectRef{}
	if sortDir > 0 {
		for i := 0; i < len(pc.CachedProjects); i++ {
			p := pc.CachedProjects[i]
			visible := isAuthenticated || (!isAuthenticated && !p.Private)
			if p.Identifier >= key && visible {
				projects = append(projects, p)
				if len(projects) == limit {
					break
				}
			}
		}
	} else {
		for i := len(pc.CachedProjects) - 1; i >= 0; i-- {
			p := pc.CachedProjects[i]
			visible := isAuthenticated || (!isAuthenticated && !p.Private)
			if p.Identifier < key && visible {
				projects = append(projects, p)
				if len(projects) == limit {
					break
				}
			}
		}
	}
	return projects, nil
}
