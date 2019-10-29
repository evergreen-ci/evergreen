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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
	GetSubscriptionsFail        bool
	CreatedHost                 apimodels.CreateHost

	AttachedFiles    map[string][]*artifact.File
	LogID            string
	LocalTestResults *task.LocalTestResults
	TestLogs         []*serviceModel.TestLog
	TestLogCount     int

	// data collected by mocked methods
	logMessages     map[string][]apimodels.LogMessage
	PatchFiles      map[string]string
	keyVal          map[string]*serviceModel.KeyVal
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
		Execution:    c.TaskExecution,
		Version:      "mock_version_id",
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
func (c *Mock) GetVersion(ctx context.Context, td TaskData) (*serviceModel.Version, error) {
	var err error
	var data []byte

	_, file, _, _ := runtime.Caller(0)

	data, err = ioutil.ReadFile(filepath.Join(filepath.Dir(file), "testdata", fmt.Sprintf("%s.yaml", td.ID)))
	if err != nil {
		grip.Error(err)
	}
	config := string(data)
	return &serviceModel.Version{
		Id:     "mock_version_id",
		Config: config,
	}, nil
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

// GetBuildloggerInfo returns mock buildlogger service information.
func (c *Mock) GetBuildloggerInfo(ctx context.Context) (*apimodels.BuildloggerInfo, error) {
	return &apimodels.BuildloggerInfo{
		BaseURL:  "base_url",
		RPCPort:  "1000",
		Username: "user",
		Password: "password",
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

func (c *Mock) GetTaskPatch(ctx context.Context, td TaskData) (*patchmodel.Patch, error) {
	patch, ok := ctx.Value("patch").(*patchmodel.Patch)
	if !ok {
		return &patchmodel.Patch{}, nil
	}

	return patch, nil
}

// GetHostsByUser will return an array with a single mock host
func (c *Mock) GetHostsByUser(ctx context.Context, user string) ([]*model.APIHost, error) {
	hosts := make([]*model.APIHost, 1)
	spawnRequest := &model.HostRequestOptions{
		DistroID:     "mock_distro",
		KeyName:      "mock_key",
		UserData:     "",
		InstanceTags: nil,
		InstanceType: "mock_type",
	}
	host, _ := c.CreateSpawnHost(ctx, spawnRequest)
	hosts = append(hosts, host)
	return hosts, nil
}

// CreateSpawnHost will return a mock host that would have been intended
func (*Mock) CreateSpawnHost(ctx context.Context, spawnRequest *model.HostRequestOptions) (*model.APIHost, error) {
	mockHost := &model.APIHost{
		Id:      model.ToAPIString("mock_host_id"),
		HostURL: model.ToAPIString("mock_url"),
		Distro: model.DistroInfo{
			Id:       model.ToAPIString(spawnRequest.DistroID),
			Provider: model.ToAPIString(evergreen.ProviderNameMock),
		},
		Type:         model.ToAPIString("mock_type"),
		Status:       model.ToAPIString(evergreen.HostUninitialized),
		StartedBy:    model.ToAPIString("mock_user"),
		UserHost:     true,
		Provisioned:  false,
		InstanceTags: spawnRequest.InstanceTags,
		InstanceType: model.ToAPIString(spawnRequest.InstanceType),
	}
	return mockHost, nil
}

func (*Mock) ModifySpawnHost(ctx context.Context, hostID string, changes host.HostModifyOptions) error {
	return errors.New("(*Mock) ModifySpawnHost is not implemented")
}

func (*Mock) TerminateSpawnHost(ctx context.Context, hostID string) error {
	return errors.New("(*Mock) TerminateSpawnHost is not implemented")
}

func (*Mock) StopSpawnHost(context.Context, string, bool) error {
	return errors.New("(*Mock) StopSpawnHost is not implemented")
}

func (*Mock) StartSpawnHost(context.Context, string, bool) error {
	return errors.New("(*Mock) StartSpawnHost is not implemented")
}

func (*Mock) ChangeSpawnHostPassword(context.Context, string, string) error {
	return errors.New("(*Mock) ChangeSpawnHostPassword is not implemented")
}

func (*Mock) ExtendSpawnHostExpiration(context.Context, string, int) error {
	return errors.New("(*Mock) ExtendSpawnHostExpiration is not implemented")
}

func (*Mock) AttachVolume(context.Context, string, *host.VolumeAttachment) error {
	return errors.New("(*Mock) AttachVolume is not implemented")
}

func (*Mock) DetachVolume(context.Context, string, string) error {
	return errors.New("(*Mock) DetachVolume is not implemented")
}

func (*Mock) CreateVolume(context.Context, *host.Volume) (*model.APIVolume, error) {
	return nil, errors.New("(*Mock) CreateVolume is not implemented")
}

func (*Mock) DeleteVolume(context.Context, string) error {
	return errors.New("(*Mock) DeleteVolume is not implemented")
}

// GetHosts will return an array with a single mock host
func (c *Mock) GetHosts(ctx context.Context, f func([]*model.APIHost) error) error {
	hosts := make([]*model.APIHost, 1)
	spawnRequest := &model.HostRequestOptions{
		DistroID:     "mock_distro",
		KeyName:      "mock_key",
		UserData:     "",
		InstanceTags: nil,
		InstanceType: "mock_type",
	}
	host, _ := c.CreateSpawnHost(ctx, spawnRequest)
	hosts = append(hosts, host)
	err := f(hosts)
	return err
}

// nolint
func (c *Mock) SetBannerMessage(ctx context.Context, m string, t evergreen.BannerTheme) error {
	return nil
}
func (c *Mock) GetBannerMessage(ctx context.Context) (string, error)                  { return "", nil }
func (c *Mock) SetServiceFlags(ctx context.Context, f *model.APIServiceFlags) error   { return nil }
func (c *Mock) GetServiceFlags(ctx context.Context) (*model.APIServiceFlags, error)   { return nil, nil }
func (c *Mock) RestartRecentTasks(ctx context.Context, starAt, endAt time.Time) error { return nil }
func (c *Mock) GetSettings(ctx context.Context) (*evergreen.Settings, error)          { return nil, nil }
func (c *Mock) UpdateSettings(ctx context.Context, update *model.APIAdminSettings) (*model.APIAdminSettings, error) {
	return nil, nil
}
func (c *Mock) GetEvents(ctx context.Context, ts time.Time, limit int) ([]interface{}, error) {
	return nil, nil
}
func (c *Mock) RevertSettings(ctx context.Context, guid string) error { return nil }

// SendResults posts a set of test results for the communicator's task.
// If results are empty or nil, this operation is a noop.
func (c *Mock) SendTestResults(ctx context.Context, td TaskData, results *task.LocalTestResults) error {
	c.LocalTestResults = results
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

func (c *Mock) GetDistrosList(ctx context.Context) ([]model.APIDistro, error) {
	mockDistros := []model.APIDistro{
		{
			Name:             model.ToAPIString("archlinux-build"),
			UserSpawnAllowed: true,
		},
		{
			Name:             model.ToAPIString("baas-linux"),
			UserSpawnAllowed: false,
		},
	}
	return mockDistros, nil
}

func (c *Mock) GetCurrentUsersKeys(ctx context.Context) ([]model.APIPubKey, error) {
	return []model.APIPubKey{
		{
			Name: model.ToAPIString("key0"),
			Key:  model.ToAPIString("ssh-fake 12345"),
		},
		{
			Name: model.ToAPIString("key1"),
			Key:  model.ToAPIString("ssh-fake 67890"),
		},
	}, nil
}

func (c *Mock) AddPublicKey(ctx context.Context, keyName, keyValue string) error {
	return errors.New("(c *Mock) AddPublicKey not implemented")
}

func (c *Mock) DeletePublicKey(ctx context.Context, keyName string) error {
	return errors.New("(c *Mock) DeletePublicKey not implemented")
}

func (c *Mock) ListAliases(ctx context.Context, keyName string) ([]serviceModel.ProjectAlias, error) {
	return nil, errors.New("(c *Mock) ListAliases not implemented")
}

func (c *Mock) GetClientConfig(ctx context.Context) (*evergreen.ClientConfig, error) {
	return &evergreen.ClientConfig{
		ClientBinaries: []evergreen.ClientBinary{
			evergreen.ClientBinary{
				Arch: "amd64",
				OS:   "darwin",
				URL:  "http://example.com/clients/darwin_amd64/evergreen",
			},
		},
		LatestRevision: evergreen.ClientVersion,
	}, nil
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

func (c *Mock) ListHosts(_ context.Context, _ TaskData) ([]model.CreateHost, error) { return nil, nil }

func (c *Mock) GetSubscriptions(_ context.Context) ([]event.Subscription, error) {
	if c.GetSubscriptionsFail {
		return nil, errors.New("failed to fetch subscriptions")
	}

	return []event.Subscription{
		{
			ResourceType: "type",
			Trigger:      "trigger",
			Owner:        "owner",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "data",
				},
			},
			Subscriber: event.Subscriber{
				Type:   "email",
				Target: "a@domain.invalid",
			},
		},
	}, nil
}

func (c *Mock) CreateVersionFromConfig(ctx context.Context, project, message string, active bool, config []byte) (*serviceModel.Version, error) {
	return &serviceModel.Version{}, nil
}

func (c *Mock) GetCommitQueue(ctx context.Context, projectID string) (*model.APICommitQueue, error) {
	return &model.APICommitQueue{
		ProjectID: model.ToAPIString("mci"),
		Queue: []model.APICommitQueueItem{
			model.APICommitQueueItem{
				Issue: model.ToAPIString("123"),
				Modules: []model.APIModule{
					model.APIModule{
						Module: model.ToAPIString("test_module"),
						Issue:  model.ToAPIString("345"),
					},
				},
			},
			model.APICommitQueueItem{
				Issue: model.ToAPIString("345"),
				Modules: []model.APIModule{
					model.APIModule{
						Module: model.ToAPIString("test_module2"),
						Issue:  model.ToAPIString("567"),
					},
				},
			},
		},
	}, nil
}

func (c *Mock) DeleteCommitQueueItem(ctx context.Context, projectID, item string) error {
	return nil
}

func (c *Mock) EnqueueItem(ctx context.Context, patchID string) (int, error) {
	return 1, nil
}

func (c *Mock) GetCommitQueueItemAuthor(ctx context.Context, projectID, item string) (string, error) {
	return "github.user", nil
}

func (c *Mock) GetUserAuthorInfo(ctx context.Context, td TaskData, userID string) (*model.APIUserAuthorInformation, error) {
	return &model.APIUserAuthorInformation{
		DisplayName: model.ToAPIString("evergreen"),
		Email:       model.ToAPIString("evergreen@mongodb.com"),
	}, nil
}

func (c *Mock) SendNotification(_ context.Context, _ string, _ interface{}) error {
	return nil
}

func (c *Mock) GetDockerLogs(context.Context, string, time.Time, time.Time, bool) ([]byte, error) {
	return []byte("this is a log"), nil
}

func (c *Mock) GetDockerStatus(context.Context, string) (*cloud.ContainerStatus, error) {
	return &cloud.ContainerStatus{HasStarted: true}, nil
}

func (c *Mock) GetManifestByTask(context.Context, string) (*manifest.Manifest, error) {
	return &manifest.Manifest{Id: "manifest0"}, nil
}
