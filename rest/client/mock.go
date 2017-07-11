package client

import (
	"errors"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"golang.org/x/net/context"
)

// Mock mocks EvergreenREST for testing.
type Mock struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	serverURL    string
	httpClient   *http.Client

	// these fields have setters
	hostID     string
	hostSecret string
	apiUser    string
	apiKey     string

	// mock behavior
	ShouldFail        bool
	loggingShouldFail bool
	NextTask          *apimodels.NextTaskResponse

	// data collected by mocked methods
	logMessages map[string][]apimodels.LogMessage
}

// NewMock returns a Communicator for testing.
func NewMock(serverURL string) Communicator {
	evergreen := &Mock{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		logMessages:  make(map[string][]apimodels.LogMessage),
		serverURL:    serverURL,
		httpClient:   &http.Client{},
		ShouldFail:   false,
	}
	return evergreen
}

// StartTask returns nil.
func (c *Mock) StartTask(ctx context.Context, taskData TaskData) error {
	return nil
}

// EndTask returns an empty EndTaskResponse.
func (c *Mock) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	return &apimodels.EndTaskResponse{}, nil
}

// GetTask returns an empty Task.
func (c *Mock) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	return &task.Task{}, nil
}

// GetProjectRef returns an empty ProjectRef.
func (c *Mock) GetProjectRef(ctx context.Context, taskData TaskData) (*model.ProjectRef, error) {
	return &model.ProjectRef{}, nil
}

// GetDistro returns an empty Distro.
func (c *Mock) GetDistro(ctx context.Context, taskData TaskData) (*distro.Distro, error) {
	return &distro.Distro{}, nil
}

// GetVersion return an empty Version.
func (c *Mock) GetVersion(ctx context.Context, taskData TaskData) (*version.Version, error) {
	return &version.Version{}, nil
}

// Heartbeat returns false, which indicates the heartbeat has succeeded.
func (c *Mock) Heartbeat(ctx context.Context, taskData TaskData) (bool, error) {
	return false, nil
}

// FetchExpansionVars returns an empty ExpansionVars.
func (c *Mock) FetchExpansionVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{}, nil
}

// GetNextTask returns a mock NextTaskResponse.
func (c *Mock) GetNextTask(ctx context.Context) (*apimodels.NextTaskResponse, error) {
	if c.ShouldFail == true {
		return nil, errors.New("ShouldFail is true")
	}
	if c.NextTask != nil {
		return c.NextTask, nil
	}
	return &apimodels.NextTaskResponse{
		TaskId:     "mock_task_id",
		TaskSecret: "mock_task_secret",
		ShouldExit: false,
		Message:    "mock message",
	}, nil
}

// SendTaskLogMessages posts tasks messages to the api server
func (c *Mock) SendTaskLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	if c.loggingShouldFail {
		return errors.New("logging failed")
	}

	c.logMessages[taskData.ID] = append(c.logMessages[taskData.ID], msgs...)

	return nil
}

// GetLoggerProducer constructs a single channel log producer.
func (c *Mock) GetLoggerProducer(taskData TaskData) LoggerProducer {
	return NewSingleChannelLogHarness(taskData.ID, newLogSender(c, apimodels.AgentLogPrefix, taskData))
}

func (c *Mock) GetPatchFile(ctx context.Context, td TaskData, patchFileID string) (string, error) {
	return "", nil
}

func (c *Mock) GetTaskPatch(ctx context.Context, td TaskData) (*patch.Patch, error) {
	return nil, nil
}

func (c *Mock) SendTaskResults(ctx context.Context, td TaskData, r *task.TestResults) error {
	return nil
}

func (c *Mock) SendTestLog(ctx context.Context, td TaskData, l *model.TestLog) (string, error) {
	return "", nil
}

// GetAllHosts ...
func (*Mock) GetAllHosts() {
	return
}

// GetHostByID ...
func (*Mock) GetHostByID() {
	return
}

// SetHostStatus ...
func (*Mock) SetHostStatus() {
	return
}

// SetHostStatuses ...
func (*Mock) SetHostStatuses() {
	return
}

// CreateSpawnHost ...
func (*Mock) CreateSpawnHost() {
	return
}

// GetSpawnHosts ...
func (*Mock) GetSpawnHosts() {
	return
}

// GetTaskByID ...
func (*Mock) GetTaskByID() {
	return
}

// GetTasksByBuild ...
func (*Mock) GetTasksByBuild() {
	return
}

// GetTasksByProjectAndCommit ...
func (*Mock) GetTasksByProjectAndCommit() {
	return
}

// SetTaskStatus ...
func (*Mock) SetTaskStatus() {
	return
}

// AbortTask ...
func (*Mock) AbortTask() {
	return
}

// RestartTask ...
func (*Mock) RestartTask() {
	return
}

// GetKeys ...
func (*Mock) GetKeys() {
	return
}

// AddKey ...
func (*Mock) AddKey() {
	return
}

// RemoveKey ...
func (*Mock) RemoveKey() {
	return
}

// GetProjectByID ...
func (*Mock) GetProjectByID() {
	return
}

// EditProject ...
func (*Mock) EditProject() {
	return
}

// CreateProject ...
func (*Mock) CreateProject() {
	return
}

// GetAllProjects ...
func (*Mock) GetAllProjects() {
	return
}

// GetBuildByID ...
func (*Mock) GetBuildByID() {
	return
}

// GetBuildByProjectAndHashAndVariant ...
func (*Mock) GetBuildByProjectAndHashAndVariant() {
	return
}

// GetBuildsByVersion ...
func (*Mock) GetBuildsByVersion() {
	return
}

// SetBuildStatus ...
func (*Mock) SetBuildStatus() {
	return
}

// AbortBuild ...
func (*Mock) AbortBuild() {
	return
}

// RestartBuild ...
func (*Mock) RestartBuild() {
	return
}

// GetTestsByTaskID ...
func (*Mock) GetTestsByTaskID() {
	return
}

// GetTestsByBuild ...
func (*Mock) GetTestsByBuild() {
	return
}

// GetTestsByTestName ...
func (*Mock) GetTestsByTestName() {
	return
}

// GetVersionByID ...
func (*Mock) GetVersionByID() {
	return
}

// GetVersions ...
func (*Mock) GetVersions() {
	return
}

// GetVersionByProjectAndCommit ...
func (*Mock) GetVersionByProjectAndCommit() {
	return
}

// GetVersionsByProject ...
func (*Mock) GetVersionsByProject() {
	return
}

// SetVersionStatus ...
func (*Mock) SetVersionStatus() {
	return
}

// AbortVersion ...
func (*Mock) AbortVersion() {
	return
}

// RestartVersion ...
func (*Mock) RestartVersion() {
	return
}

// GetAllDistros ...
func (*Mock) GetAllDistros() {
	return
}

// GetDistroByID ...
func (*Mock) GetDistroByID() {
	return
}

// CreateDistro ...
func (*Mock) CreateDistro() {
	return
}

// EditDistro ...
func (*Mock) EditDistro() {
	return
}

// DeleteDistro ...
func (*Mock) DeleteDistro() {
	return
}

// GetDistroSetupScriptByID ...
func (*Mock) GetDistroSetupScriptByID() {
	return
}

// GetDistroTeardownScriptByID ...
func (*Mock) GetDistroTeardownScriptByID() {
	return
}

// EditDistroSetupScript ...
func (*Mock) EditDistroSetupScript() {
	return
}

// EditDistroTeardownScript ...
func (*Mock) EditDistroTeardownScript() {
	return
}

// GetPatchByID ...
func (*Mock) GetPatchByID() {
	return
}

// GetPatchesByProject ...
func (*Mock) GetPatchesByProject() {
	return
}

// SetPatchStatus ...
func (*Mock) SetPatchStatus() {
	return
}

// AbortPatch ...
func (*Mock) AbortPatch() {
	return
}

// RestartPatch ...
func (*Mock) RestartPatch() {
	return
}

// SetTimeoutStart sets the initial timeout for a request.
func (c *Mock) SetTimeoutStart(timeoutStart time.Duration) {
	c.timeoutStart = timeoutStart
}

// SetTimeoutMax sets the maximum timeout for a request.
func (c *Mock) SetTimeoutMax(timeoutMax time.Duration) {
	c.timeoutMax = timeoutMax
}

// SetMaxAttempts sets the number of attempts a request will be made.
func (c *Mock) SetMaxAttempts(attempts int) {
	c.maxAttempts = attempts
}

// SetHostID sets the host ID.
func (c *Mock) SetHostID(hostID string) {
	c.hostID = hostID
}

// SetHostSecret sets the host secret.
func (c *Mock) SetHostSecret(hostSecret string) {
	c.hostSecret = hostSecret
}

// SetAPIUser sets the API user.
func (c *Mock) SetAPIUser(apiUser string) {
	c.apiUser = apiUser
}

// SetAPIKey sets the API key.
func (c *Mock) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// PostJSON does an HTTP POST for the communicator's plugin + task.
func (c *Mock) PostJSON(ctx context.Context, taskData TaskData, pluginName, endpoint string, data interface{}) (*http.Response, error) {
	return nil, nil
}

// GetJSON does an HTTP GET for the communicator's plugin + task.
func (c *Mock) GetJSON(ctx context.Context, taskData TaskData, pluginName, endpoint string) (*http.Response, error) {
	return nil, nil
}

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *Mock) SendResults(ctx context.Context, taskData TaskData, results *task.TestResults) error {
	return nil
}

// SendFiles attaches task files.
func (c *Mock) SendFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	return nil
}

// PostTestData posts a test log for a communicator's task. Is a
// noop if the test Log is nil.
func (c *Mock) PostTestData(ctx context.Context, taskData TaskData, log *model.TestLog) (string, error) {
	return "", nil
}
