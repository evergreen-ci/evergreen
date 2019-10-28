package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
)

// Communicator is an interface for communicating with the API server.
type Communicator interface {
	// ---------------------------------------------------------------------
	// Begin legacy API methods
	// ---------------------------------------------------------------------
	//
	// Setters
	//
	// SetTimeoutStart sets the initial timeout for a request.
	SetTimeoutStart(time.Duration)
	// SetTimeoutMax sets the maximum timeout for a request.
	SetTimeoutMax(time.Duration)
	// SetMaxAttempts sets the number of attempts a request will be made.
	SetMaxAttempts(int)
	// SetHostID sets the host ID.
	SetHostID(string)
	// SetHostSecret sets the host secret.
	SetHostSecret(string)
	// GetHostID returns the host ID.
	GetHostID() string
	// GetHostSecret returns the host secret.
	GetHostSecret() string

	// Method to release resources used by the communicator.
	Close()

	// Updates the clients local concept of it's last updated
	// time; used by agents to determine timeouts.
	UpdateLastMessageTime()
	LastMessageAt() time.Time
	// Agent Operations
	//
	// StartTask marks the task as started.
	StartTask(context.Context, TaskData) error
	// EndTask marks the task as finished with the given status
	EndTask(context.Context, *apimodels.TaskEndDetail, TaskData) (*apimodels.EndTaskResponse, error)
	// GetTask returns the active task.
	GetTask(context.Context, TaskData) (*task.Task, error)
	// GetProjectRef loads the task's project.
	GetProjectRef(context.Context, TaskData) (*model.ProjectRef, error)
	// GetDistro returns the distro for the task.
	GetDistro(context.Context, TaskData) (*distro.Distro, error)
	// GetVersion loads the task's Version
	GetVersion(context.Context, TaskData) (*model.Version, error)
	// GetExpansions returns all expansions for the task known by the app server
	GetExpansions(context.Context, TaskData) (util.Expansions, error)
	// Heartbeat sends a heartbeat to the API server. The server can respond with
	// an "abort" response. This function returns true if the agent should abort.
	Heartbeat(context.Context, TaskData) (bool, error)
	// FetchExpansionVars loads expansions for a communicator's task from the API server.
	FetchExpansionVars(context.Context, TaskData) (*apimodels.ExpansionVars, error)
	// GetNextTask returns a next task response by getting the next task for a given host.
	GetNextTask(context.Context, *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error)
	// GetBuildloggerInfo returns buildlogger service information including
	// the base URL, RPC port, and LDAP credentials.
	GetBuildloggerInfo(context.Context) (*apimodels.BuildloggerInfo, error)

	// Constructs a new LogProducer instance for use by tasks.
	GetLoggerProducer(context.Context, TaskData, *LoggerConfig) (LoggerProducer, error)
	GetLoggerMetadata() LoggerMetadata

	// Sends a group of log messages to the API Server
	SendLogMessages(context.Context, TaskData, []apimodels.LogMessage) error

	// The following operations use the legacy API server and are
	// used by task commands.
	SendTestResults(context.Context, TaskData, *task.LocalTestResults) error
	SendTestLog(context.Context, TaskData, *model.TestLog) (string, error)
	GetTaskPatch(context.Context, TaskData) (*patchmodel.Patch, error)
	GetPatchFile(context.Context, TaskData, string) (string, error)

	// The following operations are used by
	AttachFiles(context.Context, TaskData, []*artifact.File) error
	GetManifest(context.Context, TaskData) (*manifest.Manifest, error)
	S3Copy(context.Context, TaskData, *apimodels.S3CopyRequest) (string, error)
	KeyValInc(context.Context, TaskData, *model.KeyVal) error

	// these are for the taskdata/json plugin that saves perf data
	PostJSONData(context.Context, TaskData, string, interface{}) error
	GetJSONData(context.Context, TaskData, string, string, string) ([]byte, error)
	GetJSONHistory(context.Context, TaskData, bool, string, string) ([]byte, error)

	// GenerateTasks posts new tasks for the `generate.tasks` command.
	GenerateTasks(context.Context, TaskData, []json.RawMessage) error

	// GenerateTasksPoll polls for new tasks for the `generate.tasks` command.
	GenerateTasksPoll(context.Context, TaskData) (*apimodels.GeneratePollResponse, error)

	// Spawn-hosts for tasks methods
	CreateHost(context.Context, TaskData, apimodels.CreateHost) ([]string, error)
	ListHosts(context.Context, TaskData) ([]restmodel.CreateHost, error)

	// ---------------------------------------------------------------------
	// End legacy API methods
	// ---------------------------------------------------------------------

	// ---------------------------------------------------------------------
	// Begin REST API V2 methods
	// ---------------------------------------------------------------------

	// Client Configuration methods
	//
	SetAPIUser(string)
	SetAPIKey(string)

	// Admin methods
	//
	SetBannerMessage(context.Context, string, evergreen.BannerTheme) error
	GetBannerMessage(context.Context) (string, error)
	SetServiceFlags(context.Context, *restmodel.APIServiceFlags) error
	GetServiceFlags(context.Context) (*restmodel.APIServiceFlags, error)
	RestartRecentTasks(context.Context, time.Time, time.Time) error
	GetSettings(context.Context) (*evergreen.Settings, error)
	UpdateSettings(context.Context, *restmodel.APIAdminSettings) (*restmodel.APIAdminSettings, error)
	GetEvents(context.Context, time.Time, int) ([]interface{}, error)
	RevertSettings(context.Context, string) error

	// Host methods
	GetHostsByUser(context.Context, string) ([]*restmodel.APIHost, error)

	// Spawnhost methods
	//
	CreateSpawnHost(context.Context, *restmodel.HostRequestOptions) (*restmodel.APIHost, error)
	ModifySpawnHost(context.Context, string, host.HostModifyOptions) error
	StopSpawnHost(context.Context, string, bool) error
	StartSpawnHost(context.Context, string, bool) error
	TerminateSpawnHost(context.Context, string) error
	ChangeSpawnHostPassword(context.Context, string, string) error
	ExtendSpawnHostExpiration(context.Context, string, int) error
	GetHosts(context.Context, func([]*restmodel.APIHost) error) error
	AttachVolume(context.Context, string, *host.VolumeAttachment) error
	DetachVolume(context.Context, string, string) error
	CreateVolume(context.Context, *host.Volume) (*restmodel.APIVolume, error)
	DeleteVolume(context.Context, string) error

	// Fetch list of distributions evergreen can spawn
	GetDistrosList(context.Context) ([]restmodel.APIDistro, error)

	// Fetch the current authenticated user's public keys
	GetCurrentUsersKeys(context.Context) ([]restmodel.APIPubKey, error)

	AddPublicKey(context.Context, string, string) error

	// Delete a key with specified name from the current authenticated user
	DeletePublicKey(context.Context, string) error

	// List variant/task aliases
	ListAliases(context.Context, string) ([]model.ProjectAlias, error)

	// GetClientConfig fetches the ClientConfig for the evergreen server
	GetClientConfig(context.Context) (*evergreen.ClientConfig, error)

	// GetSubscriptions fetches the subscriptions for the user defined
	// in the local evergreen yaml
	GetSubscriptions(context.Context) ([]event.Subscription, error)

	// CreateVersionFromConfig takes an evergreen config and makes runnable tasks from it
	CreateVersionFromConfig(context.Context, string, string, bool, []byte) (*model.Version, error)

	// Commit Queue
	GetCommitQueue(ctx context.Context, projectID string) (*restmodel.APICommitQueue, error)
	GetCommitQueueItemAuthor(ctx context.Context, projectID, item string) (string, error)
	DeleteCommitQueueItem(ctx context.Context, projectID string, item string) error
	EnqueueItem(ctx context.Context, patchID string) (int, error)

	GetUserAuthorInfo(context.Context, TaskData, string) (*restmodel.APIUserAuthorInformation, error)

	// Notifications
	SendNotification(ctx context.Context, notificationType string, data interface{}) error

	// GetDockerLogs returns logs for the given docker container
	GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error)
	GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error)

	// GetManifestByTask returns the manifest corresponding to the given task
	GetManifestByTask(ctx context.Context, taskId string) (*manifest.Manifest, error)
}
