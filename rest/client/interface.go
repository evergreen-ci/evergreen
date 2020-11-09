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
	// ---------------------------------------------------------------------
	// Begin legacy API methods
	// ---------------------------------------------------------------------
	//
	// Setters
	//
	// kim: NOTE: unused
	// SetTimeoutStart sets the initial timeout for a request.
	SetTimeoutStart(time.Duration)
	// kim: NOTE: unused
	// SetTimeoutMax sets the maximum timeout for a request.
	SetTimeoutMax(time.Duration)
	// kim: NOTE: unused
	// SetMaxAttempts sets the number of attempts a request will be made.
	SetMaxAttempts(int)
	// // kim: NOTE: used only by agent.
	// // SetHostID sets the host ID.
	// SetHostID(string)
	// // kim: NOTE: used only by agent.
	// // SetHostSecret sets the host secret.
	// SetHostSecret(string)
	// // kim: NOTE: used only by agent.
	// // GetHostID returns the host ID.
	// GetHostID() string
	// // kim: NOTE: used only by agent.
	// // GetHostSecret returns the host secret.
	// GetHostSecret() string
	//
	// // kim: NOTE: used by both agent and CLI.
	// Method to release resources used by the communicator.
	Close()
	//
	// // kim: NOTE: used only by agent.
	// // Updates the clients local concept of it's last updated
	// // time; used by agents to determine timeouts.
	// UpdateLastMessageTime()
	// // kim: NOTE: used only by agent.
	// LastMessageAt() time.Time
	// // Agent Operations
	// //
	// // StartTask marks the task as started.
	// StartTask(context.Context, TaskData) error
	// // EndTask marks the task as finished with the given status
	// EndTask(context.Context, *apimodels.TaskEndDetail, TaskData) (*apimodels.EndTaskResponse, error)
	// // GetTask returns the active task.
	// GetTask(context.Context, TaskData) (*task.Task, error)
	// // GetDisplayTaskNameFromExecution returns the display task name of an
	// // execution task, if it exists. It will return an empty string and no error
	// // if the task is not part of a display task.
	// GetDisplayTaskNameFromExecution(context.Context, TaskData) (string, error)
	// // GetProjectRef loads the task's project ref.
	// GetProjectRef(context.Context, TaskData) (*model.ProjectRef, error)
	// // GetDistro returns the distro for the task.
	// GetDistro(context.Context, TaskData) (*distro.Distro, error)
	// // GetDistroAMI gets the AMI for the given distro/region
	// GetDistroAMI(context.Context, string, string, TaskData) (string, error)
	// // GetProject loads the project using the task's version ID
	// GetProject(context.Context, TaskData) (*model.Project, error)
	// // GetExpansions returns all expansions for the task known by the app server
	// GetExpansions(context.Context, TaskData) (util.Expansions, error)
	// // Heartbeat sends a heartbeat to the API server. The server can respond with
	// // an "abort" response. This function returns true if the agent should abort.
	// Heartbeat(context.Context, TaskData) (bool, error)
	// // FetchExpansionVars loads expansions for a communicator's task from the API server.
	// FetchExpansionVars(context.Context, TaskData) (*apimodels.ExpansionVars, error)
	// // GetNextTask returns a next task response by getting the next task for a given host.
	// GetNextTask(context.Context, *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error)
	// // GetCedarConfig returns the cedar service information including the
	// // base URL, RPC port, and credentials.
	// GetCedarConfig(context.Context) (*apimodels.CedarConfig, error)
	// // GetCedarGRPCConn returns the client connection to cedar if it exists, or
	// // creates it if it doesn't exist.
	// GetCedarGRPCConn(context.Context) (*grpc.ClientConn, error)
	//
	// // GetAgentSetupData populates an agent with the necessary data, including
	// // secrets.
	// GetAgentSetupData(context.Context) (*apimodels.AgentSetupData, error)
	//
	// // Constructs a new LogProducer instance for use by tasks.
	// GetLoggerProducer(context.Context, TaskData, *LoggerConfig) (LoggerProducer, error)
	// GetLoggerMetadata() LoggerMetadata
	//
	// // Sends a group of log messages to the API Server
	// SendLogMessages(context.Context, TaskData, []apimodels.LogMessage) error
	//
	// // The following operations use the legacy API server and are
	// // used by task commands.
	// SendTestResults(context.Context, TaskData, *task.LocalTestResults) error
	// SendTestLog(context.Context, TaskData, *model.TestLog) (string, error)
	// GetTaskPatch(context.Context, TaskData) (*patchmodel.Patch, error)
	// GetPatchFile(context.Context, TaskData, string) (string, error)
	//
	// // The following operations are used by
	// AttachFiles(context.Context, TaskData, []*artifact.File) error
	// GetManifest(context.Context, TaskData) (*manifest.Manifest, error)
	// S3Copy(context.Context, TaskData, *apimodels.S3CopyRequest) (string, error)
	// KeyValInc(context.Context, TaskData, *model.KeyVal) error
	//
	// // these are for the taskdata/json plugin that saves perf data
	// PostJSONData(context.Context, TaskData, string, interface{}) error
	// GetJSONData(context.Context, TaskData, string, string, string) ([]byte, error)
	// GetJSONHistory(context.Context, TaskData, bool, string, string) ([]byte, error)
	//
	// // GenerateTasks posts new tasks for the `generate.tasks` command.
	// GenerateTasks(context.Context, TaskData, []json.RawMessage) error
	//
	// // GenerateTasksPoll polls for new tasks for the `generate.tasks` command.
	// GenerateTasksPoll(context.Context, TaskData) (*apimodels.GeneratePollResponse, error)
	//
	// // Spawn-hosts for tasks methods
	// CreateHost(context.Context, TaskData, apimodels.CreateHost) ([]string, error)
	// ListHosts(context.Context, TaskData) ([]restmodel.CreateHost, error)

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

	// Fetch list of distributions evergreen can spawn
	GetDistrosList(context.Context) ([]restmodel.APIDistro, error)

	// Fetch the current authenticated user's public keys
	GetCurrentUsersKeys(context.Context) ([]restmodel.APIPubKey, error)

	AddPublicKey(context.Context, string, string) error

	// Delete a key with specified name from the current authenticated user
	DeletePublicKey(context.Context, string) error

	// List variant/task aliases
	ListAliases(context.Context, string) ([]model.ProjectAlias, error)
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
	CreatePatchForMerge(ctx context.Context, patchID string) (*restmodel.APIPatch, error)
	GetMessageForPatch(ctx context.Context, patchID string) (string, error)

	// Notifications
	SendNotification(ctx context.Context, notificationType string, data interface{}) error

	// kim: TODO: remove
	// GetDockerLogs returns logs for the given docker container
	// GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error)
	// GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error)

	// GetManifestByTask returns the manifest corresponding to the given task
	GetManifestByTask(ctx context.Context, taskId string) (*manifest.Manifest, error)

	GetRecentVersionsForProject(ctx context.Context, projectID, requester string) ([]restmodel.APIVersion, error)

	// GetTaskSyncReadCredentials returns the credentials to fetch task
	// directory from S3.
	GetTaskSyncReadCredentials(ctx context.Context) (*evergreen.S3Credentials, error)
	// GetTaskSyncPath returns the path to the task directory in S3.
	GetTaskSyncPath(ctx context.Context, taskID string) (string, error)

	// kim: NOTE: only used by the agent monitor.
	// GetClientURLs returns the all URLs that can be used to request the
	// Evergreen binary for a given distro.
	GetClientURLs(ctx context.Context, distroID string) ([]string, error)
}
