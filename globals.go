package evergreen

import (
	"os"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	User            = "mci"
	GithubPatchUser = "github_pull_request"

	HostRunning         = "running"
	HostTerminated      = "terminated"
	HostUninitialized   = "initializing"
	HostBuilding        = "building"
	HostStarting        = "starting"
	HostProvisioning    = "provisioning"
	HostProvisionFailed = "provision failed"
	HostQuarantined     = "quarantined"
	HostDecommissioned  = "decommissioned"

	HostExternalUserName = "external"

	HostStatusSuccess = "success"
	HostStatusFailed  = "failed"

	// Task Statuses used in the database models

	// TaskInactive is not assigned to any new tasks, but can be found
	// in the database and is used in the UI.
	TaskInactive = "inactive"

	// TaskUnstarted is assigned to a display task after cleaning up one of
	// its execution tasks. This indicates that the display task is
	// pending a rerun
	TaskUnstarted = "unstarted"

	// TaskUndispatched indicates either
	//  1. a task is not scheduled to run (when Task.Activated == false)
	//  2. a task is scheduled to run (when Task.Activated == true)
	TaskUndispatched = "undispatched"

	// TaskStarted indicates a task is running on an agent
	TaskStarted = "started"

	// TaskDispatched indicates that an agent has received the task, but
	// the agent has not yet told Evergreen that it's running the task
	TaskDispatched = "dispatched"

	// The task statuses below indicate that a task has finished.
	TaskSucceeded = "success"

	// These statuses indicate the types of failures that are stored in
	// Task.Status field, build TaskCache and TaskEndDetails.
	TaskFailed       = "failed"
	TaskSystemFailed = "system-failed"
	TaskTestTimedOut = "test-timed-out"
	TaskSetupFailed  = "setup-failed"

	TaskStatusBlocked = "blocked"
	TaskStatusPending = "pending"

	// Task Command Types
	CommandTypeTest   = "test"
	CommandTypeSystem = "system"
	CommandTypeSetup  = "setup"

	// Task Statuses that are currently used only by the UI, and in tests
	// (these may be used in old tasks)
	TaskSystemUnresponse = "system-unresponsive"
	TaskSystemTimedOut   = "system-timed-out"
	TaskTimedOut         = "task-timed-out"

	// TaskConflict is used only in communication with the Agent
	TaskConflict = "task-conflict"

	TestFailedStatus         = "fail"
	TestSilentlyFailedStatus = "silentfail"
	TestSkippedStatus        = "skip"
	TestSucceededStatus      = "pass"

	BuildStarted   = "started"
	BuildCreated   = "created"
	BuildFailed    = "failed"
	BuildSucceeded = "success"

	VersionStarted   = "started"
	VersionCreated   = "created"
	VersionFailed    = "failed"
	VersionSucceeded = "success"

	PatchCreated   = "created"
	PatchStarted   = "started"
	PatchSucceeded = "succeeded"
	PatchFailed    = "failed"

	PushLogPushing = "pushing"
	PushLogSuccess = "success"

	HostTypeStatic = "static"

	PushStage = "push"

	MergeTestStarted   = "started"
	MergeTestSucceeded = "succeeded"
	MergeTestFailed    = "failed"

	// maximum task (zero based) execution number
	MaxTaskExecution = 3

	// maximum task priority
	MaxTaskPriority = 100

	// LogMessage struct versions
	LogmessageFormatTimestamp = 1
	LogmessageCurrentVersion  = LogmessageFormatTimestamp

	EvergreenHome = "EVGHOME"
	MongodbUrl    = "MONGO_URL"

	// Special logging output targets
	LocalLoggingOverride          = "LOCAL"
	StandardOutputLoggingOverride = "STDOUT"

	DefaultTaskActivator   = ""
	StepbackTaskActivator  = "stepback"
	APIServerTaskActivator = "apiserver"

	// Restart Types
	RestartVersions = "versions"
	RestartTasks    = "tasks"

	RestRoutePrefix = "rest"
	APIRoutePrefix  = "api"

	AgentAPIVersion  = 2
	APIRoutePrefixV2 = "/rest/v2"

	AgentMonitorTag = "agent-monitor"

	DegradedLoggingPercent = 10

	SetupScriptName     = "setup.sh"
	TempSetupScriptName = "setup-temp.sh"
	TeardownScriptName  = "teardown.sh"

	RoutePaginatorNextPageHeaderKey = "Link"

	PlannerVersionLegacy  = "legacy"
	PlannerVersionRevised = "revised"
	PlannerVersionTunable = "tunable"

	// maximum turnaround we want to maintain for all hosts for a given distro
	MaxDurationPerDistroHost               = 30 * time.Minute
	MaxDurationPerDistroHostWithContainers = 2 * time.Minute

	FinderVersionLegacy    = "legacy"
	FinderVersionParallel  = "parallel"
	FinderVersionPipeline  = "pipeline"
	FinderVersionAlternate = "alternate"

	HostAllocatorDuration    = "duration"
	HostAllocatorDeficit     = "deficit"
	HostAllocatorUtilization = "utilization"

	TaskOrderingNotDeclared   = ""
	TaskOrderingInterleave    = "interleave"
	TaskOrderingMainlineFirst = "mainlinefirst"
	TaskOrderingPatchFirst    = "patchfirst"

	CommitQueueAlias = "__commit_queue"
	MergeTaskVariant = "commit-queue-merge"
	MergeTaskName    = "merge-patch"
	MergeTaskGroup   = "merge-task-group"

	MaxTeardownGroupTimeoutSecs = 30 * 60

	DefaultJasperPort          = 2385
	GlobalGitHubTokenExpansion = "global_github_oauth_token"
)

func IsFinishedTaskStatus(status string) bool {
	if status == TaskSucceeded ||
		IsFailedTaskStatus(status) {
		return true
	}

	return false
}

func IsFailedTaskStatus(status string) bool {
	return status == TaskFailed ||
		status == TaskSystemFailed ||
		status == TaskSystemTimedOut ||
		status == TaskSystemUnresponse ||
		status == TaskTestTimedOut ||
		status == TaskSetupFailed
}

// evergreen package names
const (
	UIPackage      = "EVERGREEN_UI"
	RESTV2Package  = "EVERGREEN_REST_V2"
	MonitorPackage = "EVERGREEN_MONITOR"
)

const (
	AuthTokenCookie     = "mci-token"
	TaskHeader          = "Task-Id"
	TaskSecretHeader    = "Task-Secret"
	HostHeader          = "Host-Id"
	HostSecretHeader    = "Host-Secret"
	ContentTypeHeader   = "Content-Type"
	ContentTypeValue    = "application/json"
	ContentLengthHeader = "Content-Length"
	APIUserHeader       = "Api-User"
	APIKeyHeader        = "Api-Key"
)

// cloud provider related constants
const (
	ProviderNameEc2Auto     = "ec2-auto"
	ProviderNameEc2OnDemand = "ec2-ondemand"
	ProviderNameEc2Spot     = "ec2-spot"
	ProviderNameEc2Fleet    = "ec2-fleet"
	ProviderNameDocker      = "docker"
	ProviderNameDockerMock  = "docker-mock"
	ProviderNameGce         = "gce"
	ProviderNameStatic      = "static"
	ProviderNameOpenstack   = "openstack"
	ProviderNameVsphere     = "vsphere"
	ProviderNameMock        = "mock"

	// Default EC2 region where hosts should be spawned
	DefaultEC2Region = "us-east-1"
)

var (
	// Providers where hosts can be created and terminated automatically.
	ProviderSpawnable = []string{
		ProviderNameDocker,
		ProviderNameEc2OnDemand,
		ProviderNameEc2Spot,
		ProviderNameEc2Auto,
		ProviderNameEc2Fleet,
		ProviderNameGce,
		ProviderNameOpenstack,
		ProviderNameVsphere,
		ProviderNameMock,
	}

	ProviderContainer = []string{
		ProviderNameDocker,
	}

	SystemVersionRequesterTypes = []string{
		RepotrackerVersionRequester,
		TriggerRequester,
	}
)

const (
	DefaultServiceConfigurationFileName = "/etc/mci_settings.yml"
	DefaultDatabaseUrl                  = "mongodb://localhost:27017"
	DefaultDatabaseName                 = "mci"
	DefaultDatabaseWriteMode            = "majority"

	// database and config directory, set to the testing version by default for safety
	NotificationsFile = "mci-notifications.yml"
	ClientDirectory   = "clients"

	// version requester types
	PatchVersionRequester       = "patch_request"
	GithubPRRequester           = "github_pull_request"
	RepotrackerVersionRequester = "gitter_request"
	TriggerRequester            = "trigger_request"
	MergeTestRequester          = "merge_test"
	AdHocRequester              = "ad_hoc"
)

const (
	GenerateTasksCommandName = "generate.tasks"
	CreateHostCommandName    = "host.create"
)

type SenderKey int

const (
	SenderGithubStatus = SenderKey(iota)
	SenderEvergreenWebhook
	SenderSlack
	SenderJIRAIssue
	SenderJIRAComment
	SenderEmail
	SenderGithubMerge
	SenderCommitQueueDequeue
)

func (k SenderKey) Validate() error {
	switch k {
	case SenderGithubStatus, SenderEvergreenWebhook, SenderSlack, SenderJIRAComment, SenderJIRAIssue,
		SenderEmail, SenderGithubMerge, SenderCommitQueueDequeue:
		return nil
	default:
		return errors.New("invalid sender defined")
	}
}

func (k SenderKey) String() string {
	switch k {
	case SenderGithubStatus:
		return "github-status"
	case SenderEmail:
		return "email"
	case SenderEvergreenWebhook:
		return "webhook"
	case SenderSlack:
		return "slack"
	case SenderJIRAComment:
		return "jira-comment"
	case SenderJIRAIssue:
		return "jira-issue"
	case SenderGithubMerge:
		return "github-merge"
	case SenderCommitQueueDequeue:
		return "commit-queue-dequeue"
	default:
		return "<error:unkwown>"
	}
}

const (
	defaultLogBufferingDuration                  = 20
	defaultAmboyPoolSize                         = 2
	defaultAmboyLocalStorageSize                 = 1024
	defaultAmboyQueueName                        = "evg.service"
	defaultSingleAmboyQueueName                  = "evg.single"
	defaultAmboyDBName                           = "amboy"
	defaultGroupWorkers                          = 1
	defaultGroupBackgroundCreateFrequencyMinutes = 10
	defaultGroupPruneFrequencyMinutes            = 10
	defaultGroupTTLMinutes                       = 1
	maxNotificationsPerSecond                    = 100

	EnableAmboyRemoteReporting = false
)

// NameTimeFormat is the format in which to log times like instance start time.
const NameTimeFormat = "20060102150405"

var (
	PatchRequesters = []string{
		PatchVersionRequester,
		GithubPRRequester,
		MergeTestRequester,
	}

	// UpHostStatus is a list of all host statuses that are considered up.
	UpHostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostBuilding,
		HostStarting,
		HostProvisioning,
		HostProvisionFailed,
	}

	// DownHostStatus is a list of all host statuses that are considered down.
	DownHostStatus = []string{
		HostTerminated,
		HostQuarantined,
		HostDecommissioned,
	}

	// NotRunningStatus is a list of host statuses from before the host starts running.
	NotRunningStatus = []string{
		HostUninitialized,
		HostBuilding,
		HostProvisioning,
		HostStarting,
	}

	// Hosts in "initializing" status aren't actually running yet:
	// they're just intents, so this list omits that value.
	ActiveStatus = []string{
		HostRunning,
		HostStarting,
		HostProvisioning,
		HostProvisionFailed,
	}

	// Set of host status values that can be user set via the API
	ValidUserSetStatus = []string{
		HostRunning,
		HostTerminated,
		HostQuarantined,
		HostDecommissioned,
	}

	// Set of valid PlannerSettings.Version strings that can be user set via the API
	ValidPlannerVersions = []string{
		PlannerVersionLegacy,
		PlannerVersionRevised,
		PlannerVersionTunable,
	}

	// Set of valid FinderSettings.Version strings that can be user set via the API
	ValidFinderVersions = []string{
		FinderVersionLegacy,
		FinderVersionAlternate,
		FinderVersionParallel,
		FinderVersionPipeline,
	}

	// Set of valid Host Allocators types
	ValidHostAllocators = []string{
		HostAllocatorDuration,
		HostAllocatorDeficit,
		HostAllocatorUtilization,
	}

	// Set of valid Task Ordering options that can be user set via the API
	ValidTaskOrderings = []string{
		TaskOrderingNotDeclared,
		TaskOrderingInterleave,
		TaskOrderingMainlineFirst,
		TaskOrderingPatchFirst,
	}

	// constant arrays for db update logic
	AbortableStatuses = []string{TaskStarted, TaskDispatched}
	CompletedStatuses = []string{TaskSucceeded, TaskFailed}

	ValidCommandTypes = []string{CommandTypeSetup, CommandTypeSystem, CommandTypeTest}
)

// FindEvergreenHome finds the directory of the EVGHOME environment variable.
func FindEvergreenHome() string {
	// check if env var is set
	root := os.Getenv(EvergreenHome)
	if len(root) > 0 {
		return root
	}

	grip.Errorf("%s is unset", EvergreenHome)
	return ""
}

// IsSystemActivator returns true when the task activator is Evergreen.
func IsSystemActivator(caller string) bool {
	return caller == DefaultTaskActivator || caller == APIServerTaskActivator
}

func IsPatchRequester(requester string) bool {
	return requester == PatchVersionRequester || requester == GithubPRRequester || requester == MergeTestRequester
}

func IsGitHubPatchRequester(requester string) bool {
	return requester == GithubPRRequester || requester == MergeTestRequester
}
