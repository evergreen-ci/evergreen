package client

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
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
	// Client Configuration methods
	//
	SetAPIUser(string)
	SetAPIKey(string)

	// Method to release resources used by the communicator.
	Close()

	// Admin methods
	//
	SetBannerMessage(context.Context, string, evergreen.BannerTheme) error
	GetBannerMessage(context.Context) (string, error)
	GetUiV2URL(context.Context) (string, error)
	SetServiceFlags(context.Context, *restmodel.APIServiceFlags) error
	GetServiceFlags(context.Context) (*restmodel.APIServiceFlags, error)
	RestartRecentTasks(context.Context, time.Time, time.Time) error
	GetSettings(context.Context) (*evergreen.Settings, error)
	UpdateSettings(context.Context, *restmodel.APIAdminSettings) (*restmodel.APIAdminSettings, error)
	GetEvents(context.Context, time.Time, int) ([]interface{}, error)
	RevertSettings(context.Context, string) error
	ExecuteOnDistro(ctx context.Context, distro string, opts restmodel.APIDistroScriptOptions) (hostIDs []string, err error)
	GetServiceUsers(ctx context.Context) ([]restmodel.APIDBUser, error)
	UpdateServiceUser(context.Context, string, string, []string) error
	DeleteServiceUser(context.Context, string) error

	// Spawnhost methods
	//
	CreateSpawnHost(context.Context, *restmodel.HostRequestOptions) (*restmodel.APIHost, error)
	GetSpawnHost(context.Context, string) (*restmodel.APIHost, error)
	ModifySpawnHost(context.Context, string, host.HostModifyOptions) error
	StopSpawnHost(context.Context, string, string, bool) error
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

	// List variant/task aliases
	ListAliases(context.Context, string) ([]model.ProjectAlias, error)
	ListPatchTriggerAliases(context.Context, string) ([]string, error)
	GetDistroByName(context.Context, string) (*restmodel.APIDistro, error)

	// Get parameters for project
	GetParameters(context.Context, string) ([]model.ParameterInfo, error)

	// GetClientConfig fetches the ClientConfig for the evergreen server
	GetClientConfig(context.Context) (*evergreen.ClientConfig, error)

	// GetSubscriptions fetches the subscriptions for the user defined
	// in the local evergreen yaml
	GetSubscriptions(context.Context) ([]event.Subscription, error)

	// CreateVersionFromConfig takes an evergreen config and makes runnable tasks from it
	CreateVersionFromConfig(context.Context, string, string, bool, []byte) (*model.Version, error)

	// Commit Queue
	GetCommitQueue(ctx context.Context, projectID string) (*restmodel.APICommitQueue, error)
	DeleteCommitQueueItem(ctx context.Context, projectID string, item string) error
	// if enqueueNext is true then allow item to be processed next
	EnqueueItem(ctx context.Context, patchID string, enqueueNext bool) (int, error)
	CreatePatchForMerge(ctx context.Context, patchID, commitMessage string) (*restmodel.APIPatch, error)
	GetMessageForPatch(ctx context.Context, patchID string) (string, error)

	// Notifications
	SendNotification(ctx context.Context, notificationType string, data interface{}) error

	// GetManifestByTask returns the manifest corresponding to the given task
	GetManifestByTask(ctx context.Context, taskId string) (*manifest.Manifest, error)

	GetRecentVersionsForProject(ctx context.Context, projectID, requester string) ([]restmodel.APIVersion, error)

	// GetTaskSyncReadCredentials returns the credentials to fetch task
	// directory from S3.
	GetTaskSyncReadCredentials(ctx context.Context) (*evergreen.S3Credentials, error)
	// GetTaskSyncPath returns the path to the task directory in S3.
	GetTaskSyncPath(ctx context.Context, taskID string) (string, error)

	// GetClientURLs returns the all URLs that can be used to request the
	// Evergreen binary for a given distro.
	GetClientURLs(ctx context.Context, distroID string) ([]string, error)

	// GetHostProvisioningOptions gets the options to provision a host.
	GetHostProvisioningOptions(ctx context.Context, hostID, hostSecret string) (*restmodel.APIHostProvisioningOptions, error)

	// CompareTasks returns the order that the given tasks would be scheduled, along with the scheduling logic.
	CompareTasks(context.Context, []string, bool) ([]string, map[string]map[string]string, error)
}
