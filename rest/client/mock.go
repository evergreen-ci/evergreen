package client

import (
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// Mock mocks EvergreenREST for testing.
type Mock struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	serverURL    string

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
	NextTaskIsNil          bool
	EndTaskResponse        *apimodels.EndTaskResponse
	EndTaskShouldFail      bool
	EndTaskResult          endTaskResult
	ShellExecFilename      string
	TimeoutFilename        string
	HeartbeatShouldAbort   bool
	HeartbeatShouldErr     bool

	// metrics collection
	ProcInfo map[string][]*message.ProcessInfo
	SysInfo  map[string]*message.SystemInfo

	// data collected by mocked methods
	logMessages map[string][]apimodels.LogMessage
	PatchFiles  map[string]string
	keyVal      map[string]*serviceModel.KeyVal

	LastMessageSent time.Time

	mu sync.RWMutex
}

type endTaskResult struct {
	Detail   *apimodels.TaskEndDetail
	TaskData TaskData
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
		ProcInfo:     make(map[string][]*message.ProcessInfo),
		SysInfo:      make(map[string]*message.SystemInfo),
		serverURL:    serverURL,
	}
}

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

// nolint
func (c *Mock) SetTimeoutStart(timeoutStart time.Duration) { c.timeoutStart = timeoutStart }
func (c *Mock) SetTimeoutMax(timeoutMax time.Duration)     { c.timeoutMax = timeoutMax }
func (c *Mock) SetMaxAttempts(attempts int)                { c.maxAttempts = attempts }
func (c *Mock) SetHostID(hostID string)                    { c.hostID = hostID }
func (c *Mock) SetHostSecret(hostSecret string)            { c.hostSecret = hostSecret }
func (c *Mock) GetHostID() string                          { return c.hostID }
func (c *Mock) GetHostSecret() string                      { return c.hostSecret }
func (c *Mock) SetAPIUser(apiUser string)                  { c.apiUser = apiUser }
func (c *Mock) SetAPIKey(apiKey string)                    { c.apiKey = apiKey }

// nolint
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

// GetDistro returns a mock Distro.
func (c *Mock) GetDistro(ctx context.Context, td TaskData) (*distro.Distro, error) {
	return &distro.Distro{
		Id:      "mock_distro_id",
		WorkDir: ".",
	}, nil
}

// GetVersion return a mock Version.
func (c *Mock) GetVersion(ctx context.Context, td TaskData) (*version.Version, error) {
	var err error
	var data []byte

	switch td.ID {
	case "shellexec":
		data, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "shellexec.yaml"))
	case "s3copy":
		data, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "s3copy.yaml"))
	case "timeout":
		data, err = ioutil.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "testdata", "timeout.yaml"))
	}
	if err != nil {
		panic(err)
	}
	config := string(data)
	return &version.Version{
		Id:     "mock_version_id",
		Config: config,
	}, nil
}

// Heartbeat returns false, which indicates the heartbeat has succeeded.
func (c *Mock) Heartbeat(ctx context.Context, td TaskData) (bool, error) {
	if c.HeartbeatShouldAbort {
		return true, nil
	}
	if c.HeartbeatShouldErr {
		return false, errors.New("mock heartbeat error")
	}
	return false, nil
}

// FetchExpansionVars returns a mock ExpansionVars.
func (c *Mock) FetchExpansionVars(ctx context.Context, td TaskData) (*apimodels.ExpansionVars, error) {
	return &apimodels.ExpansionVars{
		"shellexec_fn": c.ShellExecFilename,
		"timeout_fn":   c.TimeoutFilename,
	}, nil
}

// GetNextTask returns a mock NextTaskResponse.
func (c *Mock) GetNextTask(ctx context.Context) (*apimodels.NextTaskResponse, error) {
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
		Message:    "mock message",
	}, nil
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
func (c *Mock) GetLoggerProducer(ctx context.Context, td TaskData) LoggerProducer {
	return NewSingleChannelLogHarness(td.ID, newLogSender(ctx, c, apimodels.AgentLogPrefix, td))
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

// GetHostsByUser will return an array with a single mock host
func (c *Mock) GetHostsByUser(ctx context.Context, user string) ([]*model.APIHost, error) {
	hosts := make([]*model.APIHost, 1)
	host, _ := c.CreateSpawnHost(ctx, "mock_distro", "mock_key")
	hosts = append(hosts, host)
	return hosts, nil
}

// CreateSpawnHost will return a mock host that would have been intended
func (*Mock) CreateSpawnHost(ctx context.Context, distroID string, keyName string) (*model.APIHost, error) {
	mockHost := &model.APIHost{
		Id:      model.APIString("mock_host_id"),
		HostURL: model.APIString("mock_url"),
		Distro: model.DistroInfo{
			Id:       model.APIString(distroID),
			Provider: mock.ProviderName,
		},
		Type:        model.APIString("mock_type"),
		Status:      model.APIString(evergreen.HostUninitialized),
		StartedBy:   model.APIString("mock_user"),
		UserHost:    true,
		Provisioned: false,
	}
	return mockHost, nil
}

// GetHosts will return an array with a single mock host
func (c *Mock) GetHosts(ctx context.Context, f func([]*model.APIHost) error) error {
	hosts := make([]*model.APIHost, 1)
	host, _ := c.CreateSpawnHost(ctx, "mock_distro", "mock_key")
	hosts = append(hosts, host)
	err := f(hosts)
	return err
}

// nolint
func (*Mock) GetAllHosts()                        {}
func (*Mock) GetHostByID()                        {}
func (*Mock) SetHostStatus()                      {}
func (*Mock) SetHostStatuses()                    {}
func (*Mock) GetTaskByID()                        {}
func (*Mock) GetTasksByBuild()                    {}
func (*Mock) GetTasksByProjectAndCommit()         {}
func (*Mock) SetTaskStatus()                      {}
func (*Mock) AbortTask()                          {}
func (*Mock) RestartTask()                        {}
func (*Mock) GetKeys()                            {}
func (*Mock) AddKey()                             {}
func (*Mock) RemoveKey()                          {}
func (*Mock) GetProjectByID()                     {}
func (*Mock) EditProject()                        {}
func (*Mock) CreateProject()                      {}
func (*Mock) GetAllProjects()                     {}
func (*Mock) GetBuildByID()                       {}
func (*Mock) GetBuildByProjectAndHashAndVariant() {}
func (*Mock) GetBuildsByVersion()                 {}
func (*Mock) SetBuildStatus()                     {}
func (*Mock) AbortBuild()                         {}
func (*Mock) RestartBuild()                       {}
func (*Mock) GetTestsByTaskID()                   {}
func (*Mock) GetTestsByBuild()                    {}
func (*Mock) GetTestsByTestName()                 {}
func (*Mock) GetVersionByID()                     {}
func (*Mock) GetVersions()                        {}
func (*Mock) GetVersionByProjectAndCommit()       {}
func (*Mock) GetVersionsByProject()               {}
func (*Mock) SetVersionStatus()                   {}
func (*Mock) AbortVersion()                       {}
func (*Mock) RestartVersion()                     {}
func (*Mock) GetAllDistros()                      {}
func (*Mock) GetDistroByID()                      {}
func (*Mock) CreateDistro()                       {}
func (*Mock) EditDistro()                         {}
func (*Mock) DeleteDistro()                       {}
func (*Mock) GetDistroSetupScriptByID()           {}
func (*Mock) GetDistroTeardownScriptByID()        {}
func (*Mock) EditDistroSetupScript()              {}
func (*Mock) EditDistroTeardownScript()           {}
func (*Mock) GetPatchByID()                       {}
func (*Mock) GetPatchesByProject()                {}
func (*Mock) SetPatchStatus()                     {}
func (*Mock) AbortPatch()                         {}
func (*Mock) RestartPatch()                       {}

// nolint
func (c *Mock) SetBannerMessage(ctx context.Context, m string) error                  { return nil }
func (c *Mock) GetBannerMessage(ctx context.Context) (string, error)                  { return "", nil }
func (c *Mock) SetServiceFlags(ctx context.Context, f *model.APIServiceFlags) error   { return nil }
func (c *Mock) GetServiceFlags(ctx context.Context) (*model.APIServiceFlags, error)   { return nil, nil }
func (c *Mock) RestartRecentTasks(ctx context.Context, starAt, endAt time.Time) error { return nil }

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *Mock) SendTestResults(ctx context.Context, td TaskData, results *task.TestResults) error {
	return nil
}

// SendFiles attaches task files.
func (c *Mock) AttachFiles(ctx context.Context, td TaskData, taskFiles []*artifact.File) error {
	return nil
}

// SendTestLog posts a test log for a communicator's task. Is a
// noop if the test Log is nil.
func (c *Mock) SendTestLog(ctx context.Context, td TaskData, log *serviceModel.TestLog) (string, error) {
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

func (c *Mock) SendProcessInfo(ctx context.Context, td TaskData, procs []*message.ProcessInfo) error {
	c.mu.Lock()
	c.ProcInfo[td.ID] = procs
	c.mu.Unlock()
	return nil
}

func (c *Mock) GetProcessInfoLength(id string) int {
	c.mu.RLock()
	length := len(c.ProcInfo[id])
	c.mu.RUnlock()
	return length
}

func (c *Mock) SendSystemInfo(ctx context.Context, td TaskData, sysinfo *message.SystemInfo) error {
	c.mu.Lock()
	c.SysInfo[td.ID] = sysinfo
	c.mu.Unlock()
	return nil
}

func (c *Mock) GetSystemInfoLength() int {
	c.mu.RLock()
	length := len(c.SysInfo)
	c.mu.RUnlock()
	return length
}
