package client

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/pkg/errors"
)

// APIUserCommunicator implements Communicator and makes requests to API endpoints
// using API user authentication instead of task secrets. This is primarily used
// for the local debugger to communicate with the backend.
// We export this type so it can be type-asserted in the local executor.
type APIUserCommunicator struct {
	baseCommunicator
	apiUser string
	apiKey  string
}

// NewAPIUserCommunicator returns a Communicator that uses API user authentication
// for making HTTP REST requests against the API server. This is used by the local
// debugger to communicate with the backend without requiring task secrets.
func NewAPIUserCommunicator(serverURL, apiUser, apiKey string) Communicator {
	c := &APIUserCommunicator{
		baseCommunicator: newBaseCommunicator(serverURL, map[string]string{
			evergreen.APIUserHeader: apiUser,
			evergreen.APIKeyHeader:  apiKey,
		}),
		apiUser: apiUser,
		apiKey:  apiKey,
	}

	c.resetClient()

	return c
}

// EndTask marks the task as finished with the given status.
func (c *APIUserCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	// For local execution, we don't need to actually end a task on the server
	return &apimodels.EndTaskResponse{}, nil
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (c *APIUserCommunicator) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	// Not needed for local execution
	return nil, errors.New("GetNextTask not implemented for API user communicator")
}
