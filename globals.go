package evergreen

import (
	"os"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	User            = "mci"
	GithubPatchUser = "github_pull_request"
	ParentPatchUser = "parent_patch"

	HostRunning         = "running"
	HostTerminated      = "terminated"
	HostUninitialized   = "initializing"
	HostBuilding        = "building"
	HostStarting        = "starting"
	HostProvisioning    = "provisioning"
	HostProvisionFailed = "provision failed"
	HostQuarantined     = "quarantined"
	HostDecommissioned  = "decommissioned"

	HostStopping = "stopping"
	HostStopped  = "stopped"

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
	TaskUnscheduled  = "unscheduled"
	// TaskWillRun is a subset of TaskUndispatched and is only used in the UI
	TaskWillRun = "will-run"

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

	// This is not an official task status; however it is used by the front end to distinguish aborted and failing tasks
	// Tasks can be filtered on the front end by `aborted` status
	TaskAborted = "aborted"

	TaskStatusBlocked = "blocked"
	TaskStatusPending = "pending"

	// This is not an official task status; it is used by the front end to indicate that there is a linked issue in the annotation
	TaskKnownIssue = "known-issue"

	// Task Command Types
	CommandTypeTest   = "test"
	CommandTypeSystem = "system"
	CommandTypeSetup  = "setup"

	// Task descriptions
	TaskDescriptionHeartbeat = "heartbeat"
	TaskDescriptionStranded  = "stranded"
	TaskDescriptionNoResults = "expected test results, but none attached"

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

	PatchCreated     = "created"
	PatchStarted     = "started"
	PatchSucceeded   = "succeeded"
	PatchFailed      = "failed"
	PatchAborted     = "aborted" // This is a display status only and not a real patch status
	PatchAllOutcomes = "*"

	PushLogPushing = "pushing"
	PushLogSuccess = "success"

	HostTypeStatic = "static"

	MergeTestStarted   = "started"
	MergeTestSucceeded = "succeeded"
	MergeTestFailed    = "failed"
	EnqueueFailed      = "failed to enqueue"

	// maximum task (zero based) execution number
	MaxTaskExecution = 9

	// maximum task priority
	MaxTaskPriority = 100

	DisabledTaskPriority = int64(-1)

	// LogMessage struct versions
	LogmessageFormatTimestamp = 1
	LogmessageCurrentVersion  = LogmessageFormatTimestamp

	DefaultEvergreenConfig = ".evergreen.yml"

	EvergreenHome   = "EVGHOME"
	MongodbUrl      = "MONGO_URL"
	MongodbAuthFile = "MONGO_CREDS_FILE"

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
	HostFetchTag    = "host-fetch"

	DegradedLoggingPercent = 10

	SetupScriptName               = "setup.sh"
	TempSetupScriptName           = "setup-temp.sh"
	PowerShellSetupScriptName     = "setup.ps1"
	PowerShellTempSetupScriptName = "setup-temp.ps1"

	RoutePaginatorNextPageHeaderKey = "Link"

	PlannerVersionLegacy  = "legacy"
	PlannerVersionTunable = "tunable"

	DispatcherVersionLegacy                  = "legacy"
	DispatcherVersionRevised                 = "revised"
	DispatcherVersionRevisedWithDependencies = "revised-with-dependencies"

	// maximum turnaround we want to maintain for all hosts for a given distro
	MaxDurationPerDistroHost               = 30 * time.Minute
	MaxDurationPerDistroHostWithContainers = 2 * time.Minute

	// Spawn hosts
	SpawnHostExpireDays                 = 30
	HostExpireDays                      = 10
	ExpireOnFormat                      = "2006-01-02"
	DefaultMaxSpawnHostsPerUser         = 3
	DefaultSpawnHostExpiration          = 24 * time.Hour
	SpawnHostNoExpirationDuration       = 7 * 24 * time.Hour
	MaxSpawnHostExpirationDurationHours = 24 * time.Hour * 14
	UnattachedVolumeExpiration          = 24 * time.Hour * 30
	DefaultMaxVolumeSizePerUser         = 500
	DefaultUnexpirableHostsPerUser      = 1
	DefaultUnexpirableVolumesPerUser    = 1

	// host resource tag names
	TagName             = "name"
	TagDistro           = "distro"
	TagEvergreenService = "evergreen-service"
	TagUsername         = "username"
	TagOwner            = "owner"
	TagMode             = "mode"
	TagStartTime        = "start-time"
	TagExpireOn         = "expire-on"

	FinderVersionLegacy    = "legacy"
	FinderVersionParallel  = "parallel"
	FinderVersionPipeline  = "pipeline"
	FinderVersionAlternate = "alternate"

	HostAllocatorDeficit     = "deficit"
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

	// CommitQueueAlias and GithubPRAlias are special aliases to specify variants and tasks for commit queue and GitHub PR patches
	CommitQueueAlias  = "__commit_queue"
	GithubPRAlias     = "__github"
	GithubChecksAlias = "__github_checks"
	GitTagAlias       = "__git_tag"

	MergeTaskVariant = "commit-queue-merge"
	MergeTaskName    = "merge-patch"
	MergeTaskGroup   = "merge-task-group"

	DefaultJasperPort = 2385

	GlobalGitHubTokenExpansion = "global_github_oauth_token"

	VSCodePort = 2021

	// DefaultTaskSyncAtEndTimeout is the default timeout for task sync at the
	// end of a patch.
	DefaultTaskSyncAtEndTimeout = time.Hour

	DefaultShutdownWaitSeconds = 10

	SaveGenerateTasksError     = "error saving config in `generate.tasks`"
	TasksAlreadyGeneratedError = "generator already ran and generated tasks"
)

var InternalAliases []string = []string{
	CommitQueueAlias,
	GithubPRAlias,
	GithubChecksAlias,
	GitTagAlias,
}

var TaskFailureStatuses []string = []string{
	TaskTimedOut,
	TaskFailed,
	TaskSystemFailed,
	TaskTestTimedOut,
	TaskSetupFailed,
	TaskSystemUnresponse,
	TaskSystemTimedOut,
}

var TaskUnstartedStatuses []string = []string{
	TaskInactive,
	TaskUnstarted,
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

func IsFinishedPatchStatus(status string) bool {
	return status == PatchFailed || status == PatchSucceeded
}

func IsFinishedBuildStatus(status string) bool {
	return status == BuildFailed || status == BuildSucceeded
}

func IsFinishedVersionStatus(status string) bool {
	return status == VersionFailed || status == VersionSucceeded
}

func VersionStatusToPatchStatus(versionStatus string) (string, error) {
	switch versionStatus {
	case VersionCreated:
		return PatchCreated, nil
	case VersionStarted:
		return PatchStarted, nil
	case VersionFailed:
		return PatchFailed, nil
	case VersionSucceeded:
		return PatchSucceeded, nil
	default:
		return "", errors.Errorf("unknown version status: %s", versionStatus)
	}
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
	PodHeader           = "Pod-Id"
	PodSecretHeader     = "Pod-Secret"
	ContentTypeHeader   = "Content-Type"
	ContentTypeValue    = "application/json"
	ContentLengthHeader = "Content-Length"
	APIUserHeader       = "Api-User"
	APIKeyHeader        = "Api-Key"
)

const (
	// CredentialsCollection is the collection containing TLS credentials to
	// connect to a Jasper service running on a host.
	CredentialsCollection = "credentials"
	// CAName is the name of the root CA for the TLS credentials.
	CAName = "evergreen"
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
	// This is Amazon's EBS type default
	DefaultEBSType = "gp2"
	// This may be a temporary default
	DefaultEBSAvailabilityZone = "us-east-1a"
)

var (
	// Providers where hosts can be created and terminated automatically.
	ProviderSpawnable = []string{
		ProviderNameEc2OnDemand,
		ProviderNameEc2Spot,
		ProviderNameEc2Auto,
		ProviderNameEc2Fleet,
		ProviderNameGce,
		ProviderNameOpenstack,
		ProviderNameVsphere,
		ProviderNameMock,
		ProviderNameDocker,
	}

	// Providers that are spawnable by users
	ProviderUserSpawnable = []string{
		ProviderNameEc2OnDemand,
		ProviderNameEc2Spot,
		ProviderNameEc2Auto,
		ProviderNameEc2Fleet,
		ProviderNameGce,
		ProviderNameOpenstack,
		ProviderNameVsphere,
	}

	ProviderContainer = []string{
		ProviderNameDocker,
	}

	SystemVersionRequesterTypes = []string{
		RepotrackerVersionRequester,
		TriggerRequester,
		GitTagRequester,
	}

	ProviderSpotEc2Type = []string{
		ProviderNameEc2Auto,
		ProviderNameEc2Spot,
		ProviderNameEc2Fleet,
	}
)

const (
	DefaultServiceConfigurationFileName = "/etc/mci_settings.yml"
	DefaultDatabaseUrl                  = "mongodb://localhost:27017"
	DefaultDatabaseName                 = "mci"
	DefaultDatabaseWriteMode            = "majority"
	DefaultDatabaseReadMode             = "majority"

	// database and config directory, set to the testing version by default for safety
	NotificationsFile = "mci-notifications.yml"
	ClientDirectory   = "clients"

	// version requester types
	PatchVersionRequester       = "patch_request"
	GithubPRRequester           = "github_pull_request"
	GitTagRequester             = "git_tag_request"
	RepotrackerVersionRequester = "gitter_request"
	TriggerRequester            = "trigger_request"
	MergeTestRequester          = "merge_test"
	AdHocRequester              = "ad_hoc"
)

const (
	GenerateTasksCommandName = "generate.tasks"
	HostCreateCommandName    = "host.create"
	S3PushCommandName        = "s3.push"
	S3PullCommandName        = "s3.pull"
)

type SenderKey int

const (
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

// Recognized architectures, should be in the form ${GOOS}_${GOARCH}.
const (
	ArchDarwinAmd64  = "darwin_amd64"
	ArchDarwinArm64  = "darwin_arm64"
	ArchLinux386     = "linux_386"
	ArchLinuxPpc64le = "linux_ppc64le"
	ArchLinuxS390x   = "linux_s390x"
	ArchLinuxArm64   = "linux_arm64"
	ArchLinuxAmd64   = "linux_amd64"
	ArchWindows386   = "windows_386"
	ArchWindowsAmd64 = "windows_amd64"
)

// NameTimeFormat is the format in which to log times like instance start time.
const NameTimeFormat = "20060102150405"

var (
	PatchRequesters = []string{
		PatchVersionRequester,
		GithubPRRequester,
		MergeTestRequester,
	}

	SystemActivators = []string{
		DefaultTaskActivator,
		APIServerTaskActivator,
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

	// Hosts in "initializing" status aren't actually running yet:
	// they're just intents, so this list omits that value.
	ActiveStatus = []string{
		HostRunning,
		HostBuilding,
		HostStarting,
		HostProvisioning,
		HostProvisionFailed,
		HostStopping,
		HostStopped,
	}

	// Set of host status values that can be user set via the API
	ValidUserSetStatus = []string{
		HostRunning,
		HostTerminated,
		HostQuarantined,
		HostDecommissioned,
	}

	// Set of valid PlannerSettings.Version strings that can be user set via the API
	ValidTaskPlannerVersions = []string{
		PlannerVersionLegacy,
		PlannerVersionTunable,
	}

	// Set of valid DispatchSettings.Version strings that can be user set via the API
	ValidTaskDispatcherVersions = []string{
		DispatcherVersionLegacy,
		DispatcherVersionRevised,
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

	// constant arrays for db update logic
	AbortableStatuses   = []string{TaskStarted, TaskDispatched}
	CompletedStatuses   = []string{TaskSucceeded, TaskFailed}
	UncompletedStatuses = []string{TaskStarted, TaskUnstarted, TaskUndispatched, TaskDispatched, TaskConflict}

	SyncStatuses = []string{TaskSucceeded, TaskFailed}

	ValidCommandTypes = []string{CommandTypeSetup, CommandTypeSystem, CommandTypeTest}

	// Map from valid architectures to display names
	ValidArchDisplayNames = map[string]string{
		ArchWindowsAmd64: "Windows 64-bit",
		ArchLinuxPpc64le: "Linux PowerPC 64-bit",
		ArchLinuxS390x:   "Linux zSeries",
		ArchLinuxArm64:   "Linux ARM 64-bit",
		ArchWindows386:   "Windows 32-bit",
		ArchDarwinAmd64:  "OSX 64-bit",
		ArchDarwinArm64:  "OSX ARM 64-bit",
		ArchLinuxAmd64:   "Linux 64-bit",
		ArchLinux386:     "Linux 32-bit",
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
	return requester == GithubPRRequester || requester == MergeTestRequester
}

func IsGitTagRequester(requester string) bool {
	return requester == GitTagRequester
}

func ShouldConsiderBatchtime(requester string) bool {
	return !IsPatchRequester(requester) && requester != AdHocRequester && requester != GitTagRequester
}

// Permissions-related constants
func PermissionsDisabledForTests() bool {
	return PermissionSystemDisabled
}

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
	UnauthedUserRoles  = []string{"unauthorized_project"}
	ValidResourceTypes = []string{SuperUserResourceType, ProjectResourceType, DistroResourceType}
	// SuperUserPermissions resource ID.
	SuperUserPermissionsID = "super_user"

	// Admin permissions.
	PermissionAdminSettings = "admin_settings"
	PermissionProjectCreate = "project_create"
	PermissionDistroCreate  = "distro_create"
	PermissionRoleModify    = "modify_roles"
	// Project permissions.
	PermissionProjectSettings  = "project_settings"
	PermissionProjectVariables = "project_variables"
	PermissionGitTagVersions   = "project_git_tags"
	PermissionTasks            = "project_tasks"
	PermissionAnnotations      = "project_task_annotations"
	PermissionPatches          = "project_patches"
	PermissionLogs             = "project_logs"
	// Distro permissions.
	PermissionDistroSettings = "distro_settings"
	PermissionHosts          = "distro_hosts"
)

// permission levels
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
		Description: "Full tasks permissions",
		Value:       30,
	}
	TasksBasic = PermissionLevel{
		Description: "Basic modifications to tasks",
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
	PatchSubmit = PermissionLevel{
		Description: "Submit and edit patches",
		Value:       10,
	}
	PatchNone = PermissionLevel{
		Description: "Not able to view or submit patches",
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
	BasicProjectAccessRole = "basic_project_access"
	BasicDistroAccessRole  = "basic_distro_access"
)

var BasicAccessRoles = []string{
	BasicProjectAccessRole,
	BasicDistroAccessRole,
}

// Evergreen log types.
const (
	LogTypeAgent  = "agent_log"
	LogTypeTask   = "task_log"
	LogTypeSystem = "system_log"
)

type ECSClusterPlatform string

const (
	ECSClusterPlatformLinux   = "linux"
	ECSClusterPlatformWindows = "windows"
)

func (p ECSClusterPlatform) Validate() error {
	switch p {
	case ECSClusterPlatformLinux, ECSClusterPlatformWindows:
		return nil
	default:
		return errors.Errorf("unrecognized ECS cluster platform '%s'", p)
	}
}

// LogViewer represents recognized viewers for rendering logs.
type LogViewer string

const (
	LogViewerRaw     LogViewer = "raw"
	LogViewerHTML    LogViewer = "html"
	LogViewerLobster LogViewer = "lobster"
)
