package client

import (
	"errors"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"golang.org/x/net/context"
)

// MockEvergreenREST mocks EvergreenREST for testing.
type MockEvergreenREST struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration

	// these fields have setters
	hostID     string
	hostSecret string
	apiUser    string
	apiKey     string

	// mock behavior flags
	loggingShouldFail bool

	// data collected by mocked methods
	logMessages map[string][]apimodels.LogMessage
}

// NewMockEvergreenREST returns a Communicator for testing.
func NewMockEvergreenREST() Communicator {
	return &MockEvergreenREST{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		logMessages:  make(map[string][]apimodels.LogMessage),
	}
}

// StartTask returns nil.
func (c *MockEvergreenREST) StartTask(ctx context.Context, taskID, taskSecret string) error {
	return nil
}

// EndTask returns an empty EndTaskResponse.
func (c *MockEvergreenREST) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, taskID, taskSecret string) (*apimodels.EndTaskResponse, error) {
	return &apimodels.EndTaskResponse{}, nil
}

// GetTask returns an empty Task.
func (c *MockEvergreenREST) GetTask(ctx context.Context, taskID, taskSecret string) (*task.Task, error) {
	return &task.Task{}, nil
}

// GetProjectRef returns an empty ProjectRef.
func (c *MockEvergreenREST) GetProjectRef(ctx context.Context, taskID, taskSecret string) (*model.ProjectRef, error) {
	return &model.ProjectRef{}, nil
}

// GetDistro returns an empty Distro.
func (c *MockEvergreenREST) GetDistro(ctx context.Context, taskID, taskSecret string) (*distro.Distro, error) {
	return &distro.Distro{}, nil
}

// GetVersion return an empty Version.
func (c *MockEvergreenREST) GetVersion(ctx context.Context, taskID, taskSecret string) (*version.Version, error) {
	return &version.Version{}, nil
}

// Heartbeat returns false, which indicates the heartbeat has succeeded.
func (c *MockEvergreenREST) Heartbeat(ctx context.Context, taskID, taskSecret string) (bool, error) {
	return false, nil
}

// FetchExpansionVars returns an empty ExpansionVars.
func (c *MockEvergreenREST) FetchExpansionVars(ctx context.Context, taskID, taskSecret string) (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{}, nil
}

// GetNextTask returns an empty NextTaskResponse.
func (c *MockEvergreenREST) GetNextTask(ctx context.Context, taskID, taskSecret string) (*apimodels.NextTaskResponse, error) {
	return &apimodels.NextTaskResponse{}, nil

}

// SendTaskLogMessages posts tasks messages to the api server
func (c *MockEvergreenREST) SendTaskLogMessages(ctx context.Context, taskID, taskSecret string, msgs []apimodels.LogMessage) error {
	if c.loggingShouldFail {
		return errors.New("logging failed")
	}

	c.logMessages[taskID] = append(c.logMessages[taskID], msgs...)

	return nil
}

// GetLoggerProducer constructs a single channel log producer.
func (c *MockEvergreenREST) GetLoggerProducer(taskID, taskSecret string) LoggerProducer {
	return NewSingleChannelLogHarness(taskID, newLogSender(c, apimodels.AgentLogPrefix, taskID, taskSecret))
}

// GetAllHosts ...
func (*MockEvergreenREST) GetAllHosts() {
	return
}

// GetHostByID ...
func (*MockEvergreenREST) GetHostByID() {
	return
}

// SetHostStatus ...
func (*MockEvergreenREST) SetHostStatus() {
	return
}

// SetHostStatuses ...
func (*MockEvergreenREST) SetHostStatuses() {
	return
}

// CreateSpawnHost ...
func (*MockEvergreenREST) CreateSpawnHost() {
	return
}

// GetSpawnHosts ...
func (*MockEvergreenREST) GetSpawnHosts() {
	return
}

// GetTaskByID ...
func (*MockEvergreenREST) GetTaskByID() {
	return
}

// GetTasksByBuild ...
func (*MockEvergreenREST) GetTasksByBuild() {
	return
}

// GetTasksByProjectAndCommit ...
func (*MockEvergreenREST) GetTasksByProjectAndCommit() {
	return
}

// SetTaskStatus ...
func (*MockEvergreenREST) SetTaskStatus() {
	return
}

// AbortTask ...
func (*MockEvergreenREST) AbortTask() {
	return
}

// RestartTask ...
func (*MockEvergreenREST) RestartTask() {
	return
}

// GetKeys ...
func (*MockEvergreenREST) GetKeys() {
	return
}

// AddKey ...
func (*MockEvergreenREST) AddKey() {
	return
}

// RemoveKey ...
func (*MockEvergreenREST) RemoveKey() {
	return
}

// GetProjectByID ...
func (*MockEvergreenREST) GetProjectByID() {
	return
}

// EditProject ...
func (*MockEvergreenREST) EditProject() {
	return
}

// CreateProject ...
func (*MockEvergreenREST) CreateProject() {
	return
}

// GetAllProjects ...
func (*MockEvergreenREST) GetAllProjects() {
	return
}

// GetBuildByID ...
func (*MockEvergreenREST) GetBuildByID() {
	return
}

// GetBuildByProjectAndHashAndVariant ...
func (*MockEvergreenREST) GetBuildByProjectAndHashAndVariant() {
	return
}

// GetBuildsByVersion ...
func (*MockEvergreenREST) GetBuildsByVersion() {
	return
}

// SetBuildStatus ...
func (*MockEvergreenREST) SetBuildStatus() {
	return
}

// AbortBuild ...
func (*MockEvergreenREST) AbortBuild() {
	return
}

// RestartBuild ...
func (*MockEvergreenREST) RestartBuild() {
	return
}

// GetTestsByTaskID ...
func (*MockEvergreenREST) GetTestsByTaskID() {
	return
}

// GetTestsByBuild ...
func (*MockEvergreenREST) GetTestsByBuild() {
	return
}

// GetTestsByTestName ...
func (*MockEvergreenREST) GetTestsByTestName() {
	return
}

// GetVersionByID ...
func (*MockEvergreenREST) GetVersionByID() {
	return
}

// GetVersions ...
func (*MockEvergreenREST) GetVersions() {
	return
}

// GetVersionByProjectAndCommit ...
func (*MockEvergreenREST) GetVersionByProjectAndCommit() {
	return
}

// GetVersionsByProject ...
func (*MockEvergreenREST) GetVersionsByProject() {
	return
}

// SetVersionStatus ...
func (*MockEvergreenREST) SetVersionStatus() {
	return
}

// AbortVersion ...
func (*MockEvergreenREST) AbortVersion() {
	return
}

// RestartVersion ...
func (*MockEvergreenREST) RestartVersion() {
	return
}

// GetAllDistros ...
func (*MockEvergreenREST) GetAllDistros() {
	return
}

// GetDistroByID ...
func (*MockEvergreenREST) GetDistroByID() {
	return
}

// CreateDistro ...
func (*MockEvergreenREST) CreateDistro() {
	return
}

// EditDistro ...
func (*MockEvergreenREST) EditDistro() {
	return
}

// DeleteDistro ...
func (*MockEvergreenREST) DeleteDistro() {
	return
}

// GetDistroSetupScriptByID ...
func (*MockEvergreenREST) GetDistroSetupScriptByID() {
	return
}

// GetDistroTeardownScriptByID ...
func (*MockEvergreenREST) GetDistroTeardownScriptByID() {
	return
}

// EditDistroSetupScript ...
func (*MockEvergreenREST) EditDistroSetupScript() {
	return
}

// EditDistroTeardownScript ...
func (*MockEvergreenREST) EditDistroTeardownScript() {
	return
}

// GetPatchByID ...
func (*MockEvergreenREST) GetPatchByID() {
	return
}

// GetPatchesByProject ...
func (*MockEvergreenREST) GetPatchesByProject() {
	return
}

// SetPatchStatus ...
func (*MockEvergreenREST) SetPatchStatus() {
	return
}

// AbortPatch ...
func (*MockEvergreenREST) AbortPatch() {
	return
}

// RestartPatch ...
func (*MockEvergreenREST) RestartPatch() {
	return
}

// SetTimeoutStart sets the initial timeout for a request.
func (c *MockEvergreenREST) SetTimeoutStart(timeoutStart time.Duration) {
	c.timeoutStart = timeoutStart
}

// SetTimeoutMax sets the maximum timeout for a request.
func (c *MockEvergreenREST) SetTimeoutMax(timeoutMax time.Duration) {
	c.timeoutMax = timeoutMax
}

// SetMaxAttempts sets the number of attempts a request will be made.
func (c *MockEvergreenREST) SetMaxAttempts(attempts int) {
	c.maxAttempts = attempts
}

// SetHostID sets the host ID.
func (c *MockEvergreenREST) SetHostID(hostID string) {
	c.hostID = hostID
}

// SetHostSecret sets the host secret.
func (c *MockEvergreenREST) SetHostSecret(hostSecret string) {
	c.hostSecret = hostSecret
}

// SetAPIUser sets the API user.
func (c *MockEvergreenREST) SetAPIUser(apiUser string) {
	c.apiUser = apiUser
}

// SetAPIKey sets the API key.
func (c *MockEvergreenREST) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}
