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

func (c *hostCommunicator) GetAgentSetupData(ctx context.Context) (*apimodels.AgentSetupData, error) {
	out := &apimodels.AgentSetupData{}
	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion1,
		path:    "agent/setup",
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		err = utility.RespErrorf(resp, "failed to get agent setup info: %s", err.Error())
		grip.Alert(err)
		return nil, err
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, out); err != nil {
		return nil, errors.Wrap(err, "failed to get agent setup info")
	}
	return out, nil
}

// EndTask marks the task as finished with the given status
func (c *hostCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	grip.Info(message.Fields{
		"message":     "started EndTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	taskEndResp := &apimodels.EndTaskResponse{}
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("end")
	resp, err := c.retryRequest(ctx, info, detail)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to end task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, taskEndResp); err != nil {
		message := fmt.Sprintf("Error unmarshalling task end response: %v", err)
		return nil, errors.New(message)
	}
	grip.Info(message.Fields{
		"message":     "finished EndTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	return taskEndResp, nil
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (c *hostCommunicator) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	nextTask := &apimodels.NextTaskResponse{}
	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion1,
	}
	info.path = "agent/next_task"
	resp, err := c.retryRequest(ctx, info, details)
	if err != nil {
		err = utility.RespErrorf(resp, "failed to get next task: %s", err.Error())
		grip.Critical(err)
		return nil, err
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, nextTask); err != nil {
		err = errors.Wrap(err, "failed to read next task from response")
		return nil, err
	}
	return nextTask, nil
}

// GetCedarConfig returns the cedar service information including the base URL,
// URL, RPC port, and credentials.
func (c *hostCommunicator) GetCedarConfig(ctx context.Context) (*apimodels.CedarConfig, error) {
	cc := &apimodels.CedarConfig{}

	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion1,
		path:    "agent/cedar_config",
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		err = utility.RespErrorf(resp, "failed to get cedar config: %s", err.Error())
		grip.Critical(err)
		return nil, err
	}
	defer resp.Body.Close()

	if err = utility.ReadJSON(resp.Body, cc); err != nil {
		err = errors.Wrap(err, "reading cedar config from response")
		return nil, err
	}

	return cc, nil
}
