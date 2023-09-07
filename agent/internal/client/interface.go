package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/manifest"
	patchmodel "github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"google.golang.org/grpc"
)

type Communicator interface {
	SharedCommunicator

	// The following operations are not implemented at the base level
	// and require either a host or pod-specific implementation.
	// EndTask marks the task as finished with the given status
	EndTask(context.Context, *apimodels.TaskEndDetail, TaskData) (*apimodels.EndTaskResponse, error)
	// GetNextTask returns a next task response by getting the next task for a given host.
	GetNextTask(context.Context, *apimodels.GetNextTaskDetails) (*apimodels.NextTaskResponse, error)
	// GetAgentSetupData populates an agent with the necessary data, including secrets.
	GetAgentSetupData(context.Context) (*apimodels.AgentSetupData, error)
}

// SharedCommunicator contains operations that do not depend on the task’s
// runtime environment (i.e. host or container). They will only have a
// single non-mock implementation.
type SharedCommunicator interface {
	// Close is a method to release resources used by the communicator.
	Close()
	// UpdateLastMessageTime Updates the clients local concept of it's last updated
	// time; used by agents to determine timeouts.
	UpdateLastMessageTime()
	LastMessageAt() time.Time

	// SetTaskOutputVersion sets the task output version for the task.
	SetTaskOutputVersion(context.Context, TaskData, int) error
	// StartTask marks the task as started.
	StartTask(context.Context, TaskData) error
	// GetTask returns the active task.
	GetTask(context.Context, TaskData) (*task.Task, error)
	// GetDisplayTaskInfoFromExecution returns the display task info of an
	// execution task, if it exists. It will return an empty struct and no
	// error if the task is not part of a display task.
	GetDisplayTaskInfoFromExecution(context.Context, TaskData) (*apimodels.DisplayTaskInfo, error)
	// GetProjectRef loads the task's project ref.
	GetProjectRef(context.Context, TaskData) (*model.ProjectRef, error)
	// GetDistroView returns the view of the distro information for the task.
	GetDistroView(context.Context, TaskData) (*apimodels.DistroView, error)
	// GetDistroAMI gets the AMI for the given distro/region
	GetDistroAMI(context.Context, string, string, TaskData) (string, error)
	// GetProject loads the project using the task's version ID.
	GetProject(context.Context, TaskData) (*model.Project, error)
	// Heartbeat will return a non-empty task status if the agent should stop running the task.
	// Returning evergreen.TaskFailed means that the task has been aborted. An empty string
	// indicates the heartbeat has succeeded.
	Heartbeat(context.Context, TaskData) (string, error)
	// GetExpansionsAndVars returns the expansions, project variables, and
	// version parameters. For expansions, all expansions are loaded except for
	// the expansions defined for this task's build variant. For variables,
	// project variables, project private variables, and version parameters are
	// included, but not project parameters.
	GetExpansionsAndVars(context.Context, TaskData) (*apimodels.ExpansionsAndVars, error)
	// GetCedarConfig returns the Cedar service configuration.
	GetCedarConfig(context.Context) (*apimodels.CedarConfig, error)
	// GetCedarGRPCConn returns the client connection to cedar if it exists, or
	// creates it if it doesn't exist.
	GetCedarGRPCConn(context.Context) (*grpc.ClientConn, error)
	// SetResultsInfo sets the test results information in the task.
	SetResultsInfo(context.Context, TaskData, string, bool) error
	// GetDataPipesConfig returns the Data-Pipes service configuration.
	GetDataPipesConfig(context.Context) (*apimodels.DataPipesConfig, error)

	// GetPullRequestInfo takes in a PR number, owner, and repo and returns information from the corresponding pull request.
	GetPullRequestInfo(context.Context, TaskData, int, string, string, bool) (*apimodels.PullRequestInfo, error)

	// DisableHost signals to the app server that the host should be disabled.
	DisableHost(context.Context, string, apimodels.DisableInfo) error

	// GetLoggerProducer constructs a new LogProducer instance for use by tasks.
	GetLoggerProducer(context.Context, TaskData, *LoggerConfig) (LoggerProducer, error)
	// GetLoggerMetadata() LoggerMetadata

	// SendLogMessages sends a group of log messages to the API Server
	SendLogMessages(context.Context, TaskData, []apimodels.LogMessage) error

	// The following operations are used by task commands.
	SendTestLog(context.Context, TaskData, *model.TestLog) (string, error)
	GetTaskPatch(context.Context, TaskData, string) (*patchmodel.Patch, error)
	GetPatchFile(context.Context, TaskData, string) (string, error)

	NewPush(context.Context, TaskData, *apimodels.S3CopyRequest) (*model.PushLog, error)
	UpdatePushStatus(context.Context, TaskData, *model.PushLog) error
	AttachFiles(context.Context, TaskData, []*artifact.File) error
	GetManifest(context.Context, TaskData) (*manifest.Manifest, error)
	KeyValInc(context.Context, TaskData, *model.KeyVal) error

	// GenerateTasks posts new tasks for the `generate.tasks` command.
	GenerateTasks(context.Context, TaskData, []json.RawMessage) error

	// GenerateTasksPoll polls for new tasks for the `generate.tasks` command.
	GenerateTasksPoll(context.Context, TaskData) (*apimodels.GeneratePollResponse, error)

	// Spawn-hosts for tasks methods
	CreateHost(context.Context, TaskData, apimodels.CreateHost) ([]string, error)
	ListHosts(context.Context, TaskData) (restmodel.HostListResults, error)

	// GetDockerLogs returns logs for the given docker container
	GetDockerLogs(ctx context.Context, hostID string, startTime time.Time, endTime time.Time, isError bool) ([]byte, error)
	GetDockerStatus(ctx context.Context, hostID string) (*cloud.ContainerStatus, error)

	// ConcludeMerge reports the status of a commit queue merge back to the server
	ConcludeMerge(ctx context.Context, patchId, status string, td TaskData) error
	GetAdditionalPatches(ctx context.Context, patchId string, td TaskData) ([]string, error)

	SetDownstreamParams(ctx context.Context, downstreamParams []patchmodel.Parameter, taskData TaskData) error
}

// TaskData contains the taskData.ID and taskData.Secret. It must be set for
// some client methods.
type TaskData struct {
	ID                 string
	Secret             string
	OverrideValidation bool
}

type LoggerConfig struct {
	System             []LogOpts
	Agent              []LogOpts
	Task               []LogOpts
	SendToGlobalSender bool
}

type LogOpts struct {
	Sender          string
	SplunkServerURL string
	SplunkToken     string
	Filepath        string
	BuilderID       string
	BufferDuration  time.Duration
	BufferSize      int
}

// LoggerProducer provides a mechanism for agents (and command plugins) to access the
// process' logging facilities. The interfaces are all based on grip
// interfaces and abstractions, and the behavior of the interfaces is
// dependent on the configuration and implementation of the
// LoggerProducer instance.
type LoggerProducer interface {
	// The Execution/Task/System loggers provide a grip-like
	// logging interface for the distinct logging channels that the
	// Evergreen agent provides to tasks
	Execution() grip.Journaler
	Task() grip.Journaler
	System() grip.Journaler

	// Flush flushes the underlying senders.
	Flush(context.Context) error

	// Close releases all resources by calling Close on all underlying senders.
	Close() error
	// Closed returns true if this logger has been closed, false otherwise.
	Closed() bool
}
