package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Mock mocks the Communicator for testing.
type Mock struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	serverURL    string

	// these fields have setters
	hostID     string
	hostSecret string

	// mock behavior
	NextTaskShouldFail          bool
	NextTaskShouldConflict      bool
	GetPatchFileShouldFail      bool
	loggingShouldFail           bool
	NextTaskResponse            *apimodels.NextTaskResponse
	NextTaskIsNil               bool
	EndTaskResponse             *apimodels.EndTaskResponse
	EndTaskShouldFail           bool
	EndTaskResult               endTaskResult
	ShellExecFilename           string
	TimeoutFilename             string
	HeartbeatShouldAbort        bool
	HeartbeatShouldErr          bool
	HeartbeatShouldSometimesErr bool
	TaskExecution               int
	CreatedHost                 apimodels.CreateHost

	CedarGRPCConn *grpc.ClientConn

	AttachedFiles      map[string][]*artifact.File
	LogID              string
	LocalTestResults   *task.LocalTestResults
	HasCedarResults    bool
	CedarResultsFailed bool
	TestLogs           []*serviceModel.TestLog
	TestLogCount       int

	// data collected by mocked methods
	logMessages      map[string][]apimodels.LogMessage
	PatchFiles       map[string]string
	keyVal           map[string]*serviceModel.KeyVal
	LastMessageSent  time.Time
	DownstreamParams []patchmodel.Parameter

	mu sync.RWMutex
}

type endTaskResult struct {
	Detail   *apimodels.TaskEndDetail
	TaskData TaskData
}

// NewMock returns a Communicator for testing.
func NewMock(serverURL string) *Mock {
	return &Mock{
		maxAttempts:   defaultMaxAttempts,
		timeoutStart:  defaultTimeoutStart,
		timeoutMax:    defaultTimeoutMax,
		logMessages:   make(map[string][]apimodels.LogMessage),
		PatchFiles:    make(map[string]string),
		keyVal:        make(map[string]*serviceModel.KeyVal),
		AttachedFiles: make(map[string][]*artifact.File),
		serverURL:     serverURL,
	}
}

func (c *Mock) Close() {}

func (c *Mock) LastMessageAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastMessageSent
}

func (c *Mock) UpdateLastMessageTime() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastMessageSent = time.Now()
}

func (c *Mock) SetTimeoutStart(timeoutStart time.Duration) { c.timeoutStart = timeoutStart }
func (c *Mock) SetTimeoutMax(timeoutMax time.Duration)     { c.timeoutMax = timeoutMax }
func (c *Mock) SetMaxAttempts(attempts int)                { c.maxAttempts = attempts }

func (c *Mock) GetAgentSetupData(ctx context.Context) (*apimodels.AgentSetupData, error) {
	return &apimodels.AgentSetupData{}, nil
}

func (c *Mock) StartTask(ctx context.Context, td TaskData) error { return nil }

// EndTask returns a mock EndTaskResponse.
func (c *Mock) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, td TaskData) (*apimodels.EndTaskResponse, error) {
	if c.EndTaskShouldFail {
		return nil, errors.New("end task should fail")
	}
	if c.EndTaskResponse != nil {
		return c.EndTaskResponse, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.EndTaskResult.Detail = detail
	c.EndTaskResult.TaskData = td
	return &apimodels.EndTaskResponse{}, nil
}

// GetEndTaskDetail returns the task end detail saved in the mock.
func (c *Mock) GetEndTaskDetail() *apimodels.TaskEndDetail {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.EndTaskResult.Detail
}

// GetTask returns a mock Task.
func (c *Mock) GetTask(ctx context.Context, td TaskData) (*task.Task, error) {
	return &task.Task{
		Id:           "mock_task_id",
		Secret:       "mock_task_secret",
		BuildVariant: "mock_build_variant",
		DisplayName:  "build",
		Execution:    c.TaskExecution,
		Version:      "mock_version_id",
	}, nil
}

func (c *Mock) GetDisplayTaskInfoFromExecution(ctx context.Context, td TaskData) (*apimodels.DisplayTaskInfo, error) {
	return &apimodels.DisplayTaskInfo{
		ID:   "mock_display_task_id",
		Name: "display_task_name",
	}, nil
}

// GetProjectRef returns a mock ProjectRef.
func (c *Mock) GetProjectRef(ctx context.Context, td TaskData) (*serviceModel.ProjectRef, error) {
	return &serviceModel.ProjectRef{
		Owner:  "mock_owner",
		Repo:   "mock_repo",
		Branch: "mock_branch",
	}, nil
}

func (c *Mock) GetDistroView(context.Context, TaskData) (*apimodels.DistroView, error) {
	return &apimodels.DistroView{
		WorkDir: ".",
	}, nil
}

func (c *Mock) GetDistroAMI(context.Context, string, string, TaskData) (string, error) {
	return "ami-mock", nil
}

func (c *Mock) GetProject(ctx context.Context, td TaskData) (*serviceModel.Project, error) {
	var err error
	var data []byte
	_, file, _, _ := runtime.Caller(0)

	data, err = ioutil.ReadFile(filepath.Join(filepath.Dir(file), "testdata", fmt.Sprintf("%s.yaml", td.ID)))
	if err != nil {
		grip.Error(err)
	}
	proj := &serviceModel.Project{}
	_, err = serviceModel.LoadProjectInto(ctx, data, nil, "", proj)
	return proj, err
}

func (c *Mock) GetExpansions(ctx context.Context, taskData TaskData) (util.Expansions, error) {
	e := util.Expansions{
		"foo": "bar",
	}
	return e, nil
}

// Heartbeat returns false, which indicates the heartbeat has succeeded.
func (c *Mock) Heartbeat(ctx context.Context, td TaskData) (bool, error) {
	if c.HeartbeatShouldAbort {
		return true, nil
	}
	if c.HeartbeatShouldSometimesErr {
		if c.HeartbeatShouldErr {
			c.HeartbeatShouldErr = false
			return false, errors.New("mock heartbeat error")
		}
		c.HeartbeatShouldErr = true
		return false, nil
	}
	if c.HeartbeatShouldErr {
		return false, errors.New("mock heartbeat error")
	}
	return false, nil
}

// FetchExpansionVars returns a mock ExpansionVars.
func (c *Mock) FetchExpansionVars(ctx context.Context, td TaskData) (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{
		Vars: map[string]string{
			"shellexec_fn":   c.ShellExecFilename,
			"timeout_fn":     c.TimeoutFilename,
			"my_new_timeout": "2",
		},
	}, nil
}

// GetNextTask returns a mock NextTaskResponse.
func (c *Mock) GetNextTask(ctx context.Context, details *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error) {
	if c.NextTaskIsNil {
		return &apimodels.NextTaskResponse{
				TaskId: "",
			},
			nil
	}
	if c.NextTaskShouldFail {
		return nil, errors.New("NextTaskShouldFail is true")
	}
	if c.NextTaskShouldConflict {
		return nil, errors.WithStack(HTTPConflictError)
	}
	if c.NextTaskResponse != nil {
		return c.NextTaskResponse, nil
	}

	return &apimodels.NextTaskResponse{
		TaskId:     "mock_task_id",
		TaskSecret: "mock_task_secret",
		ShouldExit: false,
	}, nil
}

// GetCedarConfig returns mock cedar service information.
func (c *Mock) GetCedarConfig(ctx context.Context) (*apimodels.CedarConfig, error) {
	return &apimodels.CedarConfig{
		BaseURL:  "base_url",
		RPCPort:  "1000",
		Username: "user",
		APIKey:   "api_key",
	}, nil
}

// GetCedarGRPCConn returns gRPC connection if it is set.
func (c *Mock) GetCedarGRPCConn(ctx context.Context) (*grpc.ClientConn, error) {
	if c.CedarGRPCConn == nil {
		return nil, nil
	}
	return c.CedarGRPCConn, nil
}

// SendTaskLogMessages posts tasks messages to the api server
func (c *Mock) SendLogMessages(ctx context.Context, td TaskData, msgs []apimodels.LogMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loggingShouldFail {
		return errors.New("logging failed")
	}

	c.logMessages[td.ID] = append(c.logMessages[td.ID], msgs...)

	return nil
}

// GetMockMessages returns the mock's logs.
func (c *Mock) GetMockMessages() map[string][]apimodels.LogMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := map[string][]apimodels.LogMessage{}
	for k, v := range c.logMessages {
		out[k] = []apimodels.LogMessage{}
		for _, i := range v {
			new := apimodels.LogMessage{
				Type:      i.Type,
				Severity:  i.Severity,
				Message:   i.Message,
				Timestamp: i.Timestamp,
				Version:   i.Version,
			}
			out[k] = append(out[k], new)
		}
	}

	return out
}

// GetLoggerProducer constructs a single channel log producer.
func (c *Mock) GetLoggerProducer(ctx context.Context, td TaskData, config *LoggerConfig) (LoggerProducer, error) {
	return NewSingleChannelLogHarness(td.ID, newEvergreenLogSender(ctx, c, apimodels.AgentLogPrefix, td, defaultLogBufferSize, defaultLogBufferTime)), nil
}

func (c *Mock) GetLoggerMetadata() LoggerMetadata {
	return LoggerMetadata{
		Agent: []LogkeeperMetadata{{
			Build: "build1",
			Test:  "test1",
		}},
		System: []LogkeeperMetadata{{
			Build: "build1",
			Test:  "test2",
		}},
		Task: []LogkeeperMetadata{{
			Build: "build1",
			Test:  "test3",
		}},
	}
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

func (c *Mock) GetTaskPatch(ctx context.Context, td TaskData, patchId string) (*patchmodel.Patch, error) {
	patch, ok := ctx.Value("patch").(*patchmodel.Patch)
	if !ok {
		return &patchmodel.Patch{}, nil
	}

	return patch, nil
}

// CreateSpawnHost will return a mock host that would have been intended
func (*Mock) CreateSpawnHost(ctx context.Context, spawnRequest *model.HostRequestOptions) (*model.APIHost, error) {
	mockHost := &model.APIHost{
		Id:      utility.ToStringPtr("mock_host_id"),
		HostURL: utility.ToStringPtr("mock_url"),
		Distro: model.DistroInfo{
			Id:       utility.ToStringPtr(spawnRequest.DistroID),
			Provider: utility.ToStringPtr(evergreen.ProviderNameMock),
		},
		Provider:     utility.ToStringPtr(evergreen.ProviderNameMock),
		Status:       utility.ToStringPtr(evergreen.HostUninitialized),
		StartedBy:    utility.ToStringPtr("mock_user"),
		UserHost:     true,
		Provisioned:  false,
		InstanceTags: spawnRequest.InstanceTags,
		InstanceType: utility.ToStringPtr(spawnRequest.InstanceType),
	}
	return mockHost, nil
}

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *Mock) SendTestResults(ctx context.Context, td TaskData, results *task.LocalTestResults) error {
	c.LocalTestResults = results
	return nil
}

// SetHasCedarResults sets the HasCedarResults flag in the task.
func (c *Mock) SetHasCedarResults(ctx context.Context, td TaskData, failed bool) error {
	c.HasCedarResults = true
	if failed {
		c.CedarResultsFailed = true
	}
	return nil
}

// DisableHost signals to the app server that the host should be disabled.
func (c *Mock) DisableHost(ctx context.Context, hostID string, info apimodels.DisableInfo) error {
	return nil
}

// SendFiles attaches task files.
func (c *Mock) AttachFiles(ctx context.Context, td TaskData, taskFiles []*artifact.File) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	grip.Info("attaching files")
	c.AttachedFiles[td.ID] = append(c.AttachedFiles[td.ID], taskFiles...)

	return nil
}

func (c *Mock) SetDownstreamParams(ctx context.Context, downstreamParams []patchmodel.Parameter, taskData TaskData) error {
	c.DownstreamParams = downstreamParams
	return nil
}

// SendTestLog posts a test log for a communicator's task. Is a
// noop if the test Log is nil.
func (c *Mock) SendTestLog(ctx context.Context, td TaskData, log *serviceModel.TestLog) (string, error) {
	c.TestLogs = append(c.TestLogs, log)
	c.TestLogs[len(c.TestLogs)-1].Id = c.LogID
	c.TestLogCount += 1
	c.LogID = fmt.Sprintf("%s-%d", c.LogID, c.TestLogCount)
	return c.LogID, nil
}

func (c *Mock) GetManifest(ctx context.Context, td TaskData) (*manifest.Manifest, error) {
	return &manifest.Manifest{}, nil
}

func (c *Mock) S3Copy(ctx context.Context, td TaskData, req *apimodels.S3CopyRequest) (string, error) {
	return "", nil
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

// GenerateTasks posts new tasks for the `generate.tasks` command.
func (c *Mock) GenerateTasks(ctx context.Context, td TaskData, jsonBytes []json.RawMessage) error {
	if td.ID != "mock_id" {
		return errors.New("mock failed, wrong id")
	}
	if td.Secret != "mock_secret" {
		return errors.New("mock failed, wrong secret")
	}
	return nil
}

func (c *Mock) GenerateTasksPoll(ctx context.Context, td TaskData) (*apimodels.GeneratePollResponse, error) {
	return &apimodels.GeneratePollResponse{
		Finished: true,
		Errors:   []string{},
	}, nil
}

func (c *Mock) CreateHost(ctx context.Context, td TaskData, options apimodels.CreateHost) ([]string, error) {
	if td.ID == "" {
		return []string{}, errors.New("no task ID sent to CreateHost")
	}
	if td.Secret == "" {
		return []string{}, errors.New("no task secret sent to CreateHost")
	}
	c.CreatedHost = options
	return []string{"id"}, options.Validate()
}

func (c *Mock) ListHosts(_ context.Context, _ TaskData) (model.HostListResults, error) {
	return model.HostListResults{}, nil
}

func (c *Mock) GetDockerLogs(context.Context, string, time.Time, time.Time, bool) ([]byte, error) {
	return []byte("this is a log"), nil
}

func (c *Mock) GetDockerStatus(context.Context, string) (*cloud.ContainerStatus, error) {
	return &cloud.ContainerStatus{HasStarted: true}, nil
}

func (c *Mock) ConcludeMerge(ctx context.Context, patchId, status string, td TaskData) error {
	return nil
}

func (c *Mock) GetAdditionalPatches(ctx context.Context, patchId string, td TaskData) ([]string, error) {
	return []string{"555555555555555555555555"}, nil
}
