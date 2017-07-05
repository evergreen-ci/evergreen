package client

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// StartTask marks the task as started.
func (c *evergreenREST) StartTask(ctx context.Context, taskData TaskData) error {
	pidStr := strconv.Itoa(os.Getpid())
	taskStartRequest := &apimodels.TaskStartRequest{Pid: pidStr}
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("start", taskData.ID), taskData.Secret, v1, taskStartRequest)
	if err != nil {
		err = errors.Wrapf(err, "failed to start task %s", taskData.ID)
		grip.Error(err)
		return err
	}
	defer resp.Body.Close()
	return nil
}

// EndTask marks the task as finished with the given status
func (c *evergreenREST) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	taskEndResp := &apimodels.EndTaskResponse{}
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("end", taskData.ID), taskData.Secret, v1, detail)
	if err != nil {
		err = errors.Wrapf(err, "failed to end task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	if err = util.ReadJSONInto(resp.Body, taskEndResp); err != nil {
		message := fmt.Sprintf("Error unmarshalling task end response: %v", err)
		grip.Error(message)
		return nil, errors.New(message)
	}
	grip.Infof("task's end response received: %s", taskEndResp.Message)
	return taskEndResp, nil
}

// GetTask returns the active task.
func (c *evergreenREST) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	task := &task.Task{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("", taskData.ID), taskData.Secret, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, task); err != nil {
		err = errors.Wrapf(err, "failed reading json for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	return task, nil
}

// GetProjectRef loads the task's project.
func (c *evergreenREST) GetProjectRef(ctx context.Context, taskData TaskData) (*model.ProjectRef, error) {
	projectRef := &model.ProjectRef{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("project_ref", taskData.ID), taskData.Secret, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get project ref for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, projectRef); err != nil {
		err = errors.Wrapf(err, "failed reading json for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	return projectRef, nil
}

// GetDistro returns the distro for the task.
func (c *evergreenREST) GetDistro(ctx context.Context, taskData TaskData) (*distro.Distro, error) {
	d := &distro.Distro{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("distro", taskData.ID), taskData.Secret, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get distro for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, d); err != nil {
		err = errors.Wrapf(err, "unable to read distro response for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	return d, nil
}

// GetVersion loads the task's version.
func (c *evergreenREST) GetVersion(ctx context.Context, taskData TaskData) (*version.Version, error) {
	v := &version.Version{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("version", taskData.ID), taskData.Secret, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get version for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	err = util.ReadJSONInto(resp.Body, v)
	if err != nil {
		err = errors.Wrapf(err, "unable to read project version response for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	return v, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// an "abort" response. This function returns true if the agent should abort.
func (c *evergreenREST) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	grip.Info("Sending heartbeat")
	data := interface{}("heartbeat")
	ctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	resp, err := c.post(ctx, c.getTaskPathSuffix("heartbeat", taskData.ID), taskData.Secret, v1, &data)
	if err != nil {
		err = errors.Wrapf(err, "error sending heartbeat for task %s", taskData.ID)
		grip.Error(err)
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		grip.Error("wrong secret (409) sending heartbeat")
		return false, errors.Errorf("unauthorized - wrong secret")
	}
	if resp.StatusCode != http.StatusOK {
		return false, errors.Errorf("unexpected status code doing heartbeat: %v",
			resp.StatusCode)
	}

	heartbeatResponse := &apimodels.HeartbeatResponse{}
	if err = util.ReadJSONInto(resp.Body, heartbeatResponse); err != nil {
		err = errors.Wrapf(err, "Error unmarshaling heartbeat response for task %s", taskData.ID)
		grip.Error(err)
		return false, err
	}
	return heartbeatResponse.Abort, nil
}

// FetchExpansionVars loads expansions for a communicator's task from the API server.
func (c *evergreenREST) FetchExpansionVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionVars, error) {
	resultVars := &apimodels.ExpansionVars{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("fetch_vars", taskData.ID), taskData.Secret, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get task for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		err = errors.Errorf("fetching expansions failed: got 'unauthorized' response.")
		grip.Error(err)
		return nil, err
	}
	if err = util.ReadJSONInto(resp.Body, resultVars); err != nil {
		err = errors.Wrapf(err, "failed to read vars from response for task %s", taskData.ID)
		grip.Error(err)
		return nil, err
	}
	return resultVars, err
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (c *evergreenREST) GetNextTask(ctx context.Context) (*apimodels.NextTaskResponse, error) {
	taskResponse := &apimodels.NextTaskResponse{}
	resp, err := c.retryGet(ctx, "agent/next_task", "", v1)
	if err != nil {
		err = errors.Wrap(err, "failed to get task")
		grip.Error(err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict - wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, taskResponse); err != nil {
		err = errors.Wrap(err, "failed to read next task from response")
		grip.Error(err)
		return nil, err
	}
	return taskResponse, nil

}

// SendTaskLogMessages posts a group of log messages for a task.
func (c *evergreenREST) SendTaskLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	payload := apimodels.TaskLog{
		TaskId:       taskData.ID,
		Timestamp:    time.Now(),
		MessageCount: len(msgs),
		Messages:     msgs,
	}

	if _, err := c.retryPost(ctx, c.getTaskPathSuffix("log", taskData.ID), taskData.Secret, v1, &payload); err != nil {
		err = errors.Wrapf(err, "problem sending %s log messages for task %s", len(msgs), taskData.ID)
		grip.Error(err)
		return err
	}

	return nil
}
