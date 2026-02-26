package evergreen

import (
	"os"
	"strings"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// User is the generic user representing the Evergreen application as a
	// whole entity. If there's a more specific user performing an operation,
	// prefer to use that instead.
	User              = "mci"
	GithubPatchUser   = "github_pull_request"
	GithubMergeUser   = "github_merge_queue"
	PeriodicBuildUser = "periodic_build_user"
	ParentPatchUser   = "parent_patch"

	HostRunning       = "running"
	HostTerminated    = "terminated"
	HostUninitialized = "initializing"
	// HostBuilding is an intermediate state indicating that the intent host is
	// attempting to create a real host from an intent host, but has not
	// successfully done so yet.
	HostBuilding = "building"
	// HostBuildingFailed is a failure state indicating that an intent host was
	// attempting to create a host but failed during creation. Hosts that fail
	// to build will terminate shortly.
	HostBuildingFailed = "building-failed"
	HostStarting       = "starting"
	HostProvisioning   = "provisioning"
	// HostProvisionFailed is a failure state indicating that a host was
	// successfully created (i.e. requested from the cloud provider) but failed
	// while it was starting up. Hosts that fail to provisoin will terminate
	// shortly.
	HostProvisionFailed = "provision failed"
	HostQuarantined     = "quarantined"
	HostDecommissioned  = "decommissioned"

	HostStopping = "stopping"
	HostStopped  = "stopped"

	HostExternalUserName = "external"

	// Task statuses stored in the database (i.e. (Task).Status):

	// TaskUndispatched indicates either:
	//  1. a task is not scheduled to run (when Task.Activated == false)
	//  2. a task is scheduled to run (when Task.Activated == true)
	TaskUndispatched = "undispatched"

	// TaskDispatched indicates that an agent has received the task, but
	// the agent has not yet told Evergreen that it's running the task
	TaskDispatched = "dispatched"

	// TaskStarted indicates a task is running on an agent.
	TaskStarted = "started"

	// The task statuses below indicate that a task has finished.
	// TaskSucceeded indicates that the task has finished and is successful.
	TaskSucceeded = "success"

	// TaskFailed indicates that the task has finished and failed. This
	// encompasses any task failure, regardless of the specific failure reason
	// which can be found in the task end details.
	TaskFailed = "failed"

	// Task statuses used for the UI or other special-case purposes:

	// TaskUnscheduled indicates that the task is undispatched and is not
	// scheduled to eventually run. This is a display status, so it's only used
	// in the UI.
	TaskUnscheduled = "unscheduled"
	// TaskInactive is a deprecated legacy status that used to mean that the
	// task was not scheduled to run. This is equivalent to the TaskUnscheduled
	// display status. These are not stored in the task status (although they
	// used to be for very old tasks) but may be still used in some outdated
	// pieces of code.
	TaskInactive = "inactive"

	// TaskWillRun indicates that the task is scheduled to eventually run,
	// unless one of its dependencies becomes unattainable. This is a display
	// status, so it's only used in the UI.
	TaskWillRun = "will-run"

	// All other task failure reasons other than TaskFailed are display
	// statuses, so they're only used in the UI. These are not stored in the
	// task status (although they used to be for very old tasks).
	TaskSystemFailed = "system-failed"
	TaskTestTimedOut = "test-timed-out"
	TaskSetupFailed  = "setup-failed"

	// TaskAborted indicates that the task was aborted while it was running.
	TaskAborted = "aborted"

	// TaskStatusBlocked indicates that the task cannot run because it is
	// blocked by an unattainable dependency. This is a display status, so it's
	// only used in the UI.
	TaskStatusBlocked = "blocked"

	// TaskKnownIssue indicates that the task has failed and is being tracked by
	// a linked issue in the task annotations. This is a display status, so it's
	// only used in the UI.
	TaskKnownIssue = "known-issue"

	// TaskStatusPending is a special state that's used for one specific return
	// value. Generally do not use this status as it is neither a meaningful
	// status in the UI nor in the back end.
	TaskStatusPending = "pending"

	// TaskAll is not a status, but rather a UI filter indicating that it should
	// select all tasks regardless of their status.
	TaskAll = "all"

	// Task Command Types
	CommandTypeTest   = "test"
	CommandTypeSystem = "system"
	CommandTypeSetup  = "setup"

	// Task descriptions
	//
	// TaskDescriptionHeartbeat indicates that a task failed because it did not
	// send a heartbeat while it was running. Tasks are expected to send
	// periodic heartbeats back to the app server indicating the task is still
	// actively running, or else they are considered stale.
	TaskDescriptionHeartbeat = "heartbeat"
	// TaskDescriptionStranded indicates that a task failed because its
	// underlying runtime environment (i.e. container or host) encountered an
	// issue. For example, if a host is terminated while the task is still
	// running, the task is considered stranded.
	TaskDescriptionStranded = "stranded"
	// TaskDescriptionNoResults indicates that a task failed because it did not
	// post any test results.
	TaskDescriptionNoResults = "expected test results, but none attached"
	// TaskDescriptionResultsFailed indicates that a task failed because the
	// test results contained a failure.
	TaskDescriptionResultsFailed = "test results contained failing test"
	// TaskDescriptionContainerUnallocatable indicates that the reason a
	// container task failed is because it cannot be allocated a container.
	TaskDescriptionContainerUnallocatable = "container task cannot be allocated"
	// TaskDescriptionAborted indicates that the reason a task failed is specifically
	// because it was manually aborted.
	TaskDescriptionAborted = "aborted"

	// Task Statuses that are only used by the UI, event log  and tests
	// (these may be used in old tasks as actual task statuses rather than just
	// task display statuses).
	TaskSystemUnresponse = "system-unresponsive"
	TaskSystemTimedOut   = "system-timed-out"
	TaskTimedOut         = "task-timed-out"

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
	// VersionAborted is a display status only and not stored in the DB
	VersionAborted = "aborted"

	PushLogPushing = "pushing"
	PushLogSuccess = "success"

	HostTypeStatic = "static"

	MergeTestSucceeded = "succeeded"
	MergeTestFailed    = "failed"

	// MaxAutomaticRestarts is the maximum number of automatic restarts allowed for a task
	MaxAutomaticRestarts = 1

	// MaxTaskDispatchAttempts is the maximum number of times a task can be
	// dispatched before it is considered to be in a bad state.
	MaxTaskDispatchAttempts = 5

	// maximum task priority
	MaxTaskPriority = 100

	DisabledTaskPriority = int64(-1)

	// if a patch has NumTasksForLargePatch number of tasks or greater, we log to splunk for investigation
	NumTasksForLargePatch = 10000

	DefaultEvergreenConfig = ".evergreen.yml"

	WhyIsMyDataMissingName = "whyIsTheDataMissing.txt"

	// Env vars
	EvergreenHome           = "EVGHOME"
	MongodbURL              = "MONGO_URL"
	SharedMongoURL          = "SHARED_MONGO_URL"
	MongoAWSAuthEnabled     = "MONGO_AWS_AUTH"
	EvergreenVersionID      = "EVG_VERSION_ID"
	EvergreenClientS3Bucket = "EVG_CLIENT_S3_BUCKET"
	TraceEndpoint           = "TRACE_ENDPOINT"
	SettingsOverride        = "SETTINGS_OVERRIDE"

	// localLoggingOverride is a special log path indicating that the app server
	// should attempt to log to systemd if available, and otherwise fall back to
	// logging to stdout.
	localLoggingOverride = "LOCAL"
	// standardOutputLoggingOverride is a special log path indicating that the
	// app server should log to stdout.
	standardOutputLoggingOverride = "STDOUT"
	// disableLocalLoggingEnvVar is an environment variable to disable all local application logging
	// besides for fallback logging to stderr.
	disableLocalLoggingEnvVar = "DISABLE_LOCAL_LOGGING"

	// APIServerTaskActivator represents Evergreen's internal API activator
	APIServerTaskActivator = "apiserver"
	// StepbackTaskActivator represents the activator for tasks activated
	// due to stepback.
	StepbackTaskActivator = "stepback"
	// CheckBlockedTasksActivator represents the activator for task deactivated
	// by the check blocked tasks job.
	CheckBlockedTasksActivator = "check-blocked-tasks-job-activator"
	// BuildActivator represents the activator for builds that have been
	// activated.
	BuildActivator = "build-activator"
	// ElapsedBuildActivator represents the activator for batch time builds
	// that have their batchtimes elapsed.
	ElapsedBuildActivator = "elapsed-build-activator"
	// ElapsedTaskActivator represents the activator for batch time tasks
	// that have their batchtimes elapsed.
	ElapsedTaskActivator = "elapsed-task-activator"
	// GenerateTasksActivator represents the activator for tasks that have been
	// generated by a task generator.
	GenerateTasksActivator = "generate-tasks-activator"
	// AutoRestartActivator represents the activator for tasks that have been
	// automatically restarted via the retry_on_failure command flag.
	AutoRestartActivator = "automatic_restart"

	// StaleContainerTaskMonitor is the special name representing the unit
	// responsible for monitoring container tasks that have not dispatched but
	// have waiting for a long time since their activation.
	StaleContainerTaskMonitor = "stale-container-task-monitor"
	// UnderwaterTaskUnscheduler is the caller associated with unscheduling
	// and disabling tasks older than the task.UnschedulableThreshold from
	// their distro queue.
	UnderwaterTaskUnscheduler = "underwater-task-unscheduler"

	RestRoutePrefix = "rest"
	APIRoutePrefix  = "api"

	APIRoutePrefixV2 = "/rest/v2"

	AgentMonitorTag = "agent-monitor"
	HostFetchTag    = "host-fetch"

	HostIDEnvVar     = "HOST_ID"
	HostSecretEnvVar = "HOST_SECRET"

	DegradedLoggingPercent = 10

	SetupScriptName               = "setup.sh"
	TempSetupScriptName           = "setup-temp.sh"
	PowerShellSetupScriptName     = "setup.ps1"
	PowerShellTempSetupScriptName = "setup-temp.ps1"
	SpawnhostFetchScriptName      = ".evergreen-spawnhost-fetch.sh"

	PlannerVersionTunable = "tunable"

	DispatcherVersionRevisedWithDependencies = "revised-with-dependencies"

	// maximum turnaround we want to maintain for all hosts for a given distro
	MaxDurationPerDistroHost               = 30 * time.Minute
	MaxDurationPerDistroHostWithContainers = 2 * time.Minute

	// Spawn hosts
	SpawnHostExpireDays              = 30
	HostExpireDays                   = 10
	ExpireOnFormat                   = "2006-01-02"
	DefaultMaxSpawnHostsPerUser      = 3
	DefaultSpawnHostExpiration       = 24 * time.Hour
	SpawnHostRespawns                = 2
	SpawnHostNoExpirationDuration    = 7 * 24 * time.Hour
	MaxVolumeExpirationDurationHours = 24 * time.Hour * 14
	UnattachedVolumeExpiration       = 24 * time.Hour * 30
	DefaultMaxVolumeSizePerUser      = 500
	DefaultUnexpirableHostsPerUser   = 1
	DefaultUnexpirableVolumesPerUser = 1
	DefaultSleepScheduleTimeZone     = "America/New_York"

	// host resource tag names
	TagName              = "name"
	TagDistro            = "distro"
	TagEvergreenService  = "evergreen-service"
	TagUsername          = "username"
	TagOwner             = "owner"
	TagMode              = "mode"
	TagStartTime         = "start-time"
	TagExpireOn          = "expire-on"
	TagAllowRemoteAccess = "AllowRemoteAccess"
	TagIsDebug           = "IsDebug"

	FinderVersionLegacy    = "legacy"
	FinderVersionParallel  = "parallel"
	FinderVersionPipeline  = "pipeline"
	FinderVersionAlternate = "alternate"

	HostAllocatorUtilization = "utilization"

	HostAllocatorRoundDown    = "round-down"
	HostAllocatorRoundUp      = "round-up"
	HostAllocatorRoundDefault = ""

	HostAllocatorWaitsOverThreshFeedback = "waits-over-thresh-feedback"
	HostAllocatorNoFeedback              = "no-feedback"
	HostAllocatorUseDefaultFeedback      = ""

	HostsOverallocatedTerminate  = "terminate-hosts-when-overallocated"
	HostsOverallocatedIgnore     = "no-terminations-when-overallocated"
	HostsOverallocatedUseDefault = ""

	// CommitQueueAlias and GithubPRAlias are special aliases to specify variants and tasks for the merge queue and GitHub PR patches
	CommitQueueAlias  = "__commit_queue"
	GithubPRAlias     = "__github"
	GithubChecksAlias = "__github_checks"
	GitTagAlias       = "__git_tag"

	DefaultJasperPort = 2385

	GithubAppPrivateKey = "github_app_key"
	GithubKnownHosts    = "github_known_hosts"
	GithubCheckRun      = "github_check_run"

	// GitHubRetryAttempts is the github client maximum number of attempts.
	GitHubRetryAttempts = 3
	// GitHubRetryMinDelay is the github client's minimum amount of delay before attempting another request.
	GithubRetryMinDelay = time.Second

	VSCodePort = 2021

	DefaultShutdownWaitSeconds = 10

	// HeartbeatTimeoutThreshold is the timeout for how long a task can run without sending
	// a heartbeat
	HeartbeatTimeoutThreshold = 7 * time.Minute

	// MaxTeardownGroupThreshold specifies the duration after which the host should no longer continue
	// to tear down a task group. This is set one minute longer than the agent's maxTeardownGroupTimeout.
	MaxTeardownGroupThreshold = 4 * time.Minute

	SaveGenerateTasksError          = "error saving config in `generate.tasks`"
	TasksAlreadyGeneratedError      = "generator already ran and generated tasks"
	KeyTooLargeToIndexError         = "key too large to index"
	InvalidDivideInputError         = "$divide only supports numeric types"
	FetchingTaskDataUnfinishedError = "fetching task data not finished"
	DistroNotFoundForTaskError      = "distro not found"

	// ContainerHealthDashboard is the name of the Splunk dashboard that displays
	// charts relating to the health of container tasks.
	ContainerHealthDashboard = "container task health dashboard"

	// PRTasksRunningDescription is the description for a GitHub PR status
	// indicating that there are still running tasks.
	PRTasksRunningDescription = "tasks are running"

	// ResmokeRenderingType is name used for the resmoke rendering type when rendering logs.
	ResmokeRenderingType = "resmoke"

	// RedactedValue is the value that is shown in the REST API and UI for redacted values.
	RedactedValue       = "{REDACTED}"
	RedactedAfterValue  = "{REDACTED_AFTER}"
	RedactedBeforeValue = "{REDACTED_BEFORE}"

	// PresignMinimumValidTime is the minimum amount of time that a presigned URL
	// should be valid for.
	PresignMinimumValidTime = 15 * time.Minute
)

var TestFailureStatuses = []string{
	TestFailedStatus,
	TestSilentlyFailedStatus,
}

var TaskStatuses = []string{
	TaskStarted,
	TaskSucceeded,
	TaskFailed,
	TaskSystemFailed,
	TaskTestTimedOut,
	TaskSetupFailed,
	TaskAborted,
	TaskStatusBlocked,
	TaskStatusPending,
	TaskKnownIssue,
	TaskSystemUnresponse,
	TaskSystemTimedOut,
	TaskTimedOut,
	TaskWillRun,
	TaskUnscheduled,
	TaskUndispatched,
	TaskDispatched,
}

// TaskSystemFailureStatuses contains only system failure statuses used by the UI and event logs.
var TaskSystemFailureStatuses = []string{
	TaskSystemFailed,
	TaskSystemUnresponse,
	TaskSystemTimedOut,
}

var InternalAliases = []string{
	CommitQueueAlias,
	GithubPRAlias,
	GithubChecksAlias,
	GitTagAlias,
}

// TaskNonGenericFailureStatuses represents some kind of specific abnormal
// failure mode. These are display statuses used in the UI.
var TaskNonGenericFailureStatuses = []string{
	TaskTimedOut,
	TaskSystemFailed,
	TaskTestTimedOut,
	TaskSetupFailed,
	TaskSystemUnresponse,
	TaskSystemTimedOut,
}

// TaskFailureStatuses represent all the ways that a completed task can fail,
// inclusive of display statuses such as system failures.
var TaskFailureStatuses = append([]string{TaskFailed}, TaskNonGenericFailureStatuses...)

var TaskUnstartedStatuses = []string{
	TaskInactive,
	TaskUndispatched,
}

func IsUnstartedTaskStatus(status string) bool {
	return utility.StringSliceContains(TaskUnstartedStatuses, status)
}

func IsFinishedTaskStatus(status string) bool {
	if status == TaskSucceeded ||
		IsFailedTaskStatus(status) {
		return true
	}

	return false
}

func IsFailedTaskStatus(status string) bool {
	return utility.StringSliceContains(TaskFailureStatuses, status)
}

func IsValidTaskEndStatus(status string) bool {
	return status == TaskSucceeded || status == TaskFailed
}

func IsFinishedBuildStatus(status string) bool {
	return status == BuildFailed || status == BuildSucceeded
}

// IsFinishedVersionStatus returns true if the version or patch is true.
func IsFinishedVersionStatus(status string) bool {
	return status == VersionFailed || status == VersionSucceeded
}

type ModificationAction string

// ModifySpawnHostSource determines the originating source of a spawn host
// modification.
type ModifySpawnHostSource string

const (
	// ModifySpawnHostManual means the spawn host is being modified manually
	// because a user requested it.
	ModifySpawnHostManual ModifySpawnHostSource = "manual"
	// ModifySpawnHostManual means the spawn host is being modified by the
	// automatic sleep schedule.
	ModifySpawnHostSleepSchedule ModifySpawnHostSource = "sleep_schedule"
	// ModifySpawnHostProjectSettings means the spawn host is being terminated
	// because the project settings changed to disable debug hosts.
	ModifySpawnHostProjectSettings ModifySpawnHostSource = "project_settings"
)

// Common OTEL constants and attribute keys
const (
	PackageName = "github.com/evergreen-ci/evergreen"

	OtelAttributeMaxLength = 10000
	// task otel attributes
	TaskIDOtelAttribute             = "evergreen.task.id"
	TaskNameOtelAttribute           = "evergreen.task.name"
	TaskExecutionOtelAttribute      = "evergreen.task.execution"
	TaskStatusOtelAttribute         = "evergreen.task.status"
	TaskFailureTypeOtelAttribute    = "evergreen.task.failure_type"
	TaskFailingCommandOtelAttribute = "evergreen.task.failing_command"
	TaskDescriptionOtelAttribute    = "evergreen.task.description"
	TaskTagsOtelAttribute           = "evergreen.task.tags"
	TaskActivatedTimeOtelAttribute  = "evergreen.task.activated_time"
	TaskOnDemandCostOtelAttribute   = "evergreen.task.on_demand_cost"
	TaskAdjustedCostOtelAttribute   = "evergreen.task.adjusted_cost"

	// task otel attributes
	DisplayTaskIDOtelAttribute   = "evergreen.display_task.id"
	DisplayTaskNameOtelAttribute = "evergreen.display_task.name"

	// version otel attributes
	VersionIDOtelAttribute                    = "evergreen.version.id"
	VersionRequesterOtelAttribute             = "evergreen.version.requester"
	VersionStatusOtelAttribute                = "evergreen.version.status"
	VersionCreateTimeOtelAttribute            = "evergreen.version.create_time"
	VersionStartTimeOtelAttribute             = "evergreen.version.start_time"
	VersionFinishTimeOtelAttribute            = "evergreen.version.finish_time"
	VersionAuthorOtelAttribute                = "evergreen.version.author"
	VersionBranchOtelAttribute                = "evergreen.version.branch"
	VersionMakespanSecondsOtelAttribute       = "evergreen.version.makespan_seconds"
	VersionTimeTakenSecondsOtelAttribute      = "evergreen.version.time_taken_seconds"
	VersionPRNumOtelAttribute                 = "evergreen.version.pr_num"
	VersionDescriptionOtelAttribute           = "evergreen.version.description"
	VersionOnDemandCostOtelAttribute          = "evergreen.version.on_demand_cost"
	VersionAdjustedCostOtelAttribute          = "evergreen.version.adjusted_cost"
	VersionPredictedOnDemandCostOtelAttribute = "evergreen.version.predicted_on_demand_cost"
	VersionPredictedAdjustedCostOtelAttribute = "evergreen.version.predicted_adjusted_cost"

	// patch otel attributes
	PatchIsReconfiguredOtelAttribute = "evergreen.patch.is_reconfigured"

	// build otel attributes
	BuildIDOtelAttribute   = "evergreen.build.id"
	BuildNameOtelAttribute = "evergreen.build.name"

	ProjectIdentifierOtelAttribute = "evergreen.project.identifier"
	ProjectOrgOtelAttribute        = "evergreen.project.org"
	ProjectRepoOtelAttribute       = "evergreen.project.repo"
	ProjectIDOtelAttribute         = "evergreen.project.id"
	RepoRefIDOtelAttribute         = "evergreen.project.repo_ref_id"
	DistroIDOtelAttribute          = "evergreen.distro.id"
	DistroProviderOtelAttribute    = "evergreen.distro.provider"
	HostIDOtelAttribute            = "evergreen.host.id"
	HostnameOtelAttribute          = "evergreen.host.hostname"
	HostStartedByOtelAttribute     = "evergreen.host.started_by"
	HostNoExpirationOtelAttribute  = "evergreen.host.no_expiration"
	HostInstanceTypeOtelAttribute  = "evergreen.host.instance_type"
	AggregationNameOtelAttribute   = "db.aggregationName"
)

const (
	RestartAction     ModificationAction = "restart"
	SetActiveAction   ModificationAction = "set_active"
	SetPriorityAction ModificationAction = "set_priority"
	AbortAction       ModificationAction = "abort"
)

// Constants for Evergreen package names (including legacy ones).
const (
	UIPackage      = "EVERGREEN_UI"
	RESTV2Package  = "EVERGREEN_REST_V2"
	MonitorPackage = "EVERGREEN_MONITOR"
)

var UserTriggeredOrigins = []string{
	UIPackage,
	RESTV2Package,
	GithubCheckRun,
}

const (
	AuthTokenCookie     = "mci-token"
	LoginCookieTTL      = 365 * 24 * time.Hour
	TaskHeader          = "Task-Id"
	TaskSecretHeader    = "Task-Secret"
	HostHeader          = "Host-Id"
	HostSecretHeader    = "Host-Secret"
	PodHeader           = "Pod-Id"
	PodSecretHeader     = "Pod-Secret"
	ContentTypeHeader   = "Content-Type"
	ContentTypeValue    = "application/json"
	ContentLengthHeader = "Content-Length"
	APIUserHeader       = "Api-User"
	APIKeyHeader        = "Api-Key"
	SageUserHeader      = "x-authenticated-sage-user"
	AuthorizationHeader = "Authorization"
	EnvironmentHeader   = "X-Evergreen-Environment"
)

const (
	// CredentialsCollection is the collection containing TLS credentials to
	// connect to a Jasper service running on a host.
	CredentialsCollection = "credentials"
	// CAName is the name of the root CA for the TLS credentials.
	CAName = "evergreen"
)

// Constants related to cloud providers and provider-specific settings.
const (
	ProviderNameEc2OnDemand = "ec2-ondemand"
	ProviderNameEc2Fleet    = "ec2-fleet"
	ProviderNameDocker      = "docker"
	ProviderNameDockerMock  = "docker-mock"
	ProviderNameStatic      = "static"
	ProviderNameMock        = "mock"

	// DefaultEC2Region is the default region where hosts should be spawned and
	// general Evergreen operations occur in AWS if no particular region is
	// specified.
	DefaultEC2Region = "us-east-1"
	// DefaultEBSType is Amazon's default EBS type.
	DefaultEBSType = "gp3"
	// DefaultEBSAvailabilityZone is the default availability zone for EBS
	// volumes. This may be a temporary default.
	DefaultEBSAvailabilityZone = "us-east-1a"
)

// IsEc2Provider returns true if the provider is ec2.
func IsEc2Provider(provider string) bool {
	return provider == ProviderNameEc2OnDemand ||
		provider == ProviderNameEc2Fleet
}

// IsDockerProvider returns true if the provider is docker.
func IsDockerProvider(provider string) bool {
	return provider == ProviderNameDocker ||
		provider == ProviderNameDockerMock
}

// EC2Tenancy represents the physical hardware tenancy for EC2 hosts.
type EC2Tenancy string

const (
	EC2TenancyDefault   EC2Tenancy = "default"
	EC2TenancyDedicated EC2Tenancy = "dedicated"
)

// ValidEC2Tenancies represents valid EC2 tenancy values.
var ValidEC2Tenancies = []EC2Tenancy{EC2TenancyDefault, EC2TenancyDedicated}

// IsValidEC2Tenancy returns if the given EC2 tenancy is valid.
func IsValidEC2Tenancy(tenancy EC2Tenancy) bool {
	return len(utility.FilterSlice(ValidEC2Tenancies, func(t EC2Tenancy) bool { return t == tenancy })) > 0
}

var (
	// ProviderSpawnable includes all cloud provider types where hosts can be
	// dynamically created and terminated according to need. This has no
	// relation to spawn hosts.
	ProviderSpawnable = []string{
		ProviderNameEc2OnDemand,
		ProviderNameEc2Fleet,
		ProviderNameMock,
		ProviderNameDocker,
	}

	// ProviderUserSpawnable includes all cloud provider types where a user can
	// request a dynamically created host for purposes such as host.create and
	// spawn hosts.
	ProviderUserSpawnable = []string{
		ProviderNameEc2OnDemand,
		ProviderNameEc2Fleet,
	}

	ProviderContainer = []string{
		ProviderNameDocker,
	}
)

const (
	DefaultDatabaseURL       = "mongodb://localhost:27017"
	DefaultDatabaseName      = "mci"
	DefaultDatabaseWriteMode = "majority"
	DefaultDatabaseReadMode  = "majority"

	DefaultAmboyDatabaseURL = "mongodb://localhost:27017"
	DefaultCedarDatabaseURL = "mongodb://localhost:27017"

	// version requester types
	PatchVersionRequester       = "patch_request"
	GithubPRRequester           = "github_pull_request"
	GitTagRequester             = "git_tag_request"
	RepotrackerVersionRequester = "gitter_request"
	TriggerRequester            = "trigger_request"
	AdHocRequester              = "ad_hoc"               // periodic build
	GithubMergeRequester        = "github_merge_request" // GitHub merge queue
	DebugRequester              = "debug_request"        // for debug spawn hosts
)

// Constants related to requester types.

var (
	// SystemVersionRequesterTypes contain non-patch requesters that are created by the Evergreen system, i.e. configs and patch files are unchanged by author.
	SystemVersionRequesterTypes = []string{
		RepotrackerVersionRequester,
		TriggerRequester,
		GitTagRequester,
		AdHocRequester,
	}
	AllRequesterTypes = []string{
		PatchVersionRequester,
		GithubPRRequester,
		GitTagRequester,
		RepotrackerVersionRequester,
		TriggerRequester,
		AdHocRequester,
		GithubMergeRequester,
		DebugRequester,
	}
)

// UserRequester represents the allowed user-facing requester types.
type UserRequester string

// Validate checks that the user-facing requester type is valid.
func (r UserRequester) Validate() error {
	if !utility.StringSliceContains(AllRequesterTypes, UserRequesterToInternalRequester(r)) {
		return errors.Errorf("invalid user requester '%s'", r)
	}
	return nil
}

const (
	// User-facing requester types. These are equivalent in meaning to the above
	// requesters, but are more user-friendly. These should only be used for
	// user-facing functionality such as YAML configuration and expansions and
	// should be translated into the true internal requester types so they're
	// actually usable.
	PatchVersionUserRequester       UserRequester = "patch"
	GithubPRUserRequester           UserRequester = "github_pr"
	GitTagUserRequester             UserRequester = "github_tag"
	RepotrackerVersionUserRequester UserRequester = "commit"
	TriggerUserRequester            UserRequester = "trigger"
	AdHocUserRequester              UserRequester = "ad_hoc"
	GithubMergeUserRequester        UserRequester = "github_merge_queue"
)

var AllUserRequesterTypes = []UserRequester{
	PatchVersionUserRequester,
	GithubPRUserRequester,
	GitTagUserRequester,
	RepotrackerVersionUserRequester,
	TriggerUserRequester,
	AdHocUserRequester,
	GithubMergeUserRequester,
}

// InternalRequesterToUserRequester translates an internal requester type to a
// user-facing requester type.
func InternalRequesterToUserRequester(requester string) UserRequester {
	switch requester {
	case PatchVersionRequester:
		return PatchVersionUserRequester
	case GithubPRRequester:
		return GithubPRUserRequester
	case GitTagRequester:
		return GitTagUserRequester
	case RepotrackerVersionRequester:
		return RepotrackerVersionUserRequester
	case TriggerRequester:
		return TriggerUserRequester
	case AdHocRequester:
		return AdHocUserRequester
	case GithubMergeRequester:
		return GithubMergeUserRequester
	default:
		return ""
	}
}

// UserRequesterToInternalRequester translates a user-facing requester type to
// its equivalent internal requester type.
func UserRequesterToInternalRequester(requester UserRequester) string {
	switch requester {
	case PatchVersionUserRequester:
		return PatchVersionRequester
	case GithubPRUserRequester:
		return GithubPRRequester
	case GitTagUserRequester:
		return GitTagRequester
	case RepotrackerVersionUserRequester:
		return RepotrackerVersionRequester
	case TriggerUserRequester:
		return TriggerRequester
	case AdHocUserRequester:
		return AdHocRequester
	case GithubMergeUserRequester:
		return GithubMergeRequester
	default:
		return ""
	}
}

// Constants for project command names.
const (
	GenerateTasksCommandName      = "generate.tasks"
	HostCreateCommandName         = "host.create"
	ShellExecCommandName          = "shell.exec"
	AttachResultsCommandName      = "attach.results"
	AttachArtifactsCommandName    = "attach.artifacts"
	AttachXUnitResultsCommandName = "attach.xunit_results"
)

var AttachCommands = []string{
	AttachResultsCommandName,
	AttachArtifactsCommandName,
	AttachXUnitResultsCommandName,
}

type SenderKey int

const (
	// SenderGithubStatus sends messages to GitHub like PR status updates. This
	// sender key logically represents all GitHub senders collectively, of which
	// there is one per GitHub org.
	SenderGithubStatus = SenderKey(iota)
	SenderEvergreenWebhook
	SenderSlack
	SenderJIRAIssue
	SenderJIRAComment
	SenderEmail
	SenderGeneric
)

func (k SenderKey) Validate() error {
	switch k {
	case SenderGithubStatus, SenderEvergreenWebhook, SenderSlack, SenderJIRAComment, SenderJIRAIssue,
		SenderEmail, SenderGeneric:
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
	case SenderGeneric:
		return "generic"
	default:
		return "<error:unknown>"
	}
}

// DevProdJiraServiceField defines a required field for DEVPROD tickets, which we sometimes auto-generate.
// Using "Other" prevents this from getting out of sync with service naming too quickly.
var DevProdJiraServiceField = map[string]string{
	"id":    devProdServiceId,
	"value": devProdServiceValue,
}

const (
	PriorityLevelEmergency = "emergency"
	PriorityLevelAlert     = "alert"
	PriorityLevelCritical  = "critical"
	PriorityLevelError     = "error"
	PriorityLevelWarning   = "warning"
	PriorityLevelNotice    = "notice"
	PriorityLevelInfo      = "info"
	PriorityLevelDebug     = "debug"
	PriorityLevelTrace     = "trace"
)

const (
	DevProdServiceFieldName = "customfield_24158"
	devProdServiceId        = "27020"
	devProdServiceValue     = "Other"
)

// Recognized Evergreen agent CPU architectures, which should be in the form
// ${GOOS}_${GOARCH}.
const (
	ArchDarwinAmd64  = "darwin_amd64"
	ArchDarwinArm64  = "darwin_arm64"
	ArchLinuxPpc64le = "linux_ppc64le"
	ArchLinuxS390x   = "linux_s390x"
	ArchLinuxArm64   = "linux_arm64"
	ArchLinuxAmd64   = "linux_amd64"
	ArchWindowsAmd64 = "windows_amd64"
)

// NameTimeFormat is the format in which to log times like instance start time.
const NameTimeFormat = "20060102150405"

var (
	PatchRequesters = []string{
		PatchVersionRequester,
		GithubPRRequester,
		GithubMergeRequester,
	}

	SystemActivators = []string{
		APIServerTaskActivator,
		BuildActivator,
		CheckBlockedTasksActivator,
		ElapsedBuildActivator,
		ElapsedTaskActivator,
		GenerateTasksActivator,
	}

	// UpHostStatus is a list of all host statuses that are considered up.
	UpHostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostBuilding,
		HostStarting,
		HostProvisioning,
		HostProvisionFailed,
		HostStopping,
		HostStopped,
	}

	// StoppableHostStatuses represent all host statuses when it is possible to
	// stop a running host.
	StoppableHostStatuses = []string{
		// If the host is already stopped, stopping it again is a no-op but not
		// an error. It will remain stopped.
		HostStopped,
		// If the host is stopping but somehow gets stuck stopping (e.g. a
		// timeout waiting for it to stop), this is an intermediate state, so it
		// is valid to try stopping it again.
		HostStopping,
		// If the host is running, it can be stopped.
		HostRunning,
	}

	// StoppableHostStatuses represent all host statuses when it is possible to
	// start a stopped host.
	StartableHostStatuses = []string{
		// If the host is stopped, it can be started back up.
		HostStopped,
		// If the host is stopping but somehow gets stuck stopping (e.g. a
		// timeout waiting for it to stop), this is an intermediate state, so it
		// is valid to start it back up.
		HostStopping,
		// If the host is already running, starting it again is a no-op but not
		// an error. It will remain running.
		HostRunning,
	}

	StartedHostStatus = []string{
		HostBuilding,
		HostStarting,
	}

	// ProvisioningHostStatus describes hosts that have started,
	// but have not yet completed the provisioning process.
	ProvisioningHostStatus = []string{
		HostStarting,
		HostProvisioning,
		HostProvisionFailed,
		HostBuilding,
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

	// IsRunningOrWillRunStatuses includes all statuses for active hosts (see
	// ActiveStatus) where the host is either currently running or is making
	// progress towards running.
	IsRunningOrWillRunStatuses = []string{
		HostBuilding,
		HostStarting,
		HostProvisioning,
		HostRunning,
	}

	// ActiveStatuses includes all where the host is alive in the cloud provider
	// or could be alive (e.g. for building hosts, the host is in the process of
	// starting up). Intent hosts have not requested a real host from the cloud
	// provider, so they are omitted.
	ActiveStatuses = []string{
		HostBuilding,
		HostBuildingFailed,
		HostStarting,
		HostProvisioning,
		HostProvisionFailed,
		HostRunning,
		HostStopping,
		HostStopped,
		HostDecommissioned,
		HostQuarantined,
	}

	// SleepScheduleStatuses are all host statuses for which the sleep schedule
	// can take effect. If it's not in one of these states, the sleep schedule
	// does not apply.
	SleepScheduleStatuses = []string{
		HostRunning,
		HostStopped,
		HostStopping,
	}

	// Set of host status values that can be user set via the API
	ValidUserSetHostStatus = []string{
		HostRunning,
		HostTerminated,
		HostQuarantined,
		HostDecommissioned,
	}

	// Set of valid PlannerSettings.Version strings that can be user set via the API
	ValidTaskPlannerVersions = []string{
		PlannerVersionTunable,
	}

	// Set of valid DispatchSettings.Version strings that can be user set via the API
	ValidTaskDispatcherVersions = []string{
		DispatcherVersionRevisedWithDependencies,
	}

	// Set of valid FinderSettings.Version strings that can be user set via the API
	ValidTaskFinderVersions = []string{
		FinderVersionLegacy,
		FinderVersionParallel,
		FinderVersionPipeline,
		FinderVersionAlternate,
	}

	// Set of valid Host Allocators types
	ValidHostAllocators = []string{
		HostAllocatorUtilization,
	}

	ValidHostAllocatorRoundingRules = []string{
		HostAllocatorRoundDown,
		HostAllocatorRoundUp,
		HostAllocatorRoundDefault,
	}

	ValidDefaultHostAllocatorRoundingRules = []string{
		HostAllocatorRoundDown,
		HostAllocatorRoundUp,
	}
	ValidHostAllocatorFeedbackRules = []string{
		HostAllocatorWaitsOverThreshFeedback,
		HostAllocatorNoFeedback,
		HostAllocatorUseDefaultFeedback,
	}

	ValidDefaultHostAllocatorFeedbackRules = []string{
		HostAllocatorWaitsOverThreshFeedback,
		HostAllocatorNoFeedback,
	}

	ValidHostsOverallocatedRules = []string{
		HostsOverallocatedUseDefault,
		HostsOverallocatedIgnore,
		HostsOverallocatedTerminate,
	}

	ValidDefaultHostsOverallocatedRules = []string{
		HostsOverallocatedIgnore,
		HostsOverallocatedTerminate,
	}

	// TaskInProgressStatuses have been picked up by an agent but have not
	// finished running.
	TaskInProgressStatuses = []string{TaskStarted, TaskDispatched}
	// TaskCompletedStatuses are statuses for tasks that have finished running.
	// This does not include task display statuses.
	TaskCompletedStatuses = []string{TaskSucceeded, TaskFailed}
	// TaskUncompletedStatuses are all statuses that do not represent a finished state.
	TaskUncompletedStatuses = []string{
		TaskStarted,
		TaskUndispatched,
		TaskDispatched,
		TaskInactive,
	}

	SyncStatuses = []string{TaskSucceeded, TaskFailed}

	ValidCommandTypes = []string{CommandTypeSetup, CommandTypeSystem, CommandTypeTest}

	// Map from valid OS/architecture combinations to display names
	ValidArchDisplayNames = map[string]string{
		ArchWindowsAmd64: "Windows 64-bit",
		ArchLinuxPpc64le: "Linux PowerPC 64-bit",
		ArchLinuxS390x:   "Linux zSeries",
		ArchLinuxArm64:   "Linux ARM 64-bit",
		ArchDarwinAmd64:  "OSX 64-bit",
		ArchDarwinArm64:  "OSX ARM 64-bit",
		ArchLinuxAmd64:   "Linux 64-bit",
	}
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
	return utility.StringSliceContains(SystemActivators, caller)
}

func IsPatchRequester(requester string) bool {
	return requester == PatchVersionRequester || IsGitHubPatchRequester(requester)
}

func IsGitHubPatchRequester(requester string) bool {
	return requester == GithubPRRequester || requester == GithubMergeRequester
}

func IsGithubPRRequester(requester string) bool {
	return requester == GithubPRRequester
}

func IsGitTagRequester(requester string) bool {
	return requester == GitTagRequester
}

func IsGithubMergeQueueRequester(requester string) bool {
	return requester == GithubMergeRequester
}

// ShouldConsiderBatchtime checks if the requester is for a mainline commit,
// as this is the only requester checked for project activation.
func ShouldConsiderBatchtime(requester string) bool {
	return requester == RepotrackerVersionRequester
}

func PermissionsDisabledForTests() bool {
	return PermissionSystemDisabled
}

// Constants for permission scopes and resource types.
const (
	SuperUserResourceType = "super_user"
	ProjectResourceType   = "project"
	DistroResourceType    = "distro"

	AllProjectsScope          = "all_projects"
	UnrestrictedProjectsScope = "unrestricted_projects"
	RestrictedProjectsScope   = "restricted_projects"
	AllDistrosScope           = "all_distros"
)

type PermissionLevel struct {
	Description string `json:"description"`
	Value       int    `json:"value"`
}

var (
	ValidResourceTypes = []string{SuperUserResourceType, ProjectResourceType, DistroResourceType}
	// SuperUserPermissions resource ID.
	SuperUserPermissionsID = "super_user"

	// Admin permissions.
	PermissionAdminSettings = "admin_settings"
	PermissionProjectCreate = "project_create"
	PermissionDistroCreate  = "distro_create"
	PermissionRoleModify    = "modify_roles"
	// Project permissions.
	PermissionProjectSettings = "project_settings"

	PermissionGitTagVersions = "project_git_tags"
	PermissionTasks          = "project_tasks"
	PermissionAnnotations    = "project_task_annotations"
	PermissionPatches        = "project_patches"
	PermissionLogs           = "project_logs"
	// Distro permissions.
	PermissionDistroSettings = "distro_settings"
	PermissionHosts          = "distro_hosts"
)

// Constants related to permission levels.
var (
	AdminSettingsEdit = PermissionLevel{
		Description: "Edit admin settings",
		Value:       10,
	}
	ProjectCreate = PermissionLevel{
		Description: "Create new projects",
		Value:       10,
	}
	DistroCreate = PermissionLevel{
		Description: "Create new distros",
		Value:       10,
	}
	RoleModify = PermissionLevel{
		Description: "Modify system roles and permissions",
		Value:       10,
	}
	ProjectSettingsEdit = PermissionLevel{
		Description: "Edit project settings",
		Value:       20,
	}
	ProjectSettingsView = PermissionLevel{
		Description: "View project settings",
		Value:       10,
	}
	ProjectSettingsNone = PermissionLevel{
		Description: "No project settings permissions",
		Value:       0,
	}
	GitTagVersionsCreate = PermissionLevel{
		Description: "Create versions with git tags",
		Value:       10,
	}
	GitTagVersionsNone = PermissionLevel{
		Description: "Not able to create versions with git tags",
		Value:       0,
	}
	AnnotationsModify = PermissionLevel{
		Description: "Modify annotations",
		Value:       20,
	}
	AnnotationsView = PermissionLevel{
		Description: "View annotations",
		Value:       10,
	}
	AnnotationsNone = PermissionLevel{
		Description: "No annotations permissions",
		Value:       0,
	}
	TasksAdmin = PermissionLevel{
		Description: "Edit tasks and override dependencies",
		Value:       30,
	}
	TasksBasic = PermissionLevel{
		Description: "Edit tasks",
		Value:       20,
	}
	TasksView = PermissionLevel{
		Description: "View tasks",
		Value:       10,
	}
	TasksNone = PermissionLevel{
		Description: "Not able to view or edit tasks",
		Value:       0,
	}
	PatchSubmitAdmin = PermissionLevel{
		Description: "Submit/edit patches, and submit patches on behalf of users",
		Value:       20,
	}
	PatchSubmit = PermissionLevel{
		Description: "Submit and edit patches",
		Value:       10,
	}
	PatchNone = PermissionLevel{
		Description: "Not able to submit patches",
		Value:       0,
	}
	LogsView = PermissionLevel{
		Description: "View logs",
		Value:       10,
	}
	LogsNone = PermissionLevel{
		Description: "Not able to view logs",
		Value:       0,
	}
	DistroSettingsAdmin = PermissionLevel{
		Description: "Remove distro and edit distro settings",
		Value:       30,
	}
	DistroSettingsEdit = PermissionLevel{
		Description: "Edit distro settings",
		Value:       20,
	}
	DistroSettingsView = PermissionLevel{
		Description: "View distro settings",
		Value:       10,
	}
	DistroSettingsNone = PermissionLevel{
		Description: "No distro settings permissions",
		Value:       0,
	}
	HostsEdit = PermissionLevel{
		Description: "Edit hosts",
		Value:       20,
	}
	HostsView = PermissionLevel{
		Description: "View hosts",
		Value:       10,
	}
	HostsNone = PermissionLevel{
		Description: "No hosts permissions",
		Value:       0,
	}
)

// GetDisplayNameForPermissionKey gets the display name associated with a permission key
func GetDisplayNameForPermissionKey(permissionKey string) string {
	switch permissionKey {
	case PermissionProjectSettings:
		return "Project Settings"
	case PermissionTasks:
		return "Tasks"
	case PermissionAnnotations:
		return "Task Annotations"
	case PermissionPatches:
		return "Patches"
	case PermissionGitTagVersions:
		return "Git Tag Versions"
	case PermissionLogs:
		return "Logs"
	case PermissionDistroSettings:
		return "Distro Settings"
	case PermissionHosts:
		return "Distro Hosts"
	default:
		return ""
	}
}

// GetPermissionLevelsForPermissionKey gets all permissions associated with a permission key
func GetPermissionLevelsForPermissionKey(permissionKey string) []PermissionLevel {
	switch permissionKey {
	case PermissionProjectSettings:
		return []PermissionLevel{
			ProjectSettingsEdit,
			ProjectSettingsView,
			ProjectSettingsNone,
		}
	case PermissionTasks:
		return []PermissionLevel{
			TasksAdmin,
			TasksBasic,
			TasksView,
			TasksNone,
		}
	case PermissionAnnotations:
		return []PermissionLevel{
			AnnotationsModify,
			AnnotationsView,
			AnnotationsNone,
		}
	case PermissionPatches:
		return []PermissionLevel{
			PatchSubmit,
			PatchNone,
		}
	case PermissionGitTagVersions:
		return []PermissionLevel{
			GitTagVersionsCreate,
			GitTagVersionsNone,
		}
	case PermissionLogs:
		return []PermissionLevel{
			LogsView,
			LogsNone,
		}
	case PermissionDistroSettings:
		return []PermissionLevel{
			DistroSettingsAdmin,
			DistroSettingsEdit,
			DistroSettingsView,
			DistroSettingsNone,
		}
	case PermissionHosts:
		return []PermissionLevel{
			HostsEdit,
			HostsView,
			HostsNone,
		}
	default:
		return []PermissionLevel{}
	}
}

// If adding a new type of permissions, i.e. a new array of permission keys, then you must:
// 1. Add a new field in the APIPermissions model for those permissions.
// 2. Populate the value of that APIPermissions field with the getPermissions function in rest/route/permissions.go

var ProjectPermissions = []string{
	PermissionProjectSettings,
	PermissionTasks,
	PermissionAnnotations,
	PermissionPatches,
	PermissionGitTagVersions,
	PermissionLogs,
}

var DistroPermissions = []string{
	PermissionDistroSettings,
	PermissionHosts,
}

var SuperuserPermissions = []string{
	PermissionAdminSettings,
	PermissionProjectCreate,
	PermissionDistroCreate,
	PermissionRoleModify,
}

const (
	BasicProjectAccessRole     = "basic_project_access"
	BasicDistroAccessRole      = "basic_distro_access"
	SuperUserRole              = "superuser"
	SuperUserProjectAccessRole = "admin_project_access"
	SuperUserDistroAccessRole  = "superuser_distro_access"
)

// Contains both general and superuser access.
var GeneralAccessRoles = []string{
	BasicProjectAccessRole,
	BasicDistroAccessRole,
	SuperUserRole,
	SuperUserProjectAccessRole,
	SuperUserDistroAccessRole,
}

// LogViewer represents recognized viewers for rendering logs.
type LogViewer string

const (
	LogViewerRaw     LogViewer = "raw"
	LogViewerHTML    LogViewer = "html"
	LogViewerParsley LogViewer = "parsley"
)

// ContainerOS denotes the operating system of a running container.
type ContainerOS string

const (
	LinuxOS   ContainerOS = "linux"
	WindowsOS ContainerOS = "windows"
)

// ValidContainerOperatingSystems contains all recognized container operating
// systems.
var ValidContainerOperatingSystems = []ContainerOS{LinuxOS, WindowsOS}

// Validate checks that the container OS is recognized.
func (c ContainerOS) Validate() error {
	switch c {
	case LinuxOS, WindowsOS:
		return nil
	default:
		return errors.Errorf("unrecognized container OS '%s'", c)
	}
}

// ContainerArch represents the CPU architecture necessary to run a container.
type ContainerArch string

const (
	ArchARM64 ContainerArch = "arm64"
	ArchAMD64 ContainerArch = "x86_64"
)

// ValidContainerArchitectures contains all recognized container CPU
// architectures.
var ValidContainerArchitectures = []ContainerArch{ArchARM64, ArchAMD64}

// Validate checks that the container CPU architecture is recognized.
func (c ContainerArch) Validate() error {
	switch c {
	case ArchARM64, ArchAMD64:
		return nil
	default:
		return errors.Errorf("unrecognized CPU architecture '%s'", c)
	}
}

// WindowsVersion specifies the compatibility version of Windows that is required for the container to run.
type WindowsVersion string

const (
	Windows2022 WindowsVersion = "2022"
	Windows2019 WindowsVersion = "2019"
	Windows2016 WindowsVersion = "2016"
)

// ValidWindowsVersions contains all recognized container Windows versions.
var ValidWindowsVersions = []WindowsVersion{Windows2016, Windows2019, Windows2022}

// Validate checks that the container Windows version is recognized.
func (w WindowsVersion) Validate() error {
	switch w {
	case Windows2022, Windows2019, Windows2016:
		return nil
	default:
		return errors.Errorf("unrecognized Windows version '%s'", w)
	}
}

// ParserProjectStorageMethod represents a means to store the parser project.
type ParserProjectStorageMethod string

const (
	// ProjectStorageMethodDB indicates that the parser project is stored as a
	// single document in a DB collection.
	ProjectStorageMethodDB ParserProjectStorageMethod = "db"
	// ProjectStorageMethodS3 indicates that the parser project is stored as a
	// single object in S3.
	ProjectStorageMethodS3 ParserProjectStorageMethod = "s3"
)

const (
	// Valid public key types.
	publicKeyRSA     = "ssh-rsa"
	publicKeyDSS     = "ssh-dss"
	publicKeyED25519 = "ssh-ed25519"
	publicKeyECDSA   = "ecdsa-sha2-nistp256"
)

var validKeyTypes = []string{
	publicKeyRSA,
	publicKeyDSS,
	publicKeyED25519,
	publicKeyECDSA,
}

var sensitiveCollections = []string{"events"}

// ValidateSSHKey errors if the given key does not start with one of the allowed prefixes.
func ValidateSSHKey(key string) error {
	for _, prefix := range validKeyTypes {
		if strings.HasPrefix(key, prefix) {
			return nil
		}
	}
	return errors.Errorf("either an invalid Evergreen-managed key name has been provided, "+
		"or the key value is not one of the valid types: %s", validKeyTypes)
}
