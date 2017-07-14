package client

import (
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"golang.org/x/net/context"
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
	GetLoggerProducer(TaskData) LoggerProducer

	// Sends a group of log messages to the API Server
	SendLogMessages(context.Context, TaskData, []apimodels.LogMessage) error

	// The following operations use the legacy API server and are
	// used by task commands.
	SendTestResults(context.Context, TaskData, *task.TestResults) error
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

	// ---------------------------------------------------------------------
	// End legacy API methods
	// ---------------------------------------------------------------------

	// ---------------------------------------------------------------------
	// Begin REST API V2 methods
	// ---------------------------------------------------------------------
	// Setters
	//
	// SetAPIUser sets the API user.
	SetAPIUser(user string)
	// SetAPIKey sets the API key.
	SetAPIKey(apiKey string)

	// Host methods
	//
	GetAllHosts()
	GetHostByID()
	SetHostStatus()
	SetHostStatuses()

	// Spawnhost methods
	//
	CreateSpawnHost(ctx context.Context, distroID string, keyName string) (*restmodel.SpawnHost, error)
	GetSpawnHosts()

	// Task methods
	//
	GetTaskByID()
	GetTasksByBuild()
	GetTasksByProjectAndCommit()
	SetTaskStatus()
	AbortTask()
	RestartTask()

	// SSH keys methods
	//
	GetKeys()
	AddKey()
	RemoveKey()

	// Project methods
	//
	GetProjectByID()
	EditProject()
	CreateProject()
	GetAllProjects()

	// Build methods
	//
	GetBuildByID()
	GetBuildByProjectAndHashAndVariant()
	GetBuildsByVersion()
	SetBuildStatus()
	AbortBuild()
	RestartBuild()

	// Test methods
	//
	GetTestsByTaskID()
	GetTestsByBuild()
	GetTestsByTestName()

	// Version methods
	//
	GetVersionByID()
	GetVersions()
	GetVersionByProjectAndCommit()
	GetVersionsByProject()
	SetVersionStatus()
	AbortVersion()
	RestartVersion()

	// Distro methods
	//
	GetAllDistros()
	GetDistroByID()
	CreateDistro()
	EditDistro()
	DeleteDistro()
	GetDistroSetupScriptByID()
	GetDistroTeardownScriptByID()
	EditDistroSetupScript()
	EditDistroTeardownScript()

	// Patch methods
	//
	GetPatchByID()
	GetPatchesByProject()
	SetPatchStatus()
	AbortPatch()
	RestartPatch()
	// ---------------------------------------------------------------------
	// End REST API V2 methods
	// ---------------------------------------------------------------------
}
