package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// GetManifestByTask finds the version manifest corresponding to the given task.
func GetManifestByTask(taskId string) (*manifest.Manifest, error) {
	t, err := task.FindOneId(taskId)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task '%s'", t)
	}
	if t == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("manifest for task '%s' not found", taskId),
		}
	}
	mfest, err := manifest.FindFromVersion(t.Version, t.Project, t.Revision, t.Requester)
	if err != nil {
		return nil, errors.Wrapf(err, "finding manifest from version '%s'", t.Version)
	}
	if mfest == nil {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("manifest for version '%s' not found", t.Version),
		}
	}
	return mfest, nil
}
