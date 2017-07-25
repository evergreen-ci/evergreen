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
func (bc *DBBuildConnector) FindBuildById(buildId string) (*build.Build, error) {
	b, err := build.FindOne(build.ById(buildId))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build with id %s not found", buildId),
		}
	}
	return b, nil
}

// FindProjectByBranch queries the project_refs database to find the name of the
// project a given branch falls under.
func (bc *DBBuildConnector) FindProjectByBranch(branch string) (*model.ProjectRef, error) {
	return model.FindOneProjectRef(branch)
}

// AbortBuild wraps the service level AbortBuild
func (bc *DBBuildConnector) AbortBuild(buildId string, user string) error {
	return model.AbortBuild(buildId, user)
}

// MockBuildConnector is a struct that implements the Build related methods
// from the Connector through interactions with the backing database.
type MockBuildConnector struct {
	CachedBuilds   []build.Build
	CachedProjects map[string]*model.ProjectRef
	CachedAborted  map[string]string
}

// FindBuildById iterates through the CachedBuilds slice to find the build
// with matching id field.
func (bc *MockBuildConnector) FindBuildById(buildId string) (*build.Build, error) {
	for _, b := range bc.CachedBuilds {
		if b.Id == buildId {
			return &b, nil
		}
	}
	return nil, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("build with id %s not found", buildId),
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

// AbortBuild sets the value of the input build Id in CachedAborted to true.
func (bc *MockBuildConnector) AbortBuild(buildId string, user string) error {
	bc.CachedAborted[buildId] = user
	return nil
}
