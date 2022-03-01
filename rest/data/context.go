package data

import (
	"github.com/evergreen-ci/evergreen/model"
)

// DBContextConnector is a struct that implements the Context related
// functions of the ServiceConnector interface through interactions
// with the backing database.
type DBContextConnector struct{}

// LoadContext fetches the context through a call to the service layer.
func (dc *DBContextConnector) FetchContext(taskId, buildId, versionId, patchId, projectId string) (model.Context, error) {
	return model.LoadContext(taskId, buildId, versionId, patchId, projectId)
}
