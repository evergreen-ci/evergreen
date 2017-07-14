package evergreen

import (
	"fmt"
	"os"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/slogger"
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

	EvergreenHome = "EVGHOME"

	DefaultTaskActivator   = ""
	StepbackTaskActivator  = "stepback"
	APIServerTaskActivator = "apiserver"

	RestRoutePrefix = "rest"
	APIRoutePrefix  = "api"

	AgentAPIVersion = 2
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

// HTTP constants. Added after Go1.4. Here for compatibility with GCCGO
// compatibility. Copied from: https://golang.org/pkg/net/http/#pkg-constants
const (
	MethodGet     = "GET"
	MethodHead    = "HEAD"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodPatch   = "PATCH" // RFC 5789
	MethodDelete  = "DELETE"
	MethodConnect = "CONNECT"
	MethodOptions = "OPTIONS"
	MethodTrace   = "TRACE"
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
)

var (
	// UphostStatus is a list of all host statuses that are considered "up."
	// This is used for query building.
	UphostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostStarting,
		HostInitializing,
		HostProvisionFailed,
	}

	// Logger is our global logger. It can be changed for testing.
	Logger slogger.Logger

	// database and config directory, set to the testing version by default for safety
	NotificationsFile = "mci-notifications.yml"
	ClientDirectory   = "clients"

	// version requester types
	PatchVersionRequester       = "patch_request"
	RepotrackerVersionRequester = "gitter_request"

	// constant arrays for db update logic
	AbortableStatuses = []string{TaskStarted, TaskDispatched}
	CompletedStatuses = []string{TaskSucceeded, TaskFailed}
)

// SetLegacyLogger sets the global (s)logger instance to wrap the current grip Logger.
func SetLegacyLogger() {
	Logger = slogger.Logger{
		Name:      fmt.Sprintf("evg-log.%s", grip.Name()),
		Appenders: []send.Sender{grip.GetSender()},
	}
}

// FindEvergreenHome finds the directory of the EVGHOME environment variable.
func FindEvergreenHome() string {
	// check if env var is set
	root := os.Getenv(EvergreenHome)
	if len(root) > 0 {
		return root
	}

	Logger.Logf(slogger.ERROR, "%s is unset", EvergreenHome)
	return ""
}

// IsSystemActivator returns true when the task activator is Evergreen.
func IsSystemActivator(caller string) bool {
	return caller == DefaultTaskActivator ||
		caller == APIServerTaskActivator
}
