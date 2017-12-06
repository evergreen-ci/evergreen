package evergreen

import (
	"os"
	"time"

	"github.com/mongodb/grip"
)

type (
	// custom type used to attach specific values to request contexts, to prevent collisions.
	requestUserContextKey int
)

const (
	User = "mci"

	HostRunning         = "running"
	HostTerminated      = "terminated"
	HostUninitialized   = "initializing"
	HostStarting        = "starting"
	HostInitializing    = "provisioning"
	HostProvisionFailed = "provision failed"
	HostUnreachable     = "unreachable"
	HostQuarantined     = "quarantined"
	HostDecommissioned  = "decommissioned"

	HostStatusSuccess = "success"
	HostStatusFailed  = "failed"

	TaskStarted          = "started"
	TaskUnstarted        = "unstarted"
	TaskUndispatched     = "undispatched"
	TaskDispatched       = "dispatched"
	TaskFailed           = "failed"
	TaskSucceeded        = "success"
	TaskInactive         = "inactive"
	TaskSystemFailed     = "system-failed"
	TaskTimedOut         = "task-timed-out"
	TaskSystemUnresponse = "system-unresponsive"
	TaskSystemTimedOut   = "system-timed-out"
	TaskTestTimedOut     = "test-timed-out"
	TaskConflict         = "task-conflict"

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

	CompileStage = "compile"
	PushStage    = "push"

	// maximum task (zero based) execution number
	MaxTaskExecution = 3

	// maximum task priority
	MaxTaskPriority = 100

	// LogMessage struct versions
	LogmessageFormatTimestamp = 1
	LogmessageCurrentVersion  = LogmessageFormatTimestamp

	EvergreenHome        = "EVGHOME"
	LocalLoggingOverride = "LOCAL"

	DefaultTaskActivator   = ""
	StepbackTaskActivator  = "stepback"
	APIServerTaskActivator = "apiserver"

	RestRoutePrefix = "rest"
	APIRoutePrefix  = "api"

	AgentAPIVersion = 2

	DegradedLoggingPercent = 10

	// Key used to store user information in request contexts
	RequestUser requestUserContextKey = 0

	SetupScriptName    = "setup.sh"
	TeardownScriptName = "teardown.sh"
)

// evergreen package names
const (
	UIPackage     = "EVERGREEN_UI"
	RESTV2Package = "EVERGREEN_REST_V2"
)

const (
	AuthTokenCookie   = "mci-token"
	TaskSecretHeader  = "Task-Secret"
	HostHeader        = "Host-Id"
	HostSecretHeader  = "Host-Secret"
	ContentTypeHeader = "Content-Type"
	ContentTypeValue  = "application/json"
	APIUserHeader     = "Api-User"
	APIKeyHeader      = "Api-Key"
)

// cloud provider related constants
const (
	ProviderNameEc2OnDemand  = "ec2"
	ProviderNameEc2Spot      = "ec2-spot"
	ProviderNameDigitalOcean = "digitalocean"
	ProviderNameDocker       = "docker"
	ProviderNameGce          = "gce"
	ProviderNameStatic       = "static"
	ProviderNameOpenstack    = "openstack"
	ProviderNameVsphere      = "vsphere"
)

const (
	// database and config directory, set to the testing version by default for safety
	NotificationsFile = "mci-notifications.yml"
	ClientDirectory   = "clients"

	// version requester types
	PatchVersionRequester       = "patch_request"
	RepotrackerVersionRequester = "gitter_request"
)

const (
	defaultLogBufferingDuration  = 20
	defaultMgoDialTimeout        = 5 * time.Second
	defaultAmboyPoolSize         = 2
	defaultAmboyLocalStorageSize = 1024
	defaultAmboyQueueName        = "evg.service"
	defaultAmboyDBName           = "amboy"
)

var (
	// UphostStatus is a list of all hostb statuses that are considered "up."
	// This is used for query building.
	UphostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostStarting,
		HostInitializing,
		HostProvisionFailed,
	}

	// constant arrays for db update logic
	AbortableStatuses = []string{TaskStarted, TaskDispatched}
	CompletedStatuses = []string{TaskSucceeded, TaskFailed}
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
