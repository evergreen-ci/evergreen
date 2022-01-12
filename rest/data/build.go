package data

import (
	"net/http"

	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/gimlet"
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
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("build with id %s not found", buildId),
		}
	}
	return b, nil
}

// AbortBuild wraps the service level AbortBuild
func (bc *DBBuildConnector) AbortBuild(buildId string, user string) error {
	return model.AbortBuild(buildId, user)
}

// SetBuildPriority wraps the service level method
func (bc *DBBuildConnector) SetBuildPriority(buildId string, priority int64, caller string) error {
	return model.SetBuildPriority(buildId, priority, caller)
}

// SetBuildActivated wraps the service level method
func (bc *DBBuildConnector) SetBuildActivated(buildId string, user string, activated bool) error {
	return model.SetBuildActivation(buildId, activated, user)
}

// RestartBuild wraps the service level RestartBuild
func (bc *DBBuildConnector) RestartBuild(buildId string, user string) error {
	return model.RestartAllBuildTasks(buildId, user)
}
