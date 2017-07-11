package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// StartTask marks the task as started.
func (c *communicatorImpl) StartTask(ctx context.Context, taskData TaskData) error {
	pidStr := strconv.Itoa(os.Getpid())
	taskStartRequest := &apimodels.TaskStartRequest{Pid: pidStr}
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("start", taskData.ID), taskData, v1, taskStartRequest)
	if err != nil {
		err = errors.Wrapf(err, "failed to start task %s", taskData.ID)
		return err
	}
	defer resp.Body.Close()
	return nil
}

// EndTask marks the task as finished with the given status
func (c *communicatorImpl) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	taskEndResp := &apimodels.EndTaskResponse{}
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("end", taskData.ID), taskData, v1, detail)
	if err != nil {
		err = errors.Wrapf(err, "failed to end task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()
	if err = util.ReadJSONInto(resp.Body, taskEndResp); err != nil {
		message := fmt.Sprintf("Error unmarshalling task end response: %v", err)
		return nil, errors.New(message)
	}
	grip.Infof("task's end response received: %s", taskEndResp.Message)
	return taskEndResp, nil
}

// GetTask returns the active task.
func (c *communicatorImpl) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	task := &task.Task{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("", taskData.ID), taskData, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, task); err != nil {
		err = errors.Wrapf(err, "failed reading json for task %s", taskData.ID)
		return nil, err
	}
	return task, nil
}

// GetProjectRef loads the task's project.
func (c *communicatorImpl) GetProjectRef(ctx context.Context, taskData TaskData) (*model.ProjectRef, error) {
	projectRef := &model.ProjectRef{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("project_ref", taskData.ID), taskData, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get project ref for task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, projectRef); err != nil {
		err = errors.Wrapf(err, "failed reading json for task %s", taskData.ID)
		return nil, err
	}
	return projectRef, nil
}

// GetDistro returns the distro for the task.
func (c *communicatorImpl) GetDistro(ctx context.Context, taskData TaskData) (*distro.Distro, error) {
	d := &distro.Distro{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("distro", taskData.ID), taskData, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get distro for task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, d); err != nil {
		err = errors.Wrapf(err, "unable to read distro response for task %s", taskData.ID)
		return nil, err
	}
	return d, nil
}

// GetVersion loads the task's version.
func (c *communicatorImpl) GetVersion(ctx context.Context, taskData TaskData) (*version.Version, error) {
	v := &version.Version{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("version", taskData.ID), taskData, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get version for task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	err = util.ReadJSONInto(resp.Body, v)
	if err != nil {
		err = errors.Wrapf(err, "unable to read project version response for task %s", taskData.ID)
		return nil, err
	}
	return v, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// an "abort" response. This function returns true if the agent should abort.
func (c *communicatorImpl) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	grip.Info("Sending heartbeat")
	data := interface{}("heartbeat")
	ctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	resp, err := c.post(ctx, c.getTaskPathSuffix("heartbeat", taskData.ID), taskData, v1, &data)
	if err != nil {
		err = errors.Wrapf(err, "error sending heartbeat for task %s", taskData.ID)
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return false, errors.Errorf("unauthorized - wrong secret")
	}
	if resp.StatusCode != http.StatusOK {
		return false, errors.Errorf("unexpected status code doing heartbeat: %v",
			resp.StatusCode)
	}

	heartbeatResponse := &apimodels.HeartbeatResponse{}
	if err = util.ReadJSONInto(resp.Body, heartbeatResponse); err != nil {
		err = errors.Wrapf(err, "Error unmarshaling heartbeat response for task %s", taskData.ID)
		return false, err
	}
	return heartbeatResponse.Abort, nil
}

// FetchExpansionVars loads expansions for a communicator's task from the API server.
func (c *communicatorImpl) FetchExpansionVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionVars, error) {
	resultVars := &apimodels.ExpansionVars{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("fetch_vars", taskData.ID), taskData, v1)
	if err != nil {
		err = errors.Wrapf(err, "failed to get task for task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		err = errors.Errorf("fetching expansions failed: got 'unauthorized' response.")
		return nil, err
	}
	if err = util.ReadJSONInto(resp.Body, resultVars); err != nil {
		err = errors.Wrapf(err, "failed to read vars from response for task %s", taskData.ID)
		return nil, err
	}
	return resultVars, err
}

// GetNextTask returns a next task response by getting the next task for a given host.
func (c *communicatorImpl) GetNextTask(ctx context.Context) (*apimodels.NextTaskResponse, error) {
	taskResponse := &apimodels.NextTaskResponse{}
	taskData := TaskData{OverrideValidation: true}
	resp, err := c.retryGet(ctx, "agent/next_task", taskData, v1)
	if err != nil {
		err = errors.Wrap(err, "failed to get task")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict - wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, taskResponse); err != nil {
		err = errors.Wrap(err, "failed to read next task from response")
		return nil, err
	}
	return taskResponse, nil

}

// SendTaskLogMessages posts a group of log messages for a task.
func (c *communicatorImpl) SendTaskLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	payload := apimodels.TaskLog{
		TaskId:       taskData.ID,
		Timestamp:    time.Now(),
		MessageCount: len(msgs),
		Messages:     msgs,
	}

	if _, err := c.retryPost(ctx, c.getTaskPathSuffix("log", taskData.ID), taskData, v1, &payload); err != nil {
		err = errors.Wrapf(err, "problem sending %s log messages for task %s", len(msgs), taskData.ID)
		return err
	}

	return nil
}

// SendTaskResults posts a task's results, used by the attach results operations.
func (c *communicatorImpl) SendTaskResults(ctx context.Context, td TaskData, r *task.TestResults) error {
	if r == nil || len(r.Results) == 0 {
		return nil
	}

	if _, err := c.retryPost(ctx, c.getTaskPathSuffix("results", td.ID), td, v1, r); err != nil {
		err = errors.Wrapf(err, "problem adding %d results to task %s", len(r.Results), td.ID)
		return err
	}

	return nil
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure.
func (c *communicatorImpl) GetTaskPatch(ctx context.Context, td TaskData) (*patch.Patch, error) {
	patch := patch.Patch{}
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("patch", td.ID), td, v1)

	if err != nil {
		return nil, errors.Wrapf(err, "could not get patch for %s", td.ID)
	}

	if err = util.ReadJSONInto(resp.Body, &patch); err != nil {
		return nil, errors.Wrapf(err, "problem parsing patch response for %s", td.ID)
	}

	return &patch, nil
}

// GetPatchFiles is used by the git.get_project plugin and fetches
// patches from the database, used in patch builds.
func (c *communicatorImpl) GetPatchFile(ctx context.Context, td TaskData, patchFileID string) (string, error) {
	resp, err := c.retryGet(ctx, c.getTaskPathSuffix("patch/patchfile", td.ID), td, v1)
	if err != nil {
		return "", errors.Wrapf(err, "could not get file %s for patch %ss", patchFileID, td.ID)
	}
	defer resp.Body.Close()

	var result []byte
	result, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "problem reading file %s for patch %s", patchFileID, td.ID)
	}

	return string(result), nil
}

// SendTestLog is used by the attach plugin to add to the test_logs
// collection for log data associated with a test.
func (c *communicatorImpl) SendTestLog(ctx context.Context, td TaskData, log *model.TestLog) (string, error) {
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("test_logs", td.ID), td, v1, log)
	if err != nil {
		return "", errors.Wrapf(err, "problem sending task log for %s", td.ID)
	}
	defer resp.Body.Close()

	var result []byte
	result, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "problem log id after posting test log for %s", td.ID)
	}

	return string(result), nil
}

// PostJSON does an HTTP POST for the communicator's plugin + task.
func (c *communicatorImpl) PostJSON(ctx context.Context, taskData TaskData, pluginName, endpoint string, data interface{}) (*http.Response, error) {
	path := c.getTaskPathSuffix(fmt.Sprintf("%s/%s", pluginName, endpoint), taskData.ID)
	return c.retryPost(ctx, path, taskData, v1, data)
}

// GetJSON does an HTTP GET for the communicator's plugin + task.
func (c *communicatorImpl) GetJSON(ctx context.Context, taskData TaskData, pluginName, endpoint string) (*http.Response, error) {
	path := c.getTaskPathSuffix(fmt.Sprintf("%s/%s", pluginName, endpoint), taskData.ID)
	return c.retryGet(ctx, path, taskData, v1)
}

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *communicatorImpl) SendResults(ctx context.Context, taskData TaskData, results *task.TestResults) error {
	if results == nil || len(results.Results) == 0 {
		return nil
	}
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("results", taskData.ID), taskData, v1, results)
	if err != nil {
		err = errors.Wrapf(err, "failed to post results for task %s", taskData.ID)
		return err
	}
	defer resp.Body.Close()
	return nil
}

// SendFiles attaches task files.
func (c *communicatorImpl) SendFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("files", taskData.ID), taskData, v1, taskFiles)
	if err != nil {
		err = errors.Wrapf(err, "failed to post task files for task %s", taskData.ID)
		return err
	}
	defer resp.Body.Close()
	return nil
}

// PostTestData posts a test log for a communicator's task. Is a
// noop if the test Log is nil.
func (c *communicatorImpl) PostTestData(ctx context.Context, taskData TaskData, log *model.TestLog) (string, error) {
	if log == nil {
		return "", nil
	}

	resp, err := c.retryPost(ctx, c.getTaskPathSuffix("test_logs", taskData.ID), taskData, v1, log)
	if err != nil {
		err = errors.Wrapf(err, "failed to post test logs for task %s", taskData.ID)
		return "", err
	}
	defer resp.Body.Close()
	logReply := struct {
		ID string `json:"_id"`
	}{}
	if err = util.ReadJSONInto(resp.Body, &logReply); err != nil {
		message := fmt.Sprintf("Error unmarshalling post test log response: %v", err)
		return "", errors.New(message)
	}
	logID := logReply.ID
	return logID, nil
}
