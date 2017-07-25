package data

import (
	"net/http"

	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/pkg/errors"
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

// SetBuildPriority wraps the service level method
func (bc *DBBuildConnector) SetBuildPriority(buildId string, priority int64) error {
	return model.SetBuildPriority(buildId, priority)
}

// SetBuildActivated wraps the service level method
func (bc *DBBuildConnector) SetBuildActivated(buildId string, user string, activated bool) error {
	return model.SetBuildActivation(buildId, activated, user)
}

// MockBuildConnector is a struct that implements the Build related methods
// from the Connector through interactions with the backing database.
type MockBuildConnector struct {
	CachedBuilds         []build.Build
	CachedProjects       map[string]*model.ProjectRef
	CachedAborted        map[string]string
	FailOnChangePriority bool
	FailOnAbort          bool
}

// FindBuildById iterates through the CachedBuilds slice to find the build
// with matching id field.
func (bc *MockBuildConnector) FindBuildById(buildId string) (*build.Build, error) {
	for idx := range bc.CachedBuilds {
		if bc.CachedBuilds[idx].Id == buildId {
			return &bc.CachedBuilds[idx], nil
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
	if bc.FailOnAbort {
		return errors.New("manufactured fail")
	}
	bc.CachedAborted[buildId] = user
	return nil
}

// SetBuildPriority throws an error if instructed
func (bc *MockBuildConnector) SetBuildPriority(buildId string, priority int64) error {
	if bc.FailOnChangePriority {
		return errors.New("manufactured fail")
	}
	return nil
}

// SetBuildActivated sets the activated and activated_by fields on the input build.
func (bc *MockBuildConnector) SetBuildActivated(buildId string, user string, activated bool) error {
	b, err := bc.FindBuildById(buildId)
	if err != nil {
		return err
	}
	b.Activated = activated
	b.ActivatedBy = user
	return nil
}
