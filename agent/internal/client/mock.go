package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testlog"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/taskoutput"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Mock mocks the Communicator for testing.
type Mock struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	serverURL    string

	// mock behavior
	NextTaskShouldFail            bool
	GetPatchFileShouldFail        bool
	TaskShouldRetryOnFail         bool
	loggingShouldFail             bool
	NextTaskResponse              *apimodels.NextTaskResponse
	NextTaskIsNil                 bool
	StartTaskShouldFail           bool
	GetTaskResponse               *task.Task
	GetProjectResponse            *serviceModel.Project
	EndTaskResponse               *apimodels.EndTaskResponse
	EndTaskShouldFail             bool
	EndTaskResult                 EndTaskResult
	ShellExecFilename             string
	TimeoutFilename               string
	GenerateTasksShouldFail       bool
	HeartbeatShouldAbort          bool
	HeartbeatShouldConflict       bool
	HeartbeatShouldErr            bool
	HeartbeatShouldSometimesErr   bool
	HeartbeatCount                int
	TaskExecution                 int
	CreatedHost                   apimodels.CreateHost
	GetTaskPatchResponse          *patchmodel.Patch
	GetLoggerProducerShouldFail   bool
	CreateInstallationTokenFail   bool
	CreateInstallationTokenResult string

	CedarGRPCConn *grpc.ClientConn

	AttachedFiles    map[string][]*artifact.File
	LogID            string
	LocalTestResults []testresult.TestResult
	ResultsService   string
	ResultsFailed    bool
	TestLogs         []*testlog.TestLog
	TestLogCount     int

	taskLogs   map[string][]log.LogLine
	PatchFiles map[string]string
	keyVal     map[string]*serviceModel.KeyVal

	// Mock data returned from methods
	LastMessageSent  time.Time
	DownstreamParams []patchmodel.Parameter

	mu sync.RWMutex
}

type EndTaskResult struct {
	Detail   *apimodels.TaskEndDetail
	TaskData TaskData
}

// NewMock returns a Communicator for testing.
func NewMock(serverURL string) *Mock {
	return &Mock{
		maxAttempts:   defaultMaxAttempts,
		timeoutStart:  defaultTimeoutStart,
		timeoutMax:    defaultTimeoutMax,
		taskLogs:      make(map[string][]log.LogLine),
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

func (c *Mock) StartTask(ctx context.Context, td TaskData) error {
	if c.StartTaskShouldFail {
		return errors.New("start task mock failure")
	}
	return nil
}

// EndTask returns a mock EndTaskResponse.
func (c *Mock) EndTask(ctx context.Context, detail *apimodels.TaskEndDetail, td TaskData) (*apimodels.EndTaskResponse, error) {
	if c.EndTaskShouldFail {
		return nil, errors.New("end task should fail")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.EndTaskResult.Detail = detail
	c.EndTaskResult.TaskData = td

	if c.EndTaskResponse != nil {
		return c.EndTaskResponse, nil
	}

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
	if c.GetTaskResponse != nil {
		return c.GetTaskResponse, nil
	}

	var id, secret string
	if id = td.ID; id == "" {
		id = "mock_task_id"
	}
	if secret = td.Secret; secret == "" {
		secret = "mock_task_secret"
	}

	return &task.Task{
		Id:             id,
		Secret:         secret,
		BuildVariant:   "mock_build_variant",
		DisplayName:    "build",
		Execution:      c.TaskExecution,
		Version:        "mock_version_id",
		TaskOutputInfo: &taskoutput.TaskOutput{},
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
	return &apimodels.DistroView{}, nil
}

func (c *Mock) GetDistroAMI(context.Context, string, string, TaskData) (string, error) {
	return "ami-mock", nil
}

// GetProject returns the mock project. If an explicit GetProjectResponse is
// specified, it will return that. Otherwise, by default, it will load data from
// the agent's testdata directory, which contains project YAML files for
// testing. The task ID is used to identify the name of the YAML file it will
// load.
func (c *Mock) GetProject(ctx context.Context, td TaskData) (*serviceModel.Project, error) {
	if c.GetProjectResponse != nil {
		return c.GetProjectResponse, nil
	}
	var err error
	var data []byte
	_, file, _, _ := runtime.Caller(0)

	data, err = os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", fmt.Sprintf("%s.yaml", td.ID)))
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

// GetExpansionsAndVars returns a mock ExpansionsAndVars.
func (c *Mock) GetExpansionsAndVars(ctx context.Context, taskData TaskData) (*apimodels.ExpansionsAndVars, error) {
	return &apimodels.ExpansionsAndVars{
		Expansions: util.Expansions{"foo": "bar"},
		Vars: map[string]string{
			"shellexec_fn":   c.ShellExecFilename,
			"timeout_fn":     c.TimeoutFilename,
			"my_new_timeout": "2",
		},
		Parameters: map[string]string{
			"overwrite-this-parameter": "new-parameter-value",
		},
		PrivateVars: map[string]bool{
			"some_private_var": true,
		},
	}, nil
}

func (c *Mock) Heartbeat(ctx context.Context, td TaskData) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.HeartbeatCount++
	if c.HeartbeatShouldAbort {
		return evergreen.TaskFailed, nil
	}
	if c.HeartbeatShouldConflict {
		return evergreen.TaskFailed, nil
	}
	if c.HeartbeatShouldSometimesErr {
		if c.HeartbeatShouldErr {
			c.HeartbeatShouldErr = false
			return "", errors.New("mock heartbeat error")
		}
		c.HeartbeatShouldErr = true
		return "", nil
	}
	if c.HeartbeatShouldErr {
		return "", errors.New("mock heartbeat error")
	}
	return "", nil
}

// GetHeartbeatCount returns the current number of recorded heartbeats. This is
// thread-safe.
func (c *Mock) GetHeartbeatCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.HeartbeatCount
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
	if c.NextTaskResponse != nil {
		return c.NextTaskResponse, nil
	}

	return &apimodels.NextTaskResponse{
		TaskId:     "mock_task_id",
		TaskSecret: "mock_task_secret",
		ShouldExit: false,
	}, nil
}

// GetCedarConfig returns a mock Cedar service configuration.
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

// GetDataPipesConfig returns a mock Data-Pipes service configuration.
func (c *Mock) GetDataPipesConfig(ctx context.Context) (*apimodels.DataPipesConfig, error) {
	return &apimodels.DataPipesConfig{
		Host:         "url",
		Region:       "us-east-1",
		AWSAccessKey: "access",
		AWSSecretKey: "secret",
	}, nil
}

// GetLoggerProducer constructs a single channel log producer.
func (c *Mock) GetLoggerProducer(ctx context.Context, tsk *task.Task, _ *LoggerConfig) (LoggerProducer, error) {
	if c.GetLoggerProducerShouldFail {
		return nil, errors.New("operation run in fail mode.")
	}

	// Set task ID here to avoid data race with the passed in task pointer
	// in tests, otherwise the append line function will try to access the
	// task ID in the task pointer every time `sendTaskLogLine` is called.
	taskID := tsk.Id
	appendLine := func(line log.LogLine) error {
		return c.sendTaskLogLine(taskID, line)
	}

	return NewSingleChannelLogHarness(taskID, newMockSender("mock", appendLine)), nil
}

// sendTaskLogLine appends a new log line to the task log cache.
func (c *Mock) sendTaskLogLine(taskID string, line log.LogLine) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.loggingShouldFail {
		return errors.New("logging failed")
	}

	c.taskLogs[taskID] = append(c.taskLogs[taskID], line)

	return nil
}

func (c *Mock) GetTaskLogs(taskID string) []log.LogLine {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.taskLogs[taskID]
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
	if c.GetTaskPatchResponse != nil {
		return c.GetTaskPatchResponse, nil
	}

	return &patch.Patch{}, nil
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

func (c *Mock) SetResultsInfo(ctx context.Context, _ TaskData, service string, failed bool) error {
	c.ResultsService = service
	if failed {
		c.ResultsFailed = true
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

	c.AttachedFiles[td.ID] = append(c.AttachedFiles[td.ID], taskFiles...)

	return nil
}

func (c *Mock) SetDownstreamParams(ctx context.Context, downstreamParams []patchmodel.Parameter, taskData TaskData) error {
	c.DownstreamParams = downstreamParams
	return nil
}

func (c *Mock) NewPush(ctx context.Context, td TaskData, req *apimodels.S3CopyRequest) (*serviceModel.PushLog, error) {
	return nil, nil
}

func (c *Mock) UpdatePushStatus(ctx context.Context, td TaskData, pushLog *serviceModel.PushLog) error {
	return nil
}

// SendTestLog posts a test log for a communicator's task. Is a
// noop if the test Log is nil.
func (c *Mock) SendTestLog(ctx context.Context, td TaskData, log *testlog.TestLog) (string, error) {
	c.TestLogs = append(c.TestLogs, log)
	c.TestLogs[len(c.TestLogs)-1].Id = c.LogID
	c.TestLogCount += 1
	c.LogID = fmt.Sprintf("%s-%d", c.LogID, c.TestLogCount)
	return c.LogID, nil
}

func (c *Mock) GetManifest(ctx context.Context, td TaskData) (*manifest.Manifest, error) {
	return &manifest.Manifest{}, nil
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
	resp := &apimodels.GeneratePollResponse{
		Finished: true,
	}
	if c.GenerateTasksShouldFail {
		resp.Error = "error polling generate tasks!"
	}
	return resp, nil
}

func (c *Mock) CreateHost(ctx context.Context, td TaskData, options apimodels.CreateHost) ([]string, error) {
	if td.ID == "" {
		return []string{}, errors.New("no task ID sent to CreateHost")
	}
	if td.Secret == "" {
		return []string{}, errors.New("no task secret sent to CreateHost")
	}
	c.CreatedHost = options
	return []string{"id"}, options.Validate(ctx)
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

func (c *Mock) GetPullRequestInfo(ctx context.Context, taskData TaskData, prNum int, owner, repo string, lastAttempt bool) (*apimodels.PullRequestInfo, error) {
	return &apimodels.PullRequestInfo{
		Mergeable: utility.TruePtr(),
	}, nil
}

func (c *Mock) CreateInstallationToken(ctx context.Context, td TaskData, owner, repo string) (string, error) {
	if c.CreateInstallationTokenFail {
		return "", errors.New("failed to create token")
	}
	return c.CreateInstallationTokenResult, nil
}

func (c *Mock) MarkFailedTaskToRestart(ctx context.Context, td TaskData) error {
	c.TaskShouldRetryOnFail = true
	return nil
}

type mockSender struct {
	appendLine func(log.LogLine) error
	lastErr    error
	*send.Base
}

func newMockSender(name string, appendLine func(log.LogLine) error) *mockSender {
	return &mockSender{
		appendLine: appendLine,
		Base:       send.NewBase(name),
	}
}

func (s *mockSender) Send(m message.Composer) {
	ts := time.Now().UnixNano()

	if !s.Level().ShouldLog(m) {
		return
	}

	if err := s.appendLine(log.LogLine{
		Priority:  m.Priority(),
		Timestamp: ts,
		Data:      m.String(),
	}); err != nil {
		s.lastErr = err
	}
}

func (s *mockSender) Flush(_ context.Context) error { return nil }

func (c *Mock) UpsertCheckRun(ctx context.Context, td TaskData, checkRunOutput apimodels.CheckRunOutput) error {
	return nil
}
