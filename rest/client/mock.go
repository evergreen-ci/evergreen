package client

import (
	"context"
	"io"
	"time"

	"github.com/evergreen-ci/evergreen"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/rest/model"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// Mock mocks EvergreenREST for testing.
type Mock struct {
	maxAttempts  int
	timeoutStart time.Duration
	timeoutMax   time.Duration

	// these fields have setters
	apiUser string
	apiKey  string

	// mock behavior
	GetSubscriptionsFail bool
	MockServiceFlags     *model.APIServiceFlags
	MockServiceFlagErr   error

	GetRecentVersionsResult   []restmodel.APIVersion
	GetBuildsForVersionResult []restmodel.APIBuild
	GetTasksForBuildResult    []restmodel.APITask

	SendSlackNotificationData *model.APISlack
	SendEmailNotificationData *model.APIEmail

	ValidateResult validator.ValidationErrors
	ValidateErr    error
}

func (c *Mock) Close() {}

func (c *Mock) SetTimeoutStart(timeoutStart time.Duration) { c.timeoutStart = timeoutStart }
func (c *Mock) SetTimeoutMax(timeoutMax time.Duration)     { c.timeoutMax = timeoutMax }
func (c *Mock) SetMaxAttempts(attempts int)                { c.maxAttempts = attempts }

func (c *Mock) SetAPIUser(apiUser string) { c.apiUser = apiUser }
func (c *Mock) SetAPIKey(apiKey string)   { c.apiKey = apiKey }

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

func (*Mock) GetSpawnHost(ctx context.Context, hostID string) (*model.APIHost, error) {
	return nil, errors.New("(*Mock) GetSpawnHost is not implemented")
}

func (*Mock) GetProject(ctx context.Context, projectID string) (*model.APIProjectRef, error) {
	return nil, errors.New("(*Mock) GetProject is not implemented")
}

func (*Mock) ModifySpawnHost(ctx context.Context, hostID string, changes host.HostModifyOptions) error {
	return errors.New("(*Mock) ModifySpawnHost is not implemented")
}

func (*Mock) TerminateSpawnHost(ctx context.Context, hostID string) error {
	return errors.New("(*Mock) TerminateSpawnHost is not implemented")
}

func (*Mock) StopSpawnHost(ctx context.Context, hostID string, subscriptionType string, shouldKeepOff, wait bool) error {
	return errors.New("(*Mock) StopSpawnHost is not implemented")
}

func (*Mock) StartSpawnHost(context.Context, string, string, bool) error {
	return errors.New("(*Mock) StartSpawnHost is not implemented")
}

func (*Mock) ChangeSpawnHostPassword(context.Context, string, string) error {
	return errors.New("(*Mock) ChangeSpawnHostPassword is not implemented")
}

func (*Mock) FindHostByIpAddress(ctx context.Context, ip string) (*model.APIHost, error) {
	return nil, errors.New("(*Mock) FindHostByIpAddress is not implemented")
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

//nolint:all
func (c *Mock) SetBannerMessage(ctx context.Context, m string, t evergreen.BannerTheme) error {
	return nil
}
func (c *Mock) GetBannerMessage(ctx context.Context) (string, error)                { return "", nil }
func (c *Mock) SetServiceFlags(ctx context.Context, f *model.APIServiceFlags) error { return nil }

func (c *Mock) GetServiceFlags(ctx context.Context) (*model.APIServiceFlags, error) {
	if c.MockServiceFlagErr != nil {
		return c.MockServiceFlags, c.MockServiceFlagErr
	}
	return c.MockServiceFlags, nil
}

func (c *Mock) RestartRecentTasks(ctx context.Context, starAt, endAt time.Time) error { return nil }
func (c *Mock) GetSettings(ctx context.Context) (*evergreen.Settings, error)          { return nil, nil }
func (c *Mock) UpdateSettings(ctx context.Context, update *model.APIAdminSettings) (*model.APIAdminSettings, error) {
	return nil, nil
}
func (c *Mock) GetEvents(ctx context.Context, ts time.Time, limit int) ([]any, error) {
	return nil, nil
}
func (c *Mock) RevertSettings(ctx context.Context, guid string) error { return nil }

func (c *Mock) GetDistrosList(ctx context.Context) ([]model.APIDistro, error) {
	mockDistros := []model.APIDistro{
		{
			Name:             utility.ToStringPtr("archlinux-build"),
			UserSpawnAllowed: true,
		},
		{
			Name:             utility.ToStringPtr("baas-linux"),
			UserSpawnAllowed: false,
		},
	}
	return mockDistros, nil
}

func (c *Mock) GetCurrentUsersKeys(ctx context.Context) ([]model.APIPubKey, error) {
	return []model.APIPubKey{
		{
			Name: utility.ToStringPtr("key0"),
			Key:  utility.ToStringPtr("ssh-fake 12345"),
		},
		{
			Name: utility.ToStringPtr("key1"),
			Key:  utility.ToStringPtr("ssh-fake 67890"),
		},
	}, nil
}

func (c *Mock) AddPublicKey(ctx context.Context, keyName, keyValue string) error {
	return errors.New("(c *Mock) AddPublicKey not implemented")
}

func (c *Mock) DeletePublicKey(ctx context.Context, keyName string) error {
	return errors.New("(c *Mock) DeletePublicKey not implemented")
}

func (c *Mock) ListAliases(ctx context.Context, project string, includeProjectConfig bool) ([]serviceModel.ProjectAlias, error) {
	return nil, errors.New("(c *Mock) ListAliases not implemented")
}

func (c *Mock) ListPatchTriggerAliases(ctx context.Context, project string) ([]string, error) {
	return nil, errors.New("(c *Mock) ListPatchTriggerAliases not implemented")
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

func (c *Mock) SendSlackNotification(_ context.Context, data *model.APISlack) error {
	c.SendSlackNotificationData = data
	return nil
}

func (c *Mock) SendEmailNotification(_ context.Context, data *model.APIEmail) error {
	c.SendEmailNotificationData = data
	return nil
}

func (c *Mock) GetManifestByTask(context.Context, string) (*manifest.Manifest, error) {
	return &manifest.Manifest{Id: "manifest0"}, nil
}

func (c *Mock) GetManifestForVersion(context.Context, string) (*model.APIManifest, error) {
	return nil, nil
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

func (c *Mock) GetRecentVersionsForProject(ctx context.Context, project, branch string, startAtOrderNum, limit int) ([]restmodel.APIVersion, error) {
	if c.GetRecentVersionsResult != nil {
		return c.GetRecentVersionsResult, nil
	}
	return nil, nil
}

func (c *Mock) GetBuildsForVersion(ctx context.Context, versionID string) ([]restmodel.APIBuild, error) {
	if c.GetBuildsForVersionResult != nil {
		return c.GetBuildsForVersionResult, nil
	}
	return nil, nil
}

func (c *Mock) GetTasksForBuild(ctx context.Context, buildID string, startAt string, limit int) ([]restmodel.APITask, error) {
	if c.GetTasksForBuildResult != nil {
		return c.GetTasksForBuildResult, nil
	}
	return nil, nil
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

func (c *Mock) GetClientURLs(context.Context, string) ([]string, error) {
	return []string{"https://example.com"}, nil
}

func (c *Mock) PostHostIsUp(ctx context.Context, options host.HostMetadataOptions) (*restmodel.APIHost, error) {
	return &restmodel.APIHost{
		Id: utility.ToStringPtr("mock_host_id"),
	}, nil
}

func (c *Mock) GetHostProvisioningOptions(ctx context.Context) (*restmodel.APIHostProvisioningOptions, error) {
	return &restmodel.APIHostProvisioningOptions{
		Content: "echo hello world",
	}, nil
}

func (c *Mock) GetRawPatchWithModules(context.Context, string) (*restmodel.APIRawPatch, error) {
	return nil, nil
}

func (c *Mock) GetEstimatedGeneratedTasks(ctx context.Context, patchId string, tvPairs []serviceModel.TVPair) (int, error) {
	return 0, nil
}

func (c *Mock) GetTaskLogs(ctx context.Context, opts GetTaskLogsOptions) (io.ReadCloser, error) {
	return nil, nil
}

func (c *Mock) GetTestLogs(ctx context.Context, opts GetTestLogsOptions) (io.ReadCloser, error) {
	return nil, nil
}

func (c *Mock) GetUiV2URL(ctx context.Context) (string, error) {
	return "https://example.com", nil
}

func (c *Mock) Validate(ctx context.Context, data []byte, quiet bool, projectID string) (validator.ValidationErrors, error) {
	return c.ValidateResult, c.ValidateErr
}

func (c *Mock) SendPanicReport(ctx context.Context, details *model.PanicReport) error {
	return nil
}

func (c *Mock) RevokeGitHubDynamicAccessTokens(ctx context.Context, taskId string, tokens []string) error {
	return nil
}

func (c *Mock) SetHostID(hostID string) {}

func (c *Mock) SetHostSecret(hostSecret string) {}

func (c *Mock) SetOAuth(oauth string) {}

func (c *Mock) SetAPIServerHost(serverURL string) {}

func (c *Mock) IsServiceUser(context.Context, string) (bool, error) { return false, nil }
