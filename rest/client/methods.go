package client

import (
	"context"
	"encoding/json"
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
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func (c *communicatorImpl) GetAgentSetupData(ctx context.Context) (*apimodels.AgentSetupData, error) {
	out := &apimodels.AgentSetupData{}
	info := requestInfo{
		method:  get,
		version: apiVersion1,
		path:    "agent/setup",
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get agent setup info")
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, out); err != nil {
		return nil, errors.Wrap(err, "failed to get agent setup info")
	}
	return out, nil
}

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
		version:  apiVersion1,
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
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("end")
	resp, err := c.retryRequest(ctx, info, detail)
	if err != nil {
		err = errors.Wrapf(err, "failed to end task %s", taskData.ID)
		return nil, err
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

// GetTask returns the active task.
func (c *communicatorImpl) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	task := &task.Task{}
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, task); err != nil {
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
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, projectRef); err != nil {
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
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, d); err != nil {
		err = errors.Wrapf(err, "unable to read distro response for task %s", taskData.ID)
		return nil, err
	}
	return d, nil
}

func (c *communicatorImpl) GetProject(ctx context.Context, taskData TaskData) (*model.Project, error) {
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("parser_project")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		err = errors.Wrapf(err, "failed to get project for task %s", taskData.ID)
		return nil, err
	}
	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading body")
	}
	return model.GetProjectFromBSON(respBytes)
}

func (c *communicatorImpl) GetExpansions(ctx context.Context, taskData TaskData) (util.Expansions, error) {
	e := util.Expansions{}
	info := requestInfo{
		method:   get,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("expansions")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get expansions for task %s", taskData.ID)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, errors.New("conflict; wrong secret")
	}
	err = utility.ReadJSON(resp.Body, &e)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read project version response for task %s", taskData.ID)
	}
	return e, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// an "abort" response. This function returns true if the agent should abort.
func (c *communicatorImpl) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	data := interface{}("heartbeat")
	ctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	info := requestInfo{
		method:   post,
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, heartbeatResponse); err != nil {
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
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, resultVars); err != nil {
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
		version: apiVersion1,
	}
	info.path = "agent/next_task"
	resp, err := c.retryRequest(ctx, info, details)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get task")
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, nextTask); err != nil {
		err = errors.Wrap(err, "failed to read next task from response")
		return nil, err
	}
	return nextTask, nil
}

// GetBuildloggerInfo returns buildlogger service information including the
// base URL, RPC port, and LDAP credentials.
func (c *communicatorImpl) GetBuildloggerInfo(ctx context.Context) (*apimodels.BuildloggerInfo, error) {
	bi := &apimodels.BuildloggerInfo{}

	info := requestInfo{
		method:  get,
		version: apiVersion1,
		path:    "agent/buildlogger_info",
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get buildlogger service info")
	}
	defer resp.Body.Close()

	if err = utility.ReadJSON(resp.Body, bi); err != nil {
		err = errors.Wrap(err, "failed to read next task from response")
		return nil, err
	}

	return bi, nil
}

// GetCedarGRPCConn returns the client connection to cedar if it exists, or
// creates it if it doesn't exist.
func (c *communicatorImpl) GetCedarGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	if err := c.createCedarGRPCConn(ctx); err != nil {
		return nil, errors.Wrap(err, "error setting up cedar grpc connection")
	}
	return c.cedarGRPCClient, nil
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
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("log")
	var cancel context.CancelFunc
	now := time.Now()
	grip.Debugf("sending %d log messages", payload.MessageCount)
	ctx, cancel = context.WithDeadline(ctx, now.Add(10*time.Minute))
	defer cancel()
	backupTimer := time.NewTimer(15 * time.Minute)
	defer backupTimer.Stop()
	doneChan := make(chan struct{})
	defer func() {
		close(doneChan)
	}()
	go func() {
		defer recovery.LogStackTraceAndExit("backup timer")
		select {
		case <-ctx.Done():
			grip.Info("request completed or task ending, stopping backup timer thread")
			return
		case t := <-backupTimer.C:
			grip.Alert(message.Fields{
				"message":  "retryRequest exceeded 15 minutes",
				"start":    now.String(),
				"end":      t.String(),
				"task":     taskData.ID,
				"messages": msgs,
			})
			cancel()
			return
		case <-doneChan:
			return
		}
	}()
	if _, err := c.retryRequest(ctx, info, &payload); err != nil {
		return errors.Wrapf(err, "problem sending %d log messages for task %s", len(msgs), taskData.ID)
	}
	grip.Debugf("successfully sent %d log messages", payload.MessageCount)

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
		version:  apiVersion1,
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
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, &patch); err != nil {
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
		version:  apiVersion1,
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
		version:  apiVersion1,
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
	if err = utility.ReadJSON(resp.Body, &logReply); err != nil {
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
		version:  apiVersion1,
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
		version:  apiVersion1,
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
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("manifest/load")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "problem loading manifest for %s", taskData.ID)
	}
	defer resp.Body.Close()

	mfest := manifest.Manifest{}
	if err = utility.ReadJSON(resp.Body, &mfest); err != nil {
		return nil, errors.Wrapf(err, "problem parsing manifest response for %s", taskData.ID)
	}

	return &mfest, nil
}

func (c *communicatorImpl) S3Copy(ctx context.Context, taskData TaskData, req *apimodels.S3CopyRequest) (string, error) {
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("s3Copy/s3Copy")
	resp, err := c.retryRequest(ctx, info, req)
	if err != nil {
		return "", errors.Wrapf(err, "problem with s3copy for %s", taskData.ID)
	}
	defer resp.Body.Close()

	response := gimlet.ErrorResponse{}
	if err = utility.ReadJSON(resp.Body, &response); err == nil {
		return response.Message, nil
	}

	return "", nil
}

func (c *communicatorImpl) KeyValInc(ctx context.Context, taskData TaskData, kv *model.KeyVal) error {
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("keyval/inc")
	resp, err := c.retryRequest(ctx, info, kv.Key)
	if err != nil {
		return errors.Wrapf(err, "problem with keyval increment operation for %s", taskData.ID)
	}
	defer resp.Body.Close()

	if err = utility.ReadJSON(resp.Body, kv); err != nil {
		return errors.Wrapf(err, "problem parsing keyval inc response %s", taskData.ID)
	}

	return nil
}

func (c *communicatorImpl) PostJSONData(ctx context.Context, taskData TaskData, path string, data interface{}) error {
	info := requestInfo{
		method:   post,
		taskData: &taskData,
		version:  apiVersion1,
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
		version:  apiVersion1,
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
		version:  apiVersion1,
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

// GenerateTasks posts new tasks for the `generate.tasks` command.
func (c *communicatorImpl) GenerateTasks(ctx context.Context, td TaskData, jsonBytes []json.RawMessage) error {
	info := requestInfo{
		method:   post,
		taskData: &td,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("tasks/%s/generate", td.ID)
	_, err := c.retryRequest(ctx, info, jsonBytes)
	return errors.Wrap(err, "problem sending `generate.tasks` request")
}

// GenerateTasksPoll posts new tasks for the `generate.tasks` command.
func (c *communicatorImpl) GenerateTasksPoll(ctx context.Context, td TaskData) (*apimodels.GeneratePollResponse, error) {
	info := requestInfo{
		method:   get,
		taskData: &td,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("tasks/%s/generate", td.ID)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrap(err, "problem sending `generate.tasks` request")
	}
	defer resp.Body.Close()
	generated := &apimodels.GeneratePollResponse{}
	if err := utility.ReadJSON(resp.Body, generated); err != nil {
		return nil, errors.Wrapf(err, "problem reading generated from response body for '%s'", td.ID)
	}
	return generated, nil
}

// CreateHost requests a new host be created
func (c *communicatorImpl) CreateHost(ctx context.Context, td TaskData, options apimodels.CreateHost) ([]string, error) {
	info := requestInfo{
		method:   post,
		taskData: &td,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("hosts/%s/create", td.ID)
	resp, err := c.retryRequest(ctx, info, options)
	if err != nil {
		return nil, errors.Wrap(err, "problem sending `create.host` request")
	}
	defer resp.Body.Close()

	ids := []string{}
	if err = utility.ReadJSON(resp.Body, &ids); err != nil {
		return nil, errors.Wrap(err, "problem reading ids from `create.host` response")
	}
	return ids, nil
}

func (c *communicatorImpl) ListHosts(ctx context.Context, td TaskData) ([]restmodel.CreateHost, error) {
	info := requestInfo{
		method:   get,
		taskData: &td,
		version:  apiVersion2,
		path:     fmt.Sprintf("hosts/%s/list", td.ID),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "problem listing hosts for task '%s'", td.ID)
	}
	defer resp.Body.Close()

	hosts := []restmodel.CreateHost{}
	if err := utility.ReadJSON(resp.Body, &hosts); err != nil {
		return nil, errors.Wrapf(err, "problem reading hosts from response body for '%s'", td.ID)
	}
	return hosts, nil
}

func (c *communicatorImpl) GetDistroByName(ctx context.Context, id string) (*restmodel.APIDistro, error) {
	info := requestInfo{
		method:  get,
		version: apiVersion2,
		path:    fmt.Sprintf("distro/%s", id),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "problem requesting distro named '%s", id)
	}
	defer resp.Body.Close()

	d := &restmodel.APIDistro{}
	if err = utility.ReadJSON(resp.Body, &d); err != nil {
		return nil, errors.Wrapf(err, "reading distro from response body for '%s'", id)
	}

	return d, nil

}
