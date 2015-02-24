package mci

import (
	"10gen.com/mci/util"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"os"
	"path/filepath"
	"strings"
)

const (
	Revisionfile    = "revision.txt"
	DefaultRevision = "DEV"
)

const (
	MCIUser = "mci"

	UnittestConfDir          = "config_test"
	UnittestSettingsFilename = "mci_settings.yml"

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

	// patch types
	ProjectPatch = "project"
	ModulePatch  = "module"

	CompileStage = "compile"
	TestStage    = "single_test"
	SanityStage  = "smokeCppUnitTests"
	PushStage    = "push"

	// max MCI hosts
	TooManyHosts = 150

	// maximum MCI task (zero based) execution number
	MaxTaskExecution = 3

	// LogMessage struct versions
	LogmessageFormatTimestamp = 1
	LogmessageCurrentVersion  = LogmessageFormatTimestamp
)

// mci package names
const (
	AuthPackage        = "MCI_AUTH"
	BookkeepingPackage = "MCI_BOOKKEEPING"
	CloudPackage       = "MCI_CLOUD"
	CleanupPackage     = "MCI_CLEANUP"
	DBPackage          = "MCI_DB"
	AgentPackage       = "MCI_AGENT"
	RepotrackerPackage = "MCI_REPOTRACKER"
	HostPackage        = "MCI_HOST"
	HostInitPackage    = "MCI_INIT"
	MonitorPackage     = "MCI_MONITOR"
	NotifyPackage      = "MCI_NOTIFY"
	PatchPackage       = "MCI_PATCH"
	SchedulerPackage   = "MCI_SCHEDULER"
	APIServerPackage   = "MCI_APISERVER"
	TaskRunnerPackage  = "MCI_TASK_RUNNER"
	ThirdpartyPackage  = "MCI_THIRDPARTY"
	UIPackage          = "MCI_UI"
)

const AuthTokenCookie = "mci-token"
const TaskSecretHeader = "Task-Secret"
const DistrosFile = "distros.yml"

var (
	UphostStatus = []string{
		HostRunning,
		HostUninitialized,
		HostInitializing,
		HostProvisionFailed,
		HostUnreachable,
	}
	// vars instead of consts so they can be changed for testing
	RemoteRoot  = "/data/mci/"
	RemoteShell = RemoteRoot + "shell/"
	Logger      = slogger.Logger{
		Prefix:    "",
		Appenders: []slogger.Appender{slogger.StdOutAppender()},
	}

	// database and config directory, set to the testing version by default for safety
	MCIDBURL          = "mongodb://localhost:27017/mci_test"
	MCIDatabase       = "mci_test"
	Mode              = "test"
	NotificationsFile = "mci-notifications.yml"
	ProjectFile       = "mongodb-mongo-master"
	VersionFile       = "mci-version.yml"
	DropCollections   = false

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

func FindMCIHome() (string, error) {

	// check if env var is set
	root := os.Getenv("mci_home")
	if len(root) > 0 {
		return root, nil
	}

	// if the env var isn't set, use the remote shell
	return RemoteShell, nil

}

func FindMCIConfig(configName string) (string, error) {

	// check if env var is set
	root := os.Getenv("mci_home")
	if len(root) > 0 {
		if fixed, is := isConfigRoot(root, configName); is == true {
			return fixed, nil
		} else {
			return "", fmt.Errorf("Can't find mci config root '%v'", configName)
		}
	}

	// check on remote shell
	if fixed, is := isConfigRoot(RemoteShell, configName); is {
		return fixed, nil
	}

	return "", fmt.Errorf("Can't find mci config root '%v'", configName)
}

// @return fixed : "potential/config"
// @return is : true if "potential/config" exists and is a dir.
func isConfigRoot(potential string, configName string) (fixed string, is bool) {
	is = false
	fixed = filepath.Join(potential, configName)
	fixed = strings.Replace(fixed, "\\", "/", -1)
	stat, err := os.Stat(fixed)
	if err == nil && stat.IsDir() {
		is = true
	}
	return
}

func TestConfig() *MCISettings {
	mciHome, err := FindMCIHome()
	if err != nil {
		panic(err)
	}
	confFileLocation := filepath.Join(mciHome, UnittestConfDir,
		UnittestSettingsFilename)
	settings, err := NewMCISettings(confFileLocation)
	if err != nil {
		panic(err)
	}
	return settings
}

// AllProjectNames returns the un-suffixed form of *all* files
// (projects) in the project directory
func AllProjectNames(configName string) (projectNames []string, err error) {
	configRoot, err := FindMCIConfig(configName)
	if err != nil {
		return
	}

	projectDirectory, err := os.Open(filepath.Join(configRoot, "project"))
	if err != nil {
		return
	}

	allFileNames, err := projectDirectory.Readdirnames(-1)
	if err != nil {
		return
	}

	projectNames = make([]string, 0, len(allFileNames))
	for _, fileName := range allFileNames {
		if strings.HasSuffix(fileName, ".yml") {
			projectNames = append(projectNames, strings.TrimSuffix(fileName, ".yml"))
		}
	}

	return
}
