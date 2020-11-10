package client

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

// Mock mocks EvergreenREST for testing.
type Mock struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration
	serverURL    string

	// these fields have setters
	apiUser string
	apiKey  string

	// mock behavior
	GetSubscriptionsFail bool
}

// NewMock returns a Communicator for testing.
func NewMock(serverURL string) *Mock {
	return &Mock{
		maxAttempts:  defaultMaxAttempts,
		timeoutStart: defaultTimeoutStart,
		timeoutMax:   defaultTimeoutMax,
		serverURL:    serverURL,
	}
}

func (c *Mock) Close() {}

func (c *Mock) SetTimeoutStart(timeoutStart time.Duration) { c.timeoutStart = timeoutStart }
func (c *Mock) SetTimeoutMax(timeoutMax time.Duration)     { c.timeoutMax = timeoutMax }
func (c *Mock) SetMaxAttempts(attempts int)                { c.maxAttempts = attempts }

func (c *Mock) SetAPIUser(apiUser string) { c.apiUser = apiUser }
func (c *Mock) SetAPIKey(apiKey string)   { c.apiKey = apiKey }

func (c *Mock) GetClientURLs(context.Context, string) ([]string, error) {
	return []string{"https://example.com"}, nil
}

// CreateSpawnHost will return a mock host that would have been intended
func (*Mock) CreateSpawnHost(ctx context.Context, spawnRequest *model.HostRequestOptions) (*model.APIHost, error) {
	mockHost := &model.APIHost{
		Id:      model.ToStringPtr("mock_host_id"),
		HostURL: model.ToStringPtr("mock_url"),
		Distro: model.DistroInfo{
			Id:       model.ToStringPtr(spawnRequest.DistroID),
			Provider: model.ToStringPtr(evergreen.ProviderNameMock),
		},
		Provider:     model.ToStringPtr(evergreen.ProviderNameMock),
		Status:       model.ToStringPtr(evergreen.HostUninitialized),
		StartedBy:    model.ToStringPtr("mock_user"),
		UserHost:     true,
		Provisioned:  false,
		InstanceTags: spawnRequest.InstanceTags,
		InstanceType: model.ToStringPtr(spawnRequest.InstanceType),
	}
	return mockHost, nil
}

func (*Mock) GetSpawnHost(ctx context.Context, hostID string) (*model.APIHost, error) {
	return nil, errors.New("(*Mock) GetSpawnHost is not implemented")
}

func (*Mock) ModifySpawnHost(ctx context.Context, hostID string, changes host.HostModifyOptions) error {
	return errors.New("(*Mock) ModifySpawnHost is not implemented")
}

func (*Mock) TerminateSpawnHost(ctx context.Context, hostID string) error {
	return errors.New("(*Mock) TerminateSpawnHost is not implemented")
}

func (*Mock) StopSpawnHost(context.Context, string, string, bool) error {
	return errors.New("(*Mock) StopSpawnHost is not implemented")
}

func (*Mock) StartSpawnHost(context.Context, string, string, bool) error {
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

func (*Mock) ModifyVolume(context.Context, string, *model.VolumeModifyOptions) error {
	return errors.New("(*Mock) ModifyVolume is not implemented")
}

func (*Mock) GetVolumesByUser(context.Context) ([]model.APIVolume, error) {
	return nil, errors.New("(*Mock) GetVolumesByUser is not implemented")
}

func (c *Mock) GetVolume(context.Context, string) (*model.APIVolume, error) {
	return nil, errors.New("(*Mock) GetVolume is not implemented")
}

// GetHosts will return an array with a single mock host
func (c *Mock) GetHosts(ctx context.Context, data model.APIHostParams) ([]*model.APIHost, error) {
	spawnRequest := &model.HostRequestOptions{
		DistroID:     "mock_distro",
		KeyName:      "mock_key",
		UserData:     "",
		InstanceTags: nil,
		InstanceType: "mock_type",
	}
	host, _ := c.CreateSpawnHost(ctx, spawnRequest)
	return []*model.APIHost{host}, nil
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
func (c *Mock) ExecuteOnDistro(context.Context, string, model.APIDistroScriptOptions) ([]string, error) {
	return nil, nil
}

func (c *Mock) GetDistrosList(ctx context.Context) ([]model.APIDistro, error) {
	mockDistros := []model.APIDistro{
		{
			Name:             model.ToStringPtr("archlinux-build"),
			UserSpawnAllowed: true,
		},
		{
			Name:             model.ToStringPtr("baas-linux"),
			UserSpawnAllowed: false,
		},
	}
	return mockDistros, nil
}

func (c *Mock) GetCurrentUsersKeys(ctx context.Context) ([]model.APIPubKey, error) {
	return []model.APIPubKey{
		{
			Name: model.ToStringPtr("key0"),
			Key:  model.ToStringPtr("ssh-fake 12345"),
		},
		{
			Name: model.ToStringPtr("key1"),
			Key:  model.ToStringPtr("ssh-fake 67890"),
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

func (c *Mock) GetParameters(context.Context, string) ([]serviceModel.ParameterInfo, error) {
	return nil, errors.New("(c *Mock) GetParameters not implemented")
}

func (c *Mock) GetClientConfig(ctx context.Context) (*evergreen.ClientConfig, error) {
	return &evergreen.ClientConfig{
		ClientBinaries: []evergreen.ClientBinary{
			{
				Arch: "amd64",
				OS:   "darwin",
				URL:  "http://example.com/clients/darwin_amd64/evergreen",
			},
		},
		LatestRevision: evergreen.ClientVersion,
	}, nil
}

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
		ProjectID: model.ToStringPtr("mci"),
		Queue: []model.APICommitQueueItem{
			{
				Issue: model.ToStringPtr("123"),
				Modules: []model.APIModule{
					{
						Module: model.ToStringPtr("test_module"),
						Issue:  model.ToStringPtr("345"),
					},
				},
			},
			{
				Issue: model.ToStringPtr("345"),
				Modules: []model.APIModule{
					{
						Module: model.ToStringPtr("test_module2"),
						Issue:  model.ToStringPtr("567"),
					},
				},
			},
		},
	}, nil
}

func (c *Mock) DeleteCommitQueueItem(ctx context.Context, projectID, item string) error {
	return nil
}

func (c *Mock) EnqueueItem(ctx context.Context, patchID string, force bool) (int, error) {
	return 0, nil
}

func (c *Mock) CreatePatchForMerge(ctx context.Context, patchID string) (*model.APIPatch, error) {
	return nil, nil
}

func (c *Mock) SendNotification(_ context.Context, _ string, _ interface{}) error {
	return nil
}

func (c *Mock) GetManifestByTask(context.Context, string) (*manifest.Manifest, error) {
	return &manifest.Manifest{Id: "manifest0"}, nil
}

func (c *Mock) StartHostProcesses(context.Context, []string, string, int) ([]model.APIHostProcess, error) {
	return nil, nil
}

func (c *Mock) GetHostProcessOutput(context.Context, []model.APIHostProcess, int) ([]model.APIHostProcess, error) {
	return nil, nil
}

func (c *Mock) GetMatchingHosts(context.Context, time.Time, time.Time, string, bool) ([]string, error) {
	return nil, nil
}

func (c *Mock) GetRecentVersionsForProject(context.Context, string, string) ([]restmodel.APIVersion, error) {
	return nil, nil
}

func (c *Mock) GetTaskSyncReadCredentials(context.Context) (*evergreen.S3Credentials, error) {
	return &evergreen.S3Credentials{}, nil
}

func (c *Mock) GetTaskSyncPath(context.Context, string) (string, error) {
	return "", nil
}

func (c *Mock) GetDistroByName(context.Context, string) (*model.APIDistro, error) {
	return nil, nil
}

func (c *Mock) UpdateServiceUser(context.Context, string, string, []string) error {
	return nil
}
func (c *Mock) DeleteServiceUser(context.Context, string) error {
	return nil
}
func (c *Mock) GetServiceUsers(context.Context) ([]model.APIDBUser, error) {
	return nil, nil
}
func (c *Mock) GetMessageForPatch(context.Context, string) (string, error) {
	return "", nil
}
