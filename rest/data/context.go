package data

import (
	"github.com/evergreen-ci/evergreen/model"
)

// LoadContext fetches the context through a call to the service layer.
func FetchContext(taskId, buildId, versionId, patchId, projectId string) (model.Context, error) {
	return model.LoadContext(taskId, buildId, versionId, patchId, projectId)
}
