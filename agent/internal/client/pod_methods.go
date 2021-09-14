package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
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

// StartTask marks the task as started.
func (c *podCommunicator) StartTask(ctx context.Context, taskData TaskData) error {
	return errors.New("TODO: implement")
}

// EndTask marks the task as finished with the given status
func (c *podCommunicator) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	return nil, errors.New("TODO: implement")
}

// GetTask returns the active task.
func (c *podCommunicator) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	return nil, errors.New("TODO: implement")
}

// GetDisplayTaskInfoFromExecution returns the display task info associated
// with the execution task.
func (c *podCommunicator) GetDisplayTaskInfoFromExecution(ctx context.Context, td TaskData) (*apimodels.DisplayTaskInfo, error) {
	return nil, errors.New("TODO: implement")
}

// GetProjectRef loads the task's project.
func (c *podCommunicator) GetProjectRef(ctx context.Context, taskData TaskData) (*model.ProjectRef, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetDistroView(ctx context.Context, taskData TaskData) (*apimodels.DistroView, error) {
	return nil, errors.New("TODO: implement")
}

// GetDistroAMI returns the distro for the task.
func (c *podCommunicator) GetDistroAMI(ctx context.Context, distro, region string, taskData TaskData) (string, error) {
	return "", errors.New("TODO: implement")
}

func (c *podCommunicator) GetProject(ctx context.Context, taskData TaskData) (*model.Project, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetExpansions(ctx context.Context, taskData TaskData) (util.Expansions, error) {
	return nil, errors.New("TODO: implement")
}

// Heartbeat sends a heartbeat to the API server. The server can respond with
// an "abort" response. This function returns true if the agent should abort.
func (c *podCommunicator) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	return false, errors.New("TODO: implement")
}

// FetchExpansionVars loads expansions for a communicator's task from the API server.
func (c *podCommunicator) FetchExpansionVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionVars, error) {
	return nil, errors.New("TODO: implement")
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

// DisableHost signals to the app server that the pod should be disabled.
func (c *podCommunicator) DisableHost(ctx context.Context, hostID string, details apimodels.DisableInfo) error {
	return errors.New("TODO: implement")
}

// GetCedarConfig returns the cedar service information including the base URL,
// URL, RPC port, and credentials.
func (c *podCommunicator) GetCedarConfig(ctx context.Context) (*apimodels.CedarConfig, error) {
	info := requestInfo{
		method:  http.MethodGet,
		version: apiVersion2,
		path:    fmt.Sprintf("pods/%s/agent/cedar_config", c.podID),
	}

	resp, err := c.retryRequest(ctx, info, nil)
	if err != nil {
		return nil, utility.RespErrorf(resp, "getting cedar config: %s", err.Error())
	}

	var cc apimodels.CedarConfig
	if err := utility.ReadJSON(resp.Body, &cc); err != nil {
		return nil, errors.Wrap(err, "reading cedar config from response")
	}

	return &cc, nil
}

// GetCedarGRPCConn returns the client connection to cedar if it exists, or
// creates it if it doesn't exist.
func (c *podCommunicator) GetCedarGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	if err := c.createCedarGRPCConn(ctx, c); err != nil {
		return nil, errors.Wrap(err, "setting up cedar grpc connection")
	}
	return c.cedarGRPCClient, nil
}

func (c *podCommunicator) GetLoggerProducer(ctx context.Context, td TaskData, config *LoggerConfig) (LoggerProducer, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) makeSender(ctx context.Context, td TaskData, opts []LogOpts, prefix string, logType string) (send.Sender, []send.Sender, error) {
	return nil, nil, errors.New("TODO: implement")
}

// SendLogMessages posts a group of log messages for a task.
func (c *podCommunicator) SendLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	return errors.New("TODO: implement")
}

// SendTaskResults posts a task's results, used by the attach results operations.
func (c *podCommunicator) SendTaskResults(ctx context.Context, taskData TaskData, r *task.LocalTestResults) error {
	return errors.New("TODO: implement")
}

// GetPatch tries to get the patch data from the server in json format,
// and unmarhals it into a patch struct. The GET request is attempted
// multiple times upon failure. If patchId is not specified, the task's
// patch is returned
func (c *podCommunicator) GetTaskPatch(ctx context.Context, taskData TaskData, patchId string) (*patchmodel.Patch, error) {
	return nil, errors.New("TODO: implement")
}

// GetPatchFiles is used by the git.get_project plugin and fetches
// patches from the database, used in patch builds.
func (c *podCommunicator) GetPatchFile(ctx context.Context, taskData TaskData, patchFileID string) (string, error) {
	return "", errors.New("TODO: implement")
}

// SendTestLog is used by the attach plugin to add to the test_logs
// collection for log data associated with a test.
func (c *podCommunicator) SendTestLog(ctx context.Context, taskData TaskData, log *model.TestLog) (string, error) {
	return "", errors.New("TODO: implement")
}

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *podCommunicator) SendTestResults(ctx context.Context, taskData TaskData, results *task.LocalTestResults) error {
	return errors.New("TODO: implement")
}

// SetHasCedarResults sets the HasCedarResults flag to true in the given task
// in the database.
func (c *podCommunicator) SetHasCedarResults(ctx context.Context, taskData TaskData, failed bool) error {
	return errors.New("TODO: implement")
}

// AttachFiles attaches task files.
func (c *podCommunicator) AttachFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) SetDownstreamParams(ctx context.Context, downstreamParams []patchmodel.Parameter, taskData TaskData) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) GetManifest(ctx context.Context, taskData TaskData) (*manifest.Manifest, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) S3Copy(ctx context.Context, taskData TaskData, req *apimodels.S3CopyRequest) (string, error) {
	return "", errors.New("TODO: implement")
}

func (c *podCommunicator) KeyValInc(ctx context.Context, taskData TaskData, kv *model.KeyVal) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) PostJSONData(ctx context.Context, taskData TaskData, path string, data interface{}) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) GetJSONData(ctx context.Context, taskData TaskData, taskName, dataName, variantName string) ([]byte, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetJSONHistory(ctx context.Context, taskData TaskData, tags bool, taskName, dataName string) ([]byte, error) {
	return nil, errors.New("TODO: implement")
}

// GenerateTasks posts new tasks for the `generate.tasks` command.
func (c *podCommunicator) GenerateTasks(ctx context.Context, td TaskData, jsonBytes []json.RawMessage) error {
	return errors.New("TODO: implement")
}

// GenerateTasksPoll posts new tasks for the `generate.tasks` command.
func (c *podCommunicator) GenerateTasksPoll(ctx context.Context, td TaskData) (*apimodels.GeneratePollResponse, error) {
	return nil, errors.New("TODO: implement")
}

// CreateHost requests a new host be created
func (c *podCommunicator) CreateHost(ctx context.Context, td TaskData, options apimodels.CreateHost) ([]string, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) ListHosts(ctx context.Context, td TaskData) (restmodel.HostListResults, error) {
	return restmodel.HostListResults{}, errors.New("TODO: implement")
}

func (c *podCommunicator) GetDistroByName(ctx context.Context, id string) (*restmodel.APIDistro, error) {
	return nil, errors.New("TODO: implement")
}

// GetDockerStatus returns status of the container for the given host
func (c *podCommunicator) GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error) {
	return nil, errors.New("TODO: implement")
}

func (c *podCommunicator) ConcludeMerge(ctx context.Context, patchId, status string, td TaskData) error {
	return errors.New("TODO: implement")
}

func (c *podCommunicator) GetAdditionalPatches(ctx context.Context, patchId string, td TaskData) ([]string, error) {
	return nil, errors.New("TODO: implement")
}
