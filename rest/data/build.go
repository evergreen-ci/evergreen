package data

import (
	"net/http"

	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/rest"
)

// DBBuildConnector is a struct that implements the Build related methods
// from the Connector through interactions with the backing database.
type DBBuildConnector struct{}

// FindBuildById uses the service layer's build type to query the backing database for
// a specific build.
func (bc *DBBuildConnector) FindBuildById(id string) (*build.Build, error) {
	b, err := build.FindOne(build.ById(id))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build with id %s not found", id),
		}
	}
	return b, nil
}

// FindProjectByBranch queries the project_refs database to find the name of the
// project a given branch falls under.
func (bc *DBBuildConnector) FindProjectByBranch(branch string) (*model.ProjectRef, error) {
	return model.FindOneProjectRef(branch)
}

// MockBuildConnector is a struct that implements the Build related methods
// from the Connector through interactions with the backing database.
type MockBuildConnector struct {
	CachedBuilds   []build.Build
	CachedProjects map[string]*model.ProjectRef
}

// FindBuildById iterates through the CachedBuilds slice to find the build
// with matching id field.
func (bc *MockBuildConnector) FindBuildById(id string) (*build.Build, error) {
	for _, b := range bc.CachedBuilds {
		if b.Id == id {
			return &b, nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("build with id %s not found", id),
	}
}

// FindProjectByBranch accesses the map of branch names to project names to find
// the project corresponding to a given branch.
func (bc *MockBuildConnector) FindProjectByBranch(branch string) (*model.ProjectRef, error) {
	proj, ok := bc.CachedProjects[branch]
	if !ok {
		return nil, nil
	}
	return proj, nil
}
