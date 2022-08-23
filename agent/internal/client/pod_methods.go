package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func (c *podCommunicator) GetAgentSetupData(ctx context.Context) (*apimodels.AgentSetupData, error) {
	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion2,
		path:    fmt.Sprintf("pods/%s/agent/setup", c.podID),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "getting agent setup data: %s", err.Error())
	}

	var data apimodels.AgentSetupData
	if err := utility.ReadJSON(resp.Body, data); err != nil {
		return nil, errors.Wrap(err, "reading agent setup data from response")
	}

	return &data, nil
}

// EndTask marks the task as finished with the given status
func (c *podCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion2,
		path:     fmt.Sprintf("pods/%s/task/%s/end", c.podID, taskData.ID),
	}
	resp, err := c.retryRequest(ctx, info, detail)
	if err != nil {
		return nil, utility.RespErrorf(resp, "ending task '%s': %s", taskData.ID, err.Error())
	}
	var taskEndResp apimodels.EndTaskResponse
	if err = utility.ReadJSON(resp.Body, &taskEndResp); err != nil {
		return nil, errors.Wrap(err, "reading end task response")
	}
	grip.Info(message.Fields{
		"message":     "finished EndTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	return &taskEndResp, nil
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (c *podCommunicator) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion2,
		path:    fmt.Sprintf("pods/%s/agent/next_task", c.podID),
	}
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "getting next task: %s", err.Error())
	}

	var nextTask apimodels.NextTaskResponse
	if err := utility.ReadJSON(resp.Body, &nextTask); err != nil {
		return nil, errors.Wrap(err, "reading next task from response")
	}

	return &nextTask, nil
}
