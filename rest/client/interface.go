package client

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip/message"
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
	// GetVersion loads the task's version.
	GetVersion(context.Context, TaskData) (*version.Version, error)
	// Heartbeat sends a heartbeat to the API server. The server can respond with
	// an "abort" response. This function returns true if the agent should abort.
	Heartbeat(context.Context, TaskData) (bool, error)
	// FetchExpansionVars loads expansions for a communicator's task from the API server.
	FetchExpansionVars(context.Context, TaskData) (*apimodels.ExpansionVars, error)
	// GetNextTask returns a next task response by getting the next task for a given host.
	GetNextTask(context.Context) (*apimodels.NextTaskResponse, error)

	// Constructs a new LogProducer instance for use by tasks.
	GetLoggerProducer(context.Context, TaskData) LoggerProducer

	// Sends a group of log messages to the API Server
	SendLogMessages(context.Context, TaskData, []apimodels.LogMessage) error
	SendProcessInfo(context.Context, TaskData, []*message.ProcessInfo) error
	SendSystemInfo(context.Context, TaskData, *message.SystemInfo) error

	// The following operations use the legacy API server and are
	// used by task commands.
	SendTestResults(context.Context, TaskData, *task.LocalTestResults) error
	SendTestLog(context.Context, TaskData, *model.TestLog) (string, error)
	GetTaskPatch(context.Context, TaskData) (*patchmodel.Patch, error)
	GetPatchFile(context.Context, TaskData, string) (string, error)

	// The following operations are used by
	AttachFiles(context.Context, TaskData, []*artifact.File) error
	GetManifest(context.Context, TaskData) (*manifest.Manifest, error)
	S3Copy(context.Context, TaskData, *apimodels.S3CopyRequest) error
	KeyValInc(context.Context, TaskData, *model.KeyVal) error

	// these are for the taskdata/json plugin that saves perf data
	PostJSONData(context.Context, TaskData, string, interface{}) error
	GetJSONData(context.Context, TaskData, string, string, string) ([]byte, error)
	GetJSONHistory(context.Context, TaskData, bool, string, string) ([]byte, error)

	// GenerateTasks posts new tasks for the `generate.tasks` command.
	GenerateTasks(context.Context, TaskData, interface{}) error

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

	// Host methods
	GetHostsByUser(context.Context, string) ([]*restmodel.APIHost, error)

	// Spawnhost methods
	//
	CreateSpawnHost(context.Context, string, string) (*restmodel.APIHost, error)
	TerminateSpawnHost(context.Context, string) error
	ChangeSpawnHostPassword(context.Context, string, string) error
	ExtendSpawnHostExpiration(context.Context, string, int) error
	GetHosts(context.Context, func([]*restmodel.APIHost) error) error

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
}
