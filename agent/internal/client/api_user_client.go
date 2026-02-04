package client

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/pkg/errors"
)

type debugCommunicator struct {
	baseCommunicator
	apiUser string
	apiKey  string
}

// NewDebugCommunicator initializes a communicator that will be used for basic agent routes required
// for executing tasks in debug mode.
func NewDebugCommunicator(serverURL, apiUser, apiKey string) Communicator {
	c := &debugCommunicator{
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

// EndTask no-ops in debug mode.
func (c *debugCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	return &apimodels.EndTaskResponse{}, nil
}

// GetNextTask no-ops in debug mode.
func (c *debugCommunicator) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	return nil, errors.New("GetNextTask not implemented for API user communicator")
}
