package data

import (
	"net/http"

	"fmt"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/gimlet"
)

// FindBuildById uses the service layer's build type to query the backing database for
// a specific build.
func FindBuildById(buildId string) (*build.Build, error) {
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
