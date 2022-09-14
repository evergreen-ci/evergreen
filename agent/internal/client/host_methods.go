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
		method: http.MethodGet,
		path:   "agent/setup",
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
		path:     fmt.Sprintf("hosts/%s/task/%s/end", c.hostID, taskData.ID),
	}
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
		method: http.MethodGet,
	}
	info.path = fmt.Sprintf("hosts/%s/agent/next_task", c.hostID)
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
