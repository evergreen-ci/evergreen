package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// EndTask marks the task as finished with the given status
func (c *hostCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	grip.Info(message.Fields{
		"message": "started EndTask",
		"task_id": taskData.ID,
	})
	taskEndResp := &apimodels.EndTaskResponse{}
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		path:     fmt.Sprintf("hosts/%s/task/%s/end", c.hostID, taskData.ID),
	}
	resp, err := c.retryRequest(ctx, info, detail)
	if err != nil {
		return nil, util.RespErrorf(resp, errors.Wrap(err, "ending task").Error())
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, taskEndResp); err != nil {
		return nil, errors.Wrap(err, "reading end task reply from response")
	}
	grip.Info(message.Fields{
		"message": "finished EndTask",
		"task_id": taskData.ID,
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
		return nil, util.RespErrorf(resp, errors.Wrap(err, "getting next task").Error())
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, nextTask); err != nil {
		return nil, errors.Wrap(err, "reading next task reply from response")
	}
	return nextTask, nil
}
