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

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/timber/buildlogger"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
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

// StartTask marks the task as started.
func (c *hostCommunicator) StartTask(ctx context.Context, taskData TaskData) error {
	grip.Info(message.Fields{
		"message":     "started StartTask",
		"task_id":     taskData.ID,
		"task_secret": taskData.Secret,
	})
	pidStr := strconv.Itoa(os.Getpid())
	taskStartRequest := &apimodels.TaskStartRequest{Pid: pidStr}
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("start")
	resp, err := c.retryRequest(ctx, info, taskStartRequest)
	if err != nil {
		return utility.RespErrorf(resp, "failed to start task %s: %s", taskData.ID, err.Error())
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

// DisableHost signals to the app server that the host should be disabled.
func (c *hostCommunicator) DisableHost(ctx context.Context, hostID string, details apimodels.DisableInfo) error {
	info := requestInfo{
		method:  http.MethodPost,
		version: apiVersion2,
		path:    fmt.Sprintf("hosts/%s/disable", hostID),
	}
	resp, err := c.retryRequest(ctx, info, &details)
	if err != nil {
		return utility.RespErrorf(resp, "failed to disable host: %s", err.Error())
	}

	defer resp.Body.Close()
	return nil
}

// GetTask returns the active task.
func (c *hostCommunicator) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	task := &task.Task{}
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, task); err != nil {
		err = errors.Wrapf(err, "failed reading json for task %s", taskData.ID)
		return nil, err
	}
	return task, nil
}

// GetDisplayTaskInfoFromExecution returns the display task info associated
// with the execution task.
func (c *hostCommunicator) GetDisplayTaskInfoFromExecution(ctx context.Context, td TaskData) (*apimodels.DisplayTaskInfo, error) {
	info := requestInfo{
		method:   http.MethodGet,
		path:     fmt.Sprintf("tasks/%s/display_task", td.ID),
		taskData: &td,
		version:  apiVersion2,
	}
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get display task of task %s: %s", td.ID, err.Error())
	}
	defer resp.Body.Close()

	displayTaskInfo := &apimodels.DisplayTaskInfo{}
	err = utility.ReadJSON(resp.Body, &displayTaskInfo)
	if err != nil {
		return nil, errors.Wrapf(err, "reading display task info of task %s", td.ID)
	}

	return displayTaskInfo, nil
}

// GetProjectRef loads the task's project.
func (c *hostCommunicator) GetProjectRef(ctx context.Context, taskData TaskData) (*model.ProjectRef, error) {
	projectRef := &model.ProjectRef{}
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("project_ref")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get project ref for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, projectRef); err != nil {
		err = errors.Wrapf(err, "failed reading json for task %s", taskData.ID)
		return nil, err
	}
	return projectRef, nil
}

func (c *hostCommunicator) GetDistroView(ctx context.Context, taskData TaskData) (*apimodels.DistroView, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("distro_view")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get distro for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	var dv apimodels.DistroView
	if err = utility.ReadJSON(resp.Body, &dv); err != nil {
		err = errors.Wrapf(err, "unable to read distro response for task %s", taskData.ID)
		return nil, err
	}
	return &dv, nil
}

// GetDistroAMI returns the distro for the task.
func (c *hostCommunicator) GetDistroAMI(ctx context.Context, distro, region string, taskData TaskData) (string, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("distros/%s/ami", distro)
	if region != "" {
		info.path = fmt.Sprintf("%s?region=%s", info.path, region)
	}
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return "", utility.RespErrorf(resp, "failed to get distro AMI for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrapf(err, "problem reading results from body for %s", taskData.ID)
	}
	return string(out), nil
}

func (c *hostCommunicator) GetProject(ctx context.Context, taskData TaskData) (*model.Project, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("parser_project")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get project for task %s: %s", taskData.ID, err.Error())
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading body")
	}
	return model.GetProjectFromBSON(respBytes)
}

func (c *hostCommunicator) GetExpansions(ctx context.Context, taskData TaskData) (util.Expansions, error) {
	e := util.Expansions{}
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("expansions")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get expansions for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	err = utility.ReadJSON(resp.Body, &e)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read project version response for task %s", taskData.ID)
	}
	return e, nil
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// an "abort" response. This function returns true if the agent should abort.
func (c *hostCommunicator) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	data := interface{}("heartbeat")
	ctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()
	info := requestInfo{
		method:   http.MethodPost,
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
func (c *hostCommunicator) FetchExpansionVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionVars, error) {
	resultVars := &apimodels.ExpansionVars{}
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("fetch_vars")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get expansion vars for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	if err = utility.ReadJSON(resp.Body, resultVars); err != nil {
		err = errors.Wrapf(err, "failed to read vars from response for task %s", taskData.ID)
		return nil, err
	}
	return resultVars, err
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

// GetCedarGRPCConn returns the client connection to cedar if it exists, or
// creates it if it doesn't exist.
func (c *hostCommunicator) GetCedarGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	if err := c.createCedarGRPCConn(ctx, c); err != nil {
		return nil, errors.Wrap(err, "setting up cedar grpc connection")
	}
	return c.cedarGRPCClient, nil
}

func (c *hostCommunicator) GetLoggerProducer(ctx context.Context, td TaskData, config *LoggerConfig) (LoggerProducer, error) {
	if config == nil {
		config = &LoggerConfig{
			Agent:  []LogOpts{{Sender: model.EvergreenLogSender}},
			System: []LogOpts{{Sender: model.EvergreenLogSender}},
			Task:   []LogOpts{{Sender: model.EvergreenLogSender}},
		}
	}
	underlying := []send.Sender{}

	exec, senders, err := c.makeSender(ctx, td, config.Agent, apimodels.AgentLogPrefix, evergreen.LogTypeAgent)
	if err != nil {
		return nil, errors.Wrap(err, "making agent logger")
	}
	underlying = append(underlying, senders...)
	task, senders, err := c.makeSender(ctx, td, config.Task, apimodels.TaskLogPrefix, evergreen.LogTypeTask)
	if err != nil {
		return nil, errors.Wrap(err, "making task logger")
	}
	underlying = append(underlying, senders...)
	system, senders, err := c.makeSender(ctx, td, config.System, apimodels.SystemLogPrefix, evergreen.LogTypeSystem)
	if err != nil {
		return nil, errors.Wrap(err, "making system logger")
	}
	underlying = append(underlying, senders...)

	return &logHarness{
		execution:                 logging.MakeGrip(exec),
		task:                      logging.MakeGrip(task),
		system:                    logging.MakeGrip(system),
		underlyingBufferedSenders: underlying,
	}, nil
}

func (c *hostCommunicator) makeSender(ctx context.Context, td TaskData, opts []LogOpts, prefix string, logType string) (send.Sender, []send.Sender, error) {
	levelInfo := send.LevelInfo{Default: level.Info, Threshold: level.Debug}
	senders := []send.Sender{grip.GetSender()}
	underlyingBufferedSenders := []send.Sender{}

	for _, opt := range opts {
		var sender send.Sender
		var err error
		bufferDuration := defaultLogBufferTime
		if opt.BufferDuration > 0 {
			bufferDuration = opt.BufferDuration
		}
		bufferSize := defaultLogBufferSize
		if opt.BufferSize > 0 {
			bufferSize = opt.BufferSize
		}
		// disallow sending system logs to S3 or logkeeper for security reasons
		if prefix == apimodels.SystemLogPrefix && (opt.Sender == model.FileLogSender || opt.Sender == model.LogkeeperLogSender) {
			opt.Sender = model.EvergreenLogSender
		}
		switch opt.Sender {
		case model.FileLogSender:
			sender, err = send.NewPlainFileLogger(prefix, opt.Filepath, levelInfo)
			if err != nil {
				return nil, nil, errors.Wrap(err, "creating file logger")
			}

			underlyingBufferedSenders = append(underlyingBufferedSenders, sender)
			sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
		case model.SplunkLogSender:
			info := send.SplunkConnectionInfo{
				ServerURL: opt.SplunkServerURL,
				Token:     opt.SplunkToken,
			}
			sender, err = send.NewSplunkLogger(prefix, info, levelInfo)
			if err != nil {
				return nil, nil, errors.Wrap(err, "creating splunk logger")
			}
			underlyingBufferedSenders = append(underlyingBufferedSenders, sender)
			sender = send.NewBufferedSender(newAnnotatedWrapper(td.ID, prefix, sender), bufferDuration, bufferSize)
		case model.LogkeeperLogSender:
			config := send.BuildloggerConfig{
				URL:        opt.LogkeeperURL,
				Number:     opt.LogkeeperBuildNum,
				Local:      grip.GetSender(),
				Test:       prefix,
				CreateTest: true,
			}
			sender, err = send.NewBuildlogger(opt.BuilderID, &config, levelInfo)
			if err != nil {
				return nil, nil, errors.Wrap(err, "creating logkeeper logger")
			}
			underlyingBufferedSenders = append(underlyingBufferedSenders, sender)
			sender = send.NewBufferedSender(sender, bufferDuration, bufferSize)
			metadata := LogkeeperMetadata{
				Build: config.GetBuildID(),
				Test:  config.GetTestID(),
			}
			switch prefix {
			case apimodels.AgentLogPrefix:
				c.loggerInfo.Agent = append(c.loggerInfo.Agent, metadata)
			case apimodels.SystemLogPrefix:
				c.loggerInfo.System = append(c.loggerInfo.System, metadata)
			case apimodels.TaskLogPrefix:
				c.loggerInfo.Task = append(c.loggerInfo.Task, metadata)
			}
		case model.BuildloggerLogSender:
			tk, err := c.GetTask(ctx, td)
			if err != nil {
				return nil, nil, errors.Wrap(err, "setting up buildlogger sender")
			}

			if err = c.createCedarGRPCConn(ctx, c); err != nil {
				return nil, nil, errors.Wrap(err, "setting up cedar grpc connection")
			}

			timberOpts := &buildlogger.LoggerOptions{
				Project:       tk.Project,
				Version:       tk.Version,
				Variant:       tk.BuildVariant,
				TaskName:      tk.DisplayName,
				TaskID:        tk.Id,
				Execution:     int32(tk.Execution),
				Tags:          append(tk.Tags, logType, utility.RandomString()),
				Mainline:      !evergreen.IsPatchRequester(tk.Requester),
				Storage:       buildlogger.LogStorageS3,
				MaxBufferSize: opt.BufferSize,
				FlushInterval: opt.BufferDuration,
				ClientConn:    c.cedarGRPCClient,
			}
			sender, err = buildlogger.NewLoggerWithContext(ctx, opt.BuilderID, levelInfo, timberOpts)
			if err != nil {
				return nil, nil, errors.Wrap(err, "creating buildlogger logger")
			}
		default:
			sender = newEvergreenLogSender(ctx, c, prefix, td, bufferSize, bufferDuration)
		}

		grip.Error(sender.SetFormatter(send.MakeDefaultFormatter()))
		if prefix == apimodels.TaskLogPrefix {
			sender = makeTimeoutLogSender(sender, c)
		}
		senders = append(senders, sender)
	}

	return send.NewConfiguredMultiSender(senders...), underlyingBufferedSenders, nil
}

// SendLogMessages posts a group of log messages for a task.
func (c *hostCommunicator) SendLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
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
		method:   http.MethodPost,
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
	resp, err := c.retryRequest(ctx, info, &payload)
	if err != nil {
		return utility.RespErrorf(resp, "problem sending %d log messages for task %s: %s", len(msgs), taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	return nil
}

// SendTaskResults posts a task's results, used by the attach results operations.
func (c *hostCommunicator) SendTaskResults(ctx context.Context, taskData TaskData, r *task.LocalTestResults) error {
	if r == nil || len(r.Results) == 0 {
		return nil
	}

	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("results")
	resp, err := c.retryRequest(ctx, info, r)
	if err != nil {
		return utility.RespErrorf(resp, "problem adding %d results to task %s: %s", len(r.Results), taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	return nil
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure. If patchId is not specified, the task's
// patch is returned
func (c *hostCommunicator) GetTaskPatch(ctx context.Context, taskData TaskData, patchId string) (*patchmodel.Patch, error) {
	patch := patchmodel.Patch{}
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	suffix := "git/patch"
	if patchId != "" {
		suffix = fmt.Sprintf("%s?patch=%s", suffix, patchId)
	}
	info.setTaskPathSuffix(suffix)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get patch for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	if err = utility.ReadJSON(resp.Body, &patch); err != nil {
		return nil, errors.Wrapf(err, "problem parsing patch response for %s", taskData.ID)
	}

	return &patch, nil
}

// GetPatchFiles is used by the git.get_project plugin and fetches
// patches from the database, used in patch builds.
func (c *hostCommunicator) GetPatchFile(ctx context.Context, taskData TaskData, patchFileID string) (string, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("git/patchfile/" + patchFileID)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return "", utility.RespErrorf(resp, "failed to get patch file %s for task %s: %s", patchFileID, taskData.ID, err.Error())
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
func (c *hostCommunicator) SendTestLog(ctx context.Context, taskData TaskData, log *model.TestLog) (string, error) {
	if log == nil {
		return "", nil
	}

	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("test_logs")
	resp, err := c.retryRequest(ctx, info, log)
	if err != nil {
		return "", utility.RespErrorf(resp, "failed to send test log for task %s: %s", taskData.ID, err.Error())
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
func (c *hostCommunicator) SendTestResults(ctx context.Context, taskData TaskData, results *task.LocalTestResults) error {
	if results == nil || len(results.Results) == 0 {
		return nil
	}
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("results")
	resp, err := c.retryRequest(ctx, info, results)
	if err != nil {
		return utility.RespErrorf(resp, "failed to send test results for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	return nil
}

// SetHasCedarResults sets the HasCedarResults flag to true in the given task
// in the database.
func (c *hostCommunicator) SetHasCedarResults(ctx context.Context, taskData TaskData, failed bool) error {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("tasks/%s/set_has_cedar_results", taskData.ID)
	resp, err := c.retryRequest(ctx, info, &apimodels.CedarTestResultsTaskInfo{Failed: failed})
	if err != nil {
		return utility.RespErrorf(resp, "failed to set HasCedarResults for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	return nil
}

// AttachFiles attaches task files.
func (c *hostCommunicator) AttachFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	if len(taskFiles) == 0 {
		return nil
	}

	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("files")
	resp, err := c.retryRequest(ctx, info, taskFiles)
	if err != nil {
		return utility.RespErrorf(resp, "failed to post files for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	return nil
}

func (c *hostCommunicator) SetDownstreamParams(ctx context.Context, downstreamParams []patchmodel.Parameter, taskData TaskData) error {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}

	info.setTaskPathSuffix("downstreamParams")
	resp, err := c.retryRequest(ctx, info, downstreamParams)
	if err != nil {
		return utility.RespErrorf(resp, "failed to set upstream params for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	return nil
}

func (c *hostCommunicator) GetManifest(ctx context.Context, taskData TaskData) (*manifest.Manifest, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("manifest/load")
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to load manifest for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	mfest := manifest.Manifest{}
	if err = utility.ReadJSON(resp.Body, &mfest); err != nil {
		return nil, errors.Wrapf(err, "problem parsing manifest response for %s", taskData.ID)
	}

	return &mfest, nil
}

func (c *hostCommunicator) S3Copy(ctx context.Context, taskData TaskData, req *apimodels.S3CopyRequest) (string, error) {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("s3Copy/s3Copy")
	resp, err := c.retryRequest(ctx, info, req)
	if err != nil {
		return "", utility.RespErrorf(resp, "failed to copy file in S3 for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()
	return "", nil
}

func (c *hostCommunicator) KeyValInc(ctx context.Context, taskData TaskData, kv *model.KeyVal) error {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix("keyval/inc")
	resp, err := c.retryRequest(ctx, info, kv.Key)
	if err != nil {
		return utility.RespErrorf(resp, "failed to increment key for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	if err = utility.ReadJSON(resp.Body, kv); err != nil {
		return errors.Wrapf(err, "problem parsing keyval inc response %s", taskData.ID)
	}

	return nil
}

func (c *hostCommunicator) PostJSONData(ctx context.Context, taskData TaskData, path string, data interface{}) error {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix(fmt.Sprintf("json/data/%s", path))
	resp, err := c.retryRequest(ctx, info, data)
	if err != nil {
		return utility.RespErrorf(resp, "failed to post json data for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	return nil
}

func (c *hostCommunicator) GetJSONData(ctx context.Context, taskData TaskData, taskName, dataName, variantName string) ([]byte, error) {
	pathParts := []string{"json", "data", taskName, dataName}
	if variantName != "" {
		pathParts = append(pathParts, variantName)
	}
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix(strings.Join(pathParts, "/"))
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get json data for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "problem reading results from body for %s", taskData.ID)
	}

	return out, nil
}

func (c *hostCommunicator) GetJSONHistory(ctx context.Context, taskData TaskData, tags bool, taskName, dataName string) ([]byte, error) {
	path := "json/history/"
	if tags {
		path = "json/tags/"
	}

	path += fmt.Sprintf("%s/%s", taskName, dataName)

	info := requestInfo{
		method:   http.MethodGet,
		taskData: &taskData,
		version:  apiVersion1,
	}
	info.setTaskPathSuffix(path)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get json history for task %s: %s", taskData.ID, err.Error())
	}
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "problem reading results from body for %s", taskData.ID)
	}

	return out, nil
}

// GenerateTasks posts new tasks for the `generate.tasks` command.
func (c *hostCommunicator) GenerateTasks(ctx context.Context, td TaskData, jsonBytes []json.RawMessage) error {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &td,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("tasks/%s/generate", td.ID)
	resp, err := c.retryRequest(ctx, info, jsonBytes)
	if err != nil {
		return utility.RespErrorf(resp, "problem sending `generate.tasks` request: %s", err.Error())
	}
	return nil
}

// GenerateTasksPoll posts new tasks for the `generate.tasks` command.
func (c *hostCommunicator) GenerateTasksPoll(ctx context.Context, td TaskData) (*apimodels.GeneratePollResponse, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &td,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("tasks/%s/generate", td.ID)
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to send generate.tasks request for task %s: %s", td.ID, err.Error())
	}
	defer resp.Body.Close()
	generated := &apimodels.GeneratePollResponse{}
	if err := utility.ReadJSON(resp.Body, generated); err != nil {
		return nil, errors.Wrapf(err, "problem reading generated from response body for '%s'", td.ID)
	}
	return generated, nil
}

// CreateHost requests a new host be created
func (c *hostCommunicator) CreateHost(ctx context.Context, td TaskData, options apimodels.CreateHost) ([]string, error) {
	info := requestInfo{
		method:   http.MethodPost,
		taskData: &td,
		version:  apiVersion2,
	}
	info.path = fmt.Sprintf("hosts/%s/create", td.ID)
	resp, err := c.retryRequest(ctx, info, options)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to send create.host request for task %s: %s", td.ID, err.Error())
	}
	defer resp.Body.Close()

	ids := []string{}
	if err = utility.ReadJSON(resp.Body, &ids); err != nil {
		return nil, errors.Wrap(err, "problem reading ids from `create.host` response")
	}
	return ids, nil
}

func (c *hostCommunicator) ListHosts(ctx context.Context, td TaskData) (restmodel.HostListResults, error) {
	info := requestInfo{
		method:   http.MethodGet,
		taskData: &td,
		version:  apiVersion2,
		path:     fmt.Sprintf("hosts/%s/list", td.ID),
	}

	result := restmodel.HostListResults{}
	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return result, utility.RespErrorf(resp, "failed to list hosts for task %s: %s", td.ID, err.Error())
	}
	defer resp.Body.Close()

	if err := utility.ReadJSON(resp.Body, &result); err != nil {
		return result, errors.Wrapf(err, "problem reading hosts from response body for '%s'", td.ID)
	}
	return result, nil
}

func (c *hostCommunicator) GetDistroByName(ctx context.Context, id string) (*restmodel.APIDistro, error) {
	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion2,
		path:    fmt.Sprintf("distros/%s", id),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "failed to get distro named %s: %s", id, err.Error())
	}
	defer resp.Body.Close()

	d := &restmodel.APIDistro{}
	if err = utility.ReadJSON(resp.Body, &d); err != nil {
		return nil, errors.Wrapf(err, "reading distro from response body for '%s'", id)
	}

	return d, nil

}

// GetDockerStatus returns status of the container for the given host
func (c *hostCommunicator) GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error) {
	info := requestInfo{
		method:  http.MethodGet,
		path:    fmt.Sprintf("hosts/%s/status", hostID),
		version: apiVersion2,
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting container status for %s", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting container status")
	}
	status := cloud.ContainerStatus{}
	if err := utility.ReadJSON(resp.Body, &status); err != nil {
		return nil, errors.Wrap(err, "problem parsing container status")
	}

	return &status, nil
}

func (c *hostCommunicator) GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error) {
	path := fmt.Sprintf("/hosts/%s/logs", hostID)
	if isError {
		path = fmt.Sprintf("%s/error", path)
	} else {
		path = fmt.Sprintf("%s/output", path)
	}
	if !utility.IsZeroTime(startTime) && !utility.IsZeroTime(endTime) {
		path = fmt.Sprintf("%s?start_time=%s&end_time=%s", path, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	} else if !utility.IsZeroTime(startTime) {
		path = fmt.Sprintf("%s?start_time=%s", path, startTime.Format(time.RFC3339))
	} else if !utility.IsZeroTime(endTime) {
		path = fmt.Sprintf("%s?end_time=%s", path, endTime.Format(time.RFC3339))
	}

	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion2,
		path:    path,
	}
	resp, err := c.request(ctx, info, "")
	if err != nil {
		return nil, errors.Wrapf(err, "problem getting logs for container _id %s", hostID)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "getting logs for container id '%s'", hostID)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response")
	}

	return body, nil
}

func (c *hostCommunicator) ConcludeMerge(ctx context.Context, patchId, status string, td TaskData) error {
	info := requestInfo{
		method:   http.MethodPost,
		path:     fmt.Sprintf("commit_queue/%s/conclude_merge", patchId),
		version:  apiVersion2,
		taskData: &td,
	}
	body := struct {
		Status string `json:"status"`
	}{
		Status: status,
	}
	resp, err := c.request(ctx, info, body)
	if err != nil {
		return errors.Wrapf(err, "error concluding merge")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return utility.RespErrorf(resp, "error concluding merge")
	}

	return nil
}

func (c *hostCommunicator) GetAdditionalPatches(ctx context.Context, patchId string, td TaskData) ([]string, error) {
	info := requestInfo{
		method:   http.MethodGet,
		path:     fmt.Sprintf("commit_queue/%s/additional", patchId),
		version:  apiVersion2,
		taskData: &td,
	}
	resp, err := c.request(ctx, info, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting additional patches")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, utility.RespErrorf(resp, "error getting additional patches")
	}
	patches := []string{}
	if err := utility.ReadJSON(resp.Body, &patches); err != nil {
		return nil, errors.Wrap(err, "problem parsing response")
	}

	return patches, nil
}
