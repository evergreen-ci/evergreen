package evergreen

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/util"
	"os"
	"path/filepath"
	"strings"
)

const (
	User = "mci"

	TestDir      = "config_test"
	TestSettings = "mci_settings.yml"

	HostRunning         = "running"
	HostTerminated      = "terminated"
	HostUninitialized   = "starting"
	HostInitializing    = "provisioning"
	HostProvisionFailed = "provision failed"
	HostUnreachable     = "unreachable"
	HostQuarantined     = "quarantined"
	HostDecommissioned  = "decommissioned"

	HostStatusSuccess = "success"
	HostStatusFailed  = "failed"

	SpawnRequestInit       = "initializing"
	SpawnRequestReady      = "ready"
	SpawnRequestUnusable   = "unusable"
	SpawnRequestTerminated = "terminated"

	TaskStarted      = "started"
	TaskUndispatched = "undispatched"
	TaskDispatched   = "dispatched"
	TaskFailed       = "failed"
	TaskCancelled    = "cancelled"
	TaskSucceeded    = "success"

	TestFailedStatus    = "fail"
	TestSkippedStatus   = "skip"
	TestSucceededStatus = "pass"

	BuildStarted   = "started"
	BuildCreated   = "created"
	BuildFailed    = "failed"
	BuildSucceeded = "success"
	BuildCancelled = "cancelled"

	VersionStarted   = "started"
	VersionCreated   = "created"
	VersionFailed    = "failed"
	VersionSucceeded = "success"
	VersionCancelled = "cancelled"

	PatchCreated   = "created"
	PatchStarted   = "started"
	PatchSucceeded = "succeeded"
	PatchFailed    = "failed"
	PatchCancelled = "cancelled"

	PushLogPushing = "pushing"
	PushLogSuccess = "success"

	HostTypeStatic = "static"
	HostTypeEC2    = "ec2"

	CompileStage = "compile"
	TestStage    = "single_test"
	SanityStage  = "smokeCppUnitTests"
	PushStage    = "push"

	// max allowed hosts
	TooManyHosts = 150

	// maximum task (zero based) execution number
	MaxTaskExecution = 3

	// LogMessage struct versions
	LogmessageFormatTimestamp = 1
	LogmessageCurrentVersion  = LogmessageFormatTimestamp

	EvergreenHome = "mci_home"
)

// evergreen package names
const (
	UIPackage = "EVERGREEN_UI"
)

const (
	AuthTokenCookie  = "mci-token"
	TaskSecretHeader = "Task-Secret"
)

var (
	UphostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostInitializing,
		HostProvisionFailed,
		HostUnreachable,
	}

	// vars instead of consts so they can be changed for testing
	Logger = slogger.Logger{
		Prefix:    "",
		Appenders: []slogger.Appender{slogger.StdOutAppender()},
	}

	// database and config directory, set to the testing version by default for safety
	NotificationsFile = "mci-notifications.yml"

	// version requester types
	PatchVersionRequester       = "patch_request"
	RepotrackerVersionRequester = "gitter_request"

	// constant arrays for db update logic
	AbortableStatuses = []string{TaskStarted, TaskDispatched}
	CompletedStatuses = []string{TaskSucceeded, TaskFailed}
)

func SetLogger(logPath string) {
	logfile, err := util.GetAppendingFile(logPath)
	if err != nil {
		panic(fmt.Sprintf("Cannot create log file %v: %v", logPath, err))
	}

	Logger = slogger.Logger{
		Prefix:    "",
		Appenders: []slogger.Appender{&slogger.FileAppender{logfile}},
	}
}

func FindEvergreenHome() string {
	// check if env var is set
	root := os.Getenv("EVGHOME")
	if len(root) > 0 {
		return root
	}
	// TODO: Legacy--remove once we're far from the name transition
	root = os.Getenv("mci_home")
	if len(root) > 0 {
		Logger.Logf(slogger.WARN, "'mci_home' environment variable is deprecated; please use 'EVGHOME' instead")
		return root
	}
	return root
}

func FindConfig(configName string) (string, error) {
	home := FindEvergreenHome()
	if len(home) > 0 {
		root, yes := isConfigRoot(home, configName)
		if yes {
			return root, nil
		}
		return "", fmt.Errorf("Can't find mci config root: '%v'", root)
	}
	return "", fmt.Errorf("%v environment variable must be set", EvergreenHome)
}

func isConfigRoot(home string, configName string) (fixed string, is bool) {
	fixed = filepath.Join(home, configName)
	fixed = strings.Replace(fixed, "\\", "/", -1)
	stat, err := os.Stat(fixed)
	if err == nil && stat.IsDir() {
		is = true
	}
	return
}

func TestConfig() *Settings {
	evgHome := FindEvergreenHome()
	file := filepath.Join(evgHome, TestDir, TestSettings)
	settings, err := NewSettings(file)
	if err != nil {
		panic(err)
	}
	return settings
}
