package client

import (
	"net/http"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
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
	NextTaskShouldFail     bool
	GetPatchFileShouldFail bool
	loggingShouldFail      bool
	NextTaskResponse       *apimodels.NextTaskResponse
	EndTaskResponse        *apimodels.EndTaskResponse
	EndTaskShouldFail      bool

	// data collected by mocked methods
	logMessages map[string][]apimodels.LogMessage
	PatchFiles  map[string]string
	keyVal      map[string]*serviceModel.KeyVal
	mu          sync.Mutex
}

// NewMock returns a Communicator for testing.
func NewMock(serverURL string) *Mock {
	return &Mock{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		logMessages:  make(map[string][]apimodels.LogMessage),
		PatchFiles:   make(map[string]string),
		keyVal:       make(map[string]*serviceModel.KeyVal),
		serverURL:    serverURL,
		httpClient:   &http.Client{},
	}
}

// StartTask returns nil.
func (c *Mock) StartTask(ctx context.Context, taskData TaskData) error {
	return nil
}

// EndTask returns an empty EndTaskResponse.
func (c *Mock) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskData TaskData) (*apimodels.EndTaskResponse, error) {
	if c.EndTaskShouldFail {
		return nil, errors.New("end task should fail")
	}
	if c.EndTaskResponse != nil {
		return c.EndTaskResponse, nil
	}
	return &apimodels.EndTaskResponse{}, nil
}

// GetTask returns an empty Task.
func (c *Mock) GetTask(ctx context.Context, taskData TaskData) (*task.Task, error) {
	return &task.Task{}, nil
}

// GetProjectRef returns an empty ProjectRef.
func (c *Mock) GetProjectRef(ctx context.Context, taskData TaskData) (*serviceModel.ProjectRef, error) {
	return &serviceModel.ProjectRef{}, nil
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
	if c.NextTaskShouldFail == true {
		return nil, errors.New("NextTaskShouldFail is true")
	}
	if c.NextTaskResponse != nil {
		return c.NextTaskResponse, nil
	}

	return &apimodels.NextTaskResponse{
		TaskId:     "mock_task_id",
		TaskSecret: "mock_task_secret",
		ShouldExit: false,
		Message:    "mock message",
	}, nil
}

// SendTaskLogMessages posts tasks messages to the api server
func (c *Mock) SendLogMessages(ctx context.Context, taskData TaskData, msgs []apimodels.LogMessage) error {
	if c.loggingShouldFail {
		return errors.New("logging failed")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.logMessages[taskData.ID] = append(c.logMessages[taskData.ID], msgs...)

	return nil
}

// GetLoggerProducer constructs a single channel log producer.
func (c *Mock) GetLoggerProducer(taskData TaskData) LoggerProducer {
	return NewSingleChannelLogHarness(taskData.ID, newLogSender(c, apimodels.AgentLogPrefix, taskData))
}

func (c *Mock) GetPatchFile(ctx context.Context, td TaskData, patchFileID string) (string, error) {
	if c.GetPatchFileShouldFail {
		return "", errors.New("operation run in fail mode.")
	}

	out, ok := c.PatchFiles[patchFileID]

	if !ok {
		return "", errors.Errorf("patch file %s not found", patchFileID)
	}

	return out, nil
}

func (c *Mock) GetTaskPatch(ctx context.Context, td TaskData) (*patchmodel.Patch, error) {
	return &patchmodel.Patch{}, nil
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

// CreateSpawnHost will return a mock host that would have been intended
func (*Mock) CreateSpawnHost(ctx context.Context, distroID string, keyName string) (*model.SpawnHost, error) {
	mockHost := &model.SpawnHost{
		HostID:         model.APIString("mock_host_id"),
		DistroID:       model.APIString("mock_distro_id"),
		Type:           model.APIString("mock_type"),
		ExpirationTime: model.APITime(time.Now()),
		CreationTime:   model.APITime(time.Now()),
		Status:         model.APIString("starting"),
		StartedBy:      model.APIString("mock_user"),
		Tag:            model.APIString("mock_tag"),
		Project:        model.APIString("mock_project"),
		Zone:           model.APIString("mock_zone"),
		UserHost:       true,
		Provisioned:    true,
	}
	return mockHost, nil
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

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *Mock) SendTestResults(ctx context.Context, taskData TaskData, results *task.TestResults) error {
	return nil
}

// SendFiles attaches task files.
func (c *Mock) AttachFiles(ctx context.Context, taskData TaskData, taskFiles []*artifact.File) error {
	return nil
}

// SendTestLog posts a test log for a communicator's task. Is a
// noop if the test Log is nil.
func (c *Mock) SendTestLog(ctx context.Context, taskData TaskData, log *serviceModel.TestLog) (string, error) {
	return "", nil
}

func (c *Mock) GetManifest(ctx context.Context, td TaskData) (*manifest.Manifest, error) {
	return &manifest.Manifest{}, nil
}

func (c *Mock) S3Copy(ctx context.Context, td TaskData, req *apimodels.S3CopyRequest) error {
	return nil
}

func (c *Mock) KeyValInc(ctx context.Context, td TaskData, kv *serviceModel.KeyVal) error {
	if cached, ok := c.keyVal[kv.Key]; ok {
		*kv = *cached
	} else {
		c.keyVal[kv.Key] = kv
	}
	kv.Value++
	return nil
}

func (c *Mock) PostJSONData(ctx context.Context, td TaskData, path string, data interface{}) error {
	return nil
}
func (c *Mock) GetJSONData(ctx context.Context, td TaskData, tn, dn, vn string) ([]byte, error) {
	return nil, nil
}
func (c *Mock) GetJSONHistory(ctx context.Context, td TaskData, tags bool, tn, dn string) ([]byte, error) {
	return nil, nil
}
