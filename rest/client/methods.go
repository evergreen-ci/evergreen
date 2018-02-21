package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// StartTask marks the task as started.
func (c *communicatorImpl) StartTask(ctx context.Context, taskData TaskData) error {
	grip.Info(message.Fields{
		"message":     "started StartTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	pidStr := strconv.Itoa(os.Getpid())
	taskStartRequest := &apimodels.TaskStartRequest{Pid: pidStr}
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("start")
	resp, err := c.retryRequest(ctx, info, taskStartRequest)
	if err != nil {
		err = errors.Wrapf(err, "failed to start task %s", taskData.ID)
		return err
	}
	defer resp.Body.Close()
	grip.Info(message.Fields{
		"message":     "finished StartTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	return nil
}

// EndTask marks the task as finished with the given status
func (c *communicatorImpl) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	grip.Info(message.Fields{
		"message":     "started EndTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	taskEndResp := &apimodels.EndTaskResponse{}
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("end")
	resp, err := c.retryRequest(ctx, info, detail)
	if err != nil {
		err = errors.Wrapf(err, "failed to end task %s", taskData.ID)
		return nil, err
	}
	defer resp.Body.Close()
	if err = util.ReadJSONInto(resp.Body, taskEndResp); err != nil {
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

// GetTask returns the active task.
func (c *communicatorImpl) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	task := &task.Task{}
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("")
	resp, err := c.retryRequest(ctx, info, nil)
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
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("project_ref")
	resp, err := c.retryRequest(ctx, info, nil)
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
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("distro")
	resp, err := c.retryRequest(ctx, info, nil)
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
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("version")
	resp, err := c.retryRequest(ctx, info, nil)
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
	data := interface{}("heartbeat")
	ctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	info := requestInfo{
		method:   post,
		version:  v1,
		taskData: &taskData,
	}
	info.setTaskPathSuffix("heartbeat")
	resp, err := c.request(ctx, info, data)
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
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("fetch_vars")
	resp, err := c.retryRequest(ctx, info, nil)
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
func (c *communicatorImpl) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	nextTask := &apimodels.NextTaskResponse{}
	info := requestInfo{
		method:  get,
		version: v1,
	}
	info.path = "agent/next_task"
	resp, err := c.retryRequest(ctx, info, details)
	if err != nil {
		err = errors.Wrap(err, "failed to get task")
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict - wrong secret")
	}
	if err = util.ReadJSONInto(resp.Body, nextTask); err != nil {
		err = errors.Wrap(err, "failed to read next task from response")
		return nil, err
	}
	if nextTask.ShouldExit {
		return nil, errors.New("Next task response indicates agent should exit")
	}
	return nextTask, nil

}

// SendLogMessages posts a group of log messages for a task.
func (c *communicatorImpl) SendLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	payload := apimodels.TaskLog{
		TaskId:       taskData.ID,
		Timestamp:    time.Now(),
		MessageCount: len(msgs),
		Messages:     msgs,
	}

	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("log")
	if _, err := c.retryRequest(ctx, info, &payload); err != nil {
		return errors.Wrapf(err, "problem sending %d log messages for task %s", len(msgs), taskData.ID)
	}

	return nil
}

// SendTaskResults posts a task's results, used by the attach results operations.
func (c *communicatorImpl) SendTaskResults(ctx context.Context, taskData TaskData, r *task.LocalTestResults) error {
	if r == nil || len(r.Results) == 0 {
		return nil
	}

	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("results")
	if _, err := c.retryRequest(ctx, info, r); err != nil {
		return errors.Wrapf(err, "problem adding %d results to task %s", len(r.Results), taskData.ID)
	}

	return nil
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure.
func (c *communicatorImpl) GetTaskPatch(ctx context.Context, taskData TaskData) (*patchmodel.Patch, error) {
	patch := patchmodel.Patch{}
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("git/patch")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get patch for %s", taskData.ID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("could not fetch patch for task with id '%s'; expected status code 200 OK, got %d %s", taskData.ID, resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if err = util.ReadJSONInto(resp.Body, &patch); err != nil {
		return nil, errors.Wrapf(err, "problem parsing patch response for %s", taskData.ID)
	}

	return &patch, nil
}

// GetPatchFiles is used by the git.get_project plugin and fetches
// patches from the database, used in patch builds.
func (c *communicatorImpl) GetPatchFile(ctx context.Context, taskData TaskData, patchFileID string) (string, error) {
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("git/patchfile/" + patchFileID)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return "", errors.Wrapf(err, "could not get file %s for patch %ss", patchFileID, taskData.ID)
	}
	defer resp.Body.Close()

	var result []byte
	result, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "problem reading file %s for patch %s", patchFileID, taskData.ID)
	}

	return string(result), nil
}

// SendTestLog is used by the attach plugin to add to the test_logs
// collection for log data associated with a test.
func (c *communicatorImpl) SendTestLog(ctx context.Context, taskData TaskData, log *model.TestLog) (string, error) {
	if log == nil {
		return "", nil
	}

	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("test_logs")
	resp, err := c.retryRequest(ctx, info, log)
	if err != nil {
		return "", errors.Wrapf(err, "problem sending task log '%+v' for %s", *log, taskData.ID)
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

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *communicatorImpl) SendTestResults(ctx context.Context, taskData TaskData, results *task.LocalTestResults) error {
	if results == nil || len(results.Results) == 0 {
		return nil
	}
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("results")
	resp, err := c.retryRequest(ctx, info, results)
	if err != nil {
		return errors.Wrapf(err, "failed to post results '%+v' for task %s", *results, taskData.ID)
	}
	defer resp.Body.Close()
	return nil
}

// AttachFiles attaches task files.
func (c *communicatorImpl) AttachFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	if len(taskFiles) == 0 {
		return nil
	}

	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("files")
	resp, err := c.retryRequest(ctx, info, taskFiles)
	if err != nil {
		return errors.Wrapf(err, "failed to post task files for task %s", taskData.ID)
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) GetManifest(ctx context.Context, taskData TaskData) (*manifest.Manifest, error) {
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("manifest/load")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "problem loading manifest for %s", taskData.ID)
	}
	defer resp.Body.Close()

	mfest := manifest.Manifest{}
	if err = util.ReadJSONInto(resp.Body, &mfest); err != nil {
		return nil, errors.Wrapf(err, "problem parsing manifest response for %s", taskData.ID)
	}

	return &mfest, nil
}

func (c *communicatorImpl) S3Copy(ctx context.Context, taskData TaskData, req *apimodels.S3CopyRequest) error {
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("s3Copy/s3Copy")
	resp, err := c.retryRequest(ctx, info, req)
	if err != nil {
		return errors.Wrapf(err, "problem with s3copy for %s", taskData.ID)
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) KeyValInc(ctx context.Context, taskData TaskData, kv *model.KeyVal) error {
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix("keyval/inc")
	resp, err := c.retryRequest(ctx, info, kv.Key)
	if err != nil {
		return errors.Wrapf(err, "problem with keyval increment operation for %s", taskData.ID)
	}
	defer resp.Body.Close()

	if err = util.ReadJSONInto(resp.Body, kv); err != nil {
		return errors.Wrapf(err, "problem parsing keyval inc response %s", taskData.ID)
	}

	return nil
}

func (c *communicatorImpl) PostJSONData(ctx context.Context, taskData TaskData, path string, data interface{}) error {
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix(fmt.Sprintf("json/data/%s", path))
	resp, err := c.retryRequest(ctx, info, data)
	if err != nil {
		return errors.Wrapf(err, "problem with post json data operation for %s", taskData.ID)
	}
	defer resp.Body.Close()

	return nil
}

func (c *communicatorImpl) GetJSONData(ctx context.Context, taskData TaskData, taskName, dataName, variantName string) ([]byte, error) {
	pathParts := []string{"json", "data", taskName, dataName}
	if variantName != "" {
		pathParts = append(pathParts, variantName)
	}
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix(strings.Join(pathParts, "/"))
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "problem with get json data operation for %s", taskData.ID)
	}
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "problem reading results from body for %s", taskData.ID)
	}

	return out, nil
}

func (c *communicatorImpl) GetJSONHistory(ctx context.Context, taskData TaskData, tags bool, taskName, dataName string) ([]byte, error) {
	path := "json/history/"
	if tags {
		path = "json/tags/"
	}

	path += fmt.Sprintf("%s/%s", taskName, dataName)

	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  v1,
	}
	info.setTaskPathSuffix(path)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "problem json history document for %s at %s", taskData.ID, path)
	}
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "problem reading results from body for %s", taskData.ID)
	}

	return out, nil
}

func (c *communicatorImpl) SendProcessInfo(ctx context.Context, td TaskData, procs []*message.ProcessInfo) error {
	if len(procs) == 0 {
		return nil
	}

	info := requestInfo{
		method:   post,
		taskData: &td,
		version:  v1,
	}

	info.setTaskPathSuffix("process_info")
	_, err := c.retryRequest(ctx, info, procs)

	return errors.Wrap(err, "problem sending process info results")
}

func (c *communicatorImpl) SendSystemInfo(ctx context.Context, td TaskData, sysinfo *message.SystemInfo) error {
	if sysinfo == nil {
		return nil
	}

	info := requestInfo{
		method:   post,
		version:  v1,
		taskData: &td,
	}
	info.setTaskPathSuffix("system_info")
	_, err := c.retryRequest(ctx, info, sysinfo)

	return errors.Wrap(err, "problem sending sysinfo results")
}

// GenerateTasks posts new tasks for the `generate.tasks` command.
func (c *communicatorImpl) GenerateTasks(ctx context.Context, td TaskData, json [][]byte) error {
	// TODO EVG-2483
	return nil
}
