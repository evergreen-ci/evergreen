package taskrunner

import (
	"bytes"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"
)

const (
	MakeShellTimeout  = time.Second * 10
	SCPTimeout        = time.Minute
	StartAgentTimeout = time.Second * 10
	agentFile         = "agent.log"
)

// HostGateway is responsible for kicking off tasks on remote machines.
type HostGateway interface {
	// determine if the agent needs be rebuilt
	AgentNeedsBuild() (bool, error)
	// run any necessary setup before running tasks
	RunSetup() error
	// run the specified task on the specified host, return the revision of the
	// agent running the task on that host
	RunTaskOnHost(*evergreen.Settings, model.Task, host.Host) (string, error)
	// gets the current revision of the agent
	GetAgentRevision() (string, error)
}

// Implementation of the HostGateway that builds and copies over the MCI
// agent to run tasks.
type AgentBasedHostGateway struct {
	// Responsible for cross-compiling the agent
	Compiler AgentCompiler
	// Absolute path to the directory where the agent package lives
	AgentPackageDir string
	// Destination directory for the agent executables
	ExecutablesDir string

	// Internal cache of the agent package's current git hash
	currentAgentHash string
}

// Prepares to run the tasks it needs to, by building the agent if necessary.
// Returns an error if any step along the way throws an error.
func (self *AgentBasedHostGateway) RunSetup() error {
	// rebuild the agent, if necessary
	needsBuild, err := self.AgentNeedsBuild()
	if err != nil {
		return fmt.Errorf("error checking if agent needs to be built: %v",
			err)
	}
	if needsBuild {
		evergreen.Logger.Logf(slogger.INFO, "Rebuilding agent package...")
		err := self.buildAgent()
		if err != nil {
			return fmt.Errorf("error building agent: %v", err)
		}
		evergreen.Logger.Logf(slogger.INFO, "Agent package successfully rebuilt")
	} else {
		evergreen.Logger.Logf(slogger.INFO, "Agent package does not need to be"+
			" rebuilt")
	}
	return nil
}

// Start the task specified, on the host specified.  First runs any necessary
// preparation on the remote machine, then kicks off the agent process on the
// machine.
// Returns an error if any step along the way fails.
func (self *AgentBasedHostGateway) RunTaskOnHost(settings *evergreen.Settings,
	taskToRun model.Task, hostObj host.Host) (string, error) {

	// cache mci home
	evgHome := evergreen.FindEvergreenHome()

	// get the host's SSH options
	cloudHost, err := providers.GetCloudHost(&hostObj, settings)
	if err != nil {
		return "", fmt.Errorf("Failed to get cloud host for %v: %v", hostObj.Id, err)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return "", fmt.Errorf("Error getting ssh options for host %v: %v", hostObj.Id, err)
	}

	// prep the remote host
	evergreen.Logger.Logf(slogger.INFO, "Prepping remote host %v...", hostObj.Id)
	agentRevision, err := self.prepRemoteHost(settings, hostObj, sshOptions, evgHome)
	if err != nil {
		return "", fmt.Errorf("error prepping remote host %v: %v", hostObj.Id, err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Prepping host finished successfully")

	// start the agent on the remote machine
	evergreen.Logger.Logf(slogger.INFO, "Starting agent on host %v for task %v...",
		hostObj.Id, taskToRun.Id)

	err = self.startAgentOnRemote(settings, &taskToRun, &hostObj, sshOptions)
	if err != nil {
		return "", fmt.Errorf("error starting agent on %v for task %v: %v",
			hostObj.Id, taskToRun.Id, err)
	}
	evergreen.Logger.Logf(slogger.INFO, "Agent successfully started")

	return agentRevision, nil
}

// Gets the git revision of the currently built agent
func (self *AgentBasedHostGateway) GetAgentRevision() (string, error) {

	versionFile := filepath.Join(self.ExecutablesDir, "version")
	hashBytes, err := ioutil.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("error reading agent version file: %v", err)
	}

	return strings.TrimSpace(string(hashBytes)), nil
}

// Determines if either there is no currently built version of the agent,
// or if the currently built version is out of date.
// Returns whether a new version needs to be built, and an error if any
// of the checks within the function throw an error.
func (self *AgentBasedHostGateway) AgentNeedsBuild() (bool, error) {

	// compute and cache the current hash of the agent package
	agentHash, err := util.CurrentGitHash(self.AgentPackageDir)
	if err != nil {
		return false, fmt.Errorf("error getting git hash of agent package "+
			"at %v: %v", self.AgentPackageDir, err)
	}
	self.currentAgentHash = agentHash

	// if the local directory where the executables live does not exist, then we
	// certainly need to build the agent
	exists, err := util.FileExists(self.ExecutablesDir)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}

	// if the most recently built agent hash is different from the current
	// mci hash, then the agent needs to be rebuilt
	lastBuiltRevision, err := db.GetLastAgentBuild()
	if err != nil {
		return false, fmt.Errorf("error getting last agent build hash: %v", err)
	}

	return self.currentAgentHash != lastBuiltRevision, nil
}

// Compile the agent package into the appropriate binaries.
// Returns an error if the compilation fails, or if storing the last built
// hash in the db fails
func (self *AgentBasedHostGateway) buildAgent() error {

	// sanity check that we have an available agent compiler
	if self.Compiler == nil {
		panic("no AgentCompiler was initialized to cross-compile the agent" +
			" package")
	}

	// compile the agent to the appropriate destination
	err := self.Compiler.Compile(self.AgentPackageDir, self.ExecutablesDir)
	if err != nil {
		return fmt.Errorf("error building agent from %v to %v: %v",
			self.AgentPackageDir, self.ExecutablesDir, err)
	}

	// record the build as successful
	err = db.StoreLastAgentBuild(self.currentAgentHash)
	if err != nil {
		return fmt.Errorf("error saving last built agent hash: %v", err)
	}

	return nil
}

// Prepare the remote machine to run a task.
func (self *AgentBasedHostGateway) prepRemoteHost(settings *evergreen.Settings,
	hostObj host.Host, sshOptions []string, mciHome string) (string, error) {

	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return "", fmt.Errorf("error parsing ssh info %v: %v", hostObj.Host, err)
	}

	// first, create the necessary sandbox of directories on the remote machine
	makeShellCmd := &command.RemoteCommand{
		CmdString:      fmt.Sprintf("mkdir -m 777 -p %v", hostObj.Distro.WorkDir),
		Stdout:         ioutil.Discard, // TODO(EVG-233) change to real logging
		Stderr:         ioutil.Discard,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     false,
	}

	evergreen.Logger.Logf(slogger.INFO, "Directories command: '%#v'", makeShellCmd)

	// run the make shell command with a timeout
	err = util.RunFunctionWithTimeout(
		makeShellCmd.Run,
		MakeShellTimeout,
	)
	if err != nil {
		// if it timed out, kill the command
		if err == util.ErrTimedOut {
			makeShellCmd.Stop()
			return "", fmt.Errorf("creating remote directories timed out")
		}
		return "", fmt.Errorf("error creating directories on remote machine: %v", err)
	}

	scpConfigsCmd := &command.ScpCommand{
		Source:         filepath.Join(mciHome, settings.ConfigDir),
		Dest:           hostObj.Distro.WorkDir,
		Stdout:         ioutil.Discard, // TODO(EVG-233) change to real logging
		Stderr:         ioutil.Discard,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options: append([]string{"-P", hostInfo.Port, "-r"},
			sshOptions...),
	}

	// run the command to scp the configs with a timeout
	err = util.RunFunctionWithTimeout(
		scpConfigsCmd.Run,
		SCPTimeout,
	)
	if err != nil {
		// if it timed out, kill the scp command
		if err == util.ErrTimedOut {
			scpConfigsCmd.Stop()
			return "", fmt.Errorf("scp-ing config directory timed out")
		}
		return "", fmt.Errorf("error copying config directory to remote: "+
			"machine %v", err)
	}

	// third, copy over the correct agent binary to the remote machine
	var scpAgentCmdStderr bytes.Buffer
	executableSubPath, err := self.Compiler.ExecutableSubPath(hostObj.Distro.Id)
	if err != nil {
		return "", fmt.Errorf("error computing subpath to executable: %v", err)
	}
	scpAgentCmd := &command.ScpCommand{
		Source:         filepath.Join(self.ExecutablesDir, executableSubPath),
		Dest:           hostObj.Distro.WorkDir,
		Stdout:         ioutil.Discard, // TODO(EVG-233) change to real logging
		Stderr:         &scpAgentCmdStderr,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-P", hostInfo.Port}, sshOptions...),
	}

	// get the agent's revision before scp'ing over the executable
	preSCPAgentRevision, err := self.GetAgentRevision()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error getting pre scp agent "+
			"revision: %v", err)
	}

	// run the command to scp the agent with a timeout
	err = util.RunFunctionWithTimeout(
		scpAgentCmd.Run,
		SCPTimeout,
	)
	if err != nil {
		if err == util.ErrTimedOut {
			scpAgentCmd.Stop()
			return "", fmt.Errorf("scp-ing agent binary timed out")
		}
		return "", fmt.Errorf("error (%v) copying agent binary to remote "+
			"machine: %v", err, scpAgentCmdStderr.String())
	}

	// get the agent's revision after scp'ing over the executable
	postSCPAgentRevision, err := self.GetAgentRevision()
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error getting post scp agent "+
			"revision: %v", err)
	}

	if preSCPAgentRevision != postSCPAgentRevision {
		evergreen.Logger.Logf(slogger.WARN, "Agent revision was %v before scp "+
			"but is now %v. Using previous revision %v for host %v",
			preSCPAgentRevision, postSCPAgentRevision, preSCPAgentRevision,
			hostObj.Id)
	}

	return preSCPAgentRevision, nil
}

// Start the agent process on the specified remote host, and have it run
// the specified task.
// Returns an error if starting the agent remotely fails.
func (self *AgentBasedHostGateway) startAgentOnRemote(
	settings *evergreen.Settings, task *model.Task,
	hostObj *host.Host, sshOptions []string) error {

	// the path to the agent binary on the remote machine
	pathToExecutable := filepath.Join(hostObj.Distro.WorkDir, "main")

	// build the command to run on the remote machine
	remoteCmd := fmt.Sprintf(
		`%v -api_server "%v" -task_id "%v" -task_secret "%v" -log_file "%v" -https_cert "%v"`,
		pathToExecutable, settings.ApiUrl, task.Id, task.Secret, filepath.Join(hostObj.Distro.WorkDir,
			agentFile), settings.Expansions["api_httpscert_path"],
	)
	evergreen.Logger.Logf(slogger.INFO, "%v", remoteCmd)

	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return fmt.Errorf("error parsing ssh info %v: %v", hostObj.Host, err)
	}

	// run the command to kick off the agent remotely
	startAgentCmd := &command.RemoteCommand{
		CmdString:      remoteCmd,
		Stdout:         ioutil.Discard,
		Stderr:         ioutil.Discard,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     true,
	}

	// run the command to start the agent with a timeout
	err = util.RunFunctionWithTimeout(
		startAgentCmd.Run,
		StartAgentTimeout,
	)
	if err != nil {
		if err == util.ErrTimedOut {
			startAgentCmd.Stop()
			return fmt.Errorf("starting agent timed out")
		}
		return fmt.Errorf("error starting agent on host %v: %v", hostObj.Id, err)
	}

	return nil
}
