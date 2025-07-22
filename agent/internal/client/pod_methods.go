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

// EndTask marks the task as finished with the given status.
func (c *podCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		path:     fmt.Sprintf("pods/%s/task/%s/end", c.podID, taskData.ID),
	}
	resp, err := c.retryRequest(ctx, info, detail)
	if err != nil {
		return nil, util.RespError(resp, errors.Wrap(err, "ending task").Error())
	}

	var taskEndResp apimodels.EndTaskResponse
	if err = utility.ReadJSON(resp.Body, &taskEndResp); err != nil {
		return nil, errors.Wrap(err, "reading end task response")
	}
	grip.Info(message.Fields{
		"message": "finished EndTask",
		"task_id": taskData.ID,
	})
	return &taskEndResp, nil
}

// GetNextTask returns information about the next task to run, or other
// miscellaneous actions to take in between tasks.
func (c *podCommunicator) GetNextTask(ctx context.Context, _ *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	info := requestInfo{
		method: http.MethodGet,
		path:   fmt.Sprintf("pods/%s/agent/next_task", c.podID),
	}
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, util.RespError(resp, errors.Wrap(err, "getting next task").Error())
	}

	var nextTask apimodels.NextTaskResponse
	if err := utility.ReadJSON(resp.Body, &nextTask); err != nil {
		return nil, errors.Wrap(err, "reading next task reply from response")
	}

	return &nextTask, nil
}
