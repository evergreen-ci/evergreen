package client

import (
	"context"
	"io"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/validator"
)

// Communicator is an interface for communicating with the API server.
type Communicator interface {
	// Setters
	//
	// SetTimeoutStart sets the initial timeout for a request.
	SetTimeoutStart(time.Duration)
	// SetTimeoutMax sets the maximum timeout for a request.
	SetTimeoutMax(time.Duration)
	// SetMaxAttempts sets the number of attempts a request will be made.
	SetMaxAttempts(int)
	// Client authentication methods (for users)
	SetAPIUser(string)
	SetAPIKey(string)
	SetJWT(string)
	SetAPIServerHost(string)
	// Client authentication methods (for hosts)
	SetHostID(string)
	SetHostSecret(string)
	// Method to release resources used by the communicator.
	Close()

	// Admin methods
	//
	SetBannerMessage(context.Context, string, evergreen.BannerTheme) error
	GetBannerMessage(context.Context) (string, error)
	GetUiV2URL(context.Context) (string, error)
	SetServiceFlags(context.Context, *restmodel.APIServiceFlags) error
	GetServiceFlags(context.Context) (*restmodel.APIServiceFlags, error)
	IsServiceUser(context.Context, string) (bool, error)
	RestartRecentTasks(context.Context, time.Time, time.Time) error
	GetSettings(context.Context) (*evergreen.Settings, error)
	UpdateSettings(context.Context, *restmodel.APIAdminSettings) (*restmodel.APIAdminSettings, error)
	GetEvents(context.Context, time.Time, int) ([]any, error)
	RevertSettings(context.Context, string) error
	GetServiceUsers(ctx context.Context) ([]restmodel.APIDBUser, error)
	UpdateServiceUser(context.Context, string, string, []string) error
	DeleteServiceUser(context.Context, string) error

	// Spawnhost methods
	//
	CreateSpawnHost(context.Context, *restmodel.HostRequestOptions) (*restmodel.APIHost, error)
	GetSpawnHost(context.Context, string) (*restmodel.APIHost, error)
	ModifySpawnHost(context.Context, string, host.HostModifyOptions) error
	StopSpawnHost(context.Context, string, string, bool, bool) error
	StartSpawnHost(context.Context, string, string, bool) error
	TerminateSpawnHost(context.Context, string) error
	ChangeSpawnHostPassword(context.Context, string, string) error
	ExtendSpawnHostExpiration(context.Context, string, int) error
	GetHosts(context.Context, restmodel.APIHostParams) ([]*restmodel.APIHost, error)
	AttachVolume(context.Context, string, *host.VolumeAttachment) error
	DetachVolume(context.Context, string, string) error
	CreateVolume(context.Context, *host.Volume) (*restmodel.APIVolume, error)
	DeleteVolume(context.Context, string) error
	ModifyVolume(context.Context, string, *restmodel.VolumeModifyOptions) error
	GetVolume(context.Context, string) (*restmodel.APIVolume, error)
	GetVolumesByUser(context.Context) ([]restmodel.APIVolume, error)
	StartHostProcesses(context.Context, []string, string, int) ([]restmodel.APIHostProcess, error)
	GetHostProcessOutput(context.Context, []restmodel.APIHostProcess, int) ([]restmodel.APIHostProcess, error)
	FindHostByIpAddress(context.Context, string) (*restmodel.APIHost, error)

	// Fetch list of distributions evergreen can spawn
	GetDistrosList(context.Context) ([]restmodel.APIDistro, error)

	// Fetch the current authenticated user's public keys
	GetCurrentUsersKeys(context.Context) ([]restmodel.APIPubKey, error)

	AddPublicKey(context.Context, string, string) error

	// Delete a key with specified name from the current authenticated user
	DeletePublicKey(context.Context, string) error

	// List variant/task aliases, with bool parameter to optionally include YAML-defined aliases.
	ListAliases(context.Context, string, bool) ([]model.ProjectAlias, error)
	ListPatchTriggerAliases(context.Context, string) ([]string, error)
	GetDistroByName(context.Context, string) (*restmodel.APIDistro, error)

	// Get parameters for project
	GetParameters(context.Context, string) ([]model.ParameterInfo, error)

	// GetClientConfig fetches the ClientConfig for the evergreen server
	GetClientConfig(context.Context) (*evergreen.ClientConfig, error)

	// GetSubscriptions fetches the subscriptions for the user defined
	// in the local evergreen yaml
	GetSubscriptions(context.Context) ([]event.Subscription, error)

	// Notifications
	SendNotification(ctx context.Context, notificationType string, data any) error

	// GetManifestByTask returns the manifest corresponding to the given task
	GetManifestByTask(ctx context.Context, taskId string) (*manifest.Manifest, error)
	// GetManifestForVersion returns the manifest for a given version ID.
	GetManifestForVersion(ctx context.Context, versionID string) (*restmodel.APIManifest, error)

	// GetRecentVersionsForProject returns the most recent versions for a
	// project.
	GetRecentVersionsForProject(ctx context.Context, projectID, requester string, startAtOrderNum, limit int) ([]restmodel.APIVersion, error)

	// GetBuildsForVersion gets all builds for a version.
	GetBuildsForVersion(ctx context.Context, versionID string) ([]restmodel.APIBuild, error)
	// GetTasksForBuild gets all tasks in a build.
	GetTasksForBuild(ctx context.Context, buildID string, startAt string, limit int) ([]restmodel.APITask, error)

	// GetClientURLs returns the all URLs that can be used to request the
	// Evergreen binary for a given distro.
	GetClientURLs(ctx context.Context, distroID string) ([]string, error)

	// PostHostIsUp indicates to the app server that the task host is up and
	// running.
	PostHostIsUp(ctx context.Context, opts host.HostMetadataOptions) (*restmodel.APIHost, error)
	// GetHostProvisioningOptions gets the options to provision a host.
	GetHostProvisioningOptions(ctx context.Context) (*restmodel.APIHostProvisioningOptions, error)

	// GetRawPatchWithModules fetches the raw patch and module diffs for a given patch ID.
	GetRawPatchWithModules(ctx context.Context, patchId string) (*restmodel.APIRawPatch, error)

	// GetTaskLogs returns task logs for the given task.
	GetTaskLogs(context.Context, GetTaskLogsOptions) (io.ReadCloser, error)
	// GetTaskLogs returns test logs for the given task.
	GetTestLogs(context.Context, GetTestLogsOptions) (io.ReadCloser, error)

	// GetEstimatedGeneratedTasks returns the estimated number of generated tasks to be created by an unfinalized patch.
	GetEstimatedGeneratedTasks(context.Context, string, []model.TVPair) (int, error)

	// RevokeGitHubDynamicAccessToken revokes the given GitHub dynamic access tokens.
	RevokeGitHubDynamicAccessTokens(ctx context.Context, taskID string, tokens []string) error

	// Validate validates a project configuration file.
	Validate(ctx context.Context, data []byte, quiet bool, projectID string) (validator.ValidationErrors, error)
}

// GetTaskLogsOptions are the options for fetching task logs for a given task.
type GetTaskLogsOptions struct {
	TaskID        string
	Execution     *int
	Type          string
	Start         string
	End           string
	LineLimit     int
	TailLimit     int
	PrintTime     bool
	PrintPriority bool
	Paginate      bool
}

// GetTestLogsOptions are the options for fetching test logs for a given task.
type GetTestLogsOptions struct {
	TaskID        string
	Path          string
	Execution     *int
	LogsToMerge   []string
	Start         string
	End           string
	LineLimit     int
	TailLimit     int
	PrintTime     bool
	PrintPriority bool
	Paginate      bool
}
