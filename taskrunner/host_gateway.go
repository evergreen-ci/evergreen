package taskrunner

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	MakeShellTimeout  = 2 * time.Minute
	SCPTimeout        = 3 * time.Minute
	StartAgentTimeout = 2 * time.Minute
	agentFile         = "agent"
)

// HostGateway is responsible for kicking off tasks on remote machines.
type HostGateway interface {
	// run the specified task on the specified host, return the revision of the
	// agent running the task on that host
	StartAgentOnHost(*evergreen.Settings, host.Host) error
	// gets the current revision of the agent
	GetAgentRevision() (string, error)
}

// Implementation of the HostGateway that builds and copies over the MCI
// agent to run tasks.
type AgentHostGateway struct {
	// Destination directory for the agent executables
	ExecutablesDir string
}

// Start the task specified, on the host specified.  First runs any necessary
// preparation on the remote machine, then kicks off the agent process on the
// machine.
// Returns an error if any step along the way fails.
func (agbh *AgentHostGateway) StartAgentOnHost(settings *evergreen.Settings, hostObj host.Host) error {

	// get the host's SSH options
	cloudHost, err := providers.GetCloudHost(&hostObj, settings)
	if err != nil {
		return errors.Wrapf(err, "Failed to get cloud host for %s", hostObj.Id)
	}
	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "Error getting ssh options for host %s", hostObj.Id)
	}

	d, err := distro.FindOne(distro.ById(hostObj.Distro.Id))
	if err != nil {
		return errors.Wrapf(err, "error finding distro %s", hostObj.Distro.Id)
	}
	hostObj.Distro = *d

	// prep the remote host
	grip.Infof("Prepping remote host %v...", hostObj.Id)
	agentRevision, err := agbh.prepRemoteHost(hostObj, sshOptions)
	if err != nil {
		return errors.Wrapf(err, "error prepping remote host %s", hostObj.Id)
	}
	grip.Infof("Prepping host %v finished successfully", hostObj.Id)

	// start the agent on the remote machine
	grip.Infof("Starting agent on host %v", hostObj.Id)

	// generate the host secret if none exists
	if hostObj.Secret == "" {
		if err = hostObj.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", hostObj.Id)
		}
	}

	err = startAgentOnRemote(settings, &hostObj, sshOptions)
	if err != nil {
		return errors.WithStack(err)
	}
	grip.Infof("Agent successfully started for host %v", hostObj.Id)

	err = hostObj.SetAgentRevision(agentRevision)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Gets the git revision of the currently built agent
func (agbh *AgentHostGateway) GetAgentRevision() (string, error) {
	versionFile := filepath.Join(agbh.ExecutablesDir, "version")
	hashBytes, err := ioutil.ReadFile(versionFile)
	if err != nil {
		return "", errors.Wrap(err, "error reading agent version file")
	}

	return strings.TrimSpace(string(hashBytes)), nil
}

// executableSubPath returns the directory containing the compiled agents.
func executableSubPath(d *distro.Distro) string {
	mainName := "evergreen"
	if isWindows(d) {
		mainName += ".exe"
	}

	return filepath.Join(d.Arch, mainName)
}

func isWindows(d *distro.Distro) bool {
	return strings.HasPrefix(d.Arch, "windows")
}

func newCappedOutputLog() *util.CappedWriter {
	// store up to 1MB of streamed command output to print if a command fails
	return &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
}

// Prepare the remote machine to run a task.
// This consists of:
// 1. Creating the directories on the remote host where, according to the distro's settings,
//    the agent should be placed.
// 2. Copying the agent into that directory.
func (agbh *AgentHostGateway) prepRemoteHost(hostObj host.Host, sshOptions []string) (string, error) {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return "", errors.Wrapf(err, "error parsing ssh info %v", hostObj.Host)
	}

	// first, create the necessary sandbox of directories on the remote machine
	mkdirOutput := newCappedOutputLog()
	makeShellCmd := &subprocess.RemoteCommand{
		Id:             fmt.Sprintf("agent_mkdir-%v", rand.Int()),
		CmdString:      fmt.Sprintf("mkdir -m 777 -p %v", hostObj.Distro.WorkDir),
		Stdout:         mkdirOutput,
		Stderr:         mkdirOutput,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     false,
	}
	grip.Infof("Directories command: '%#v'", makeShellCmd)

	// run the make shell command with a timeout
	ctx, cancel := context.WithTimeout(context.TODO(), MakeShellTimeout)
	defer cancel()
	err = makeShellCmd.Run(ctx)

	grip.Notice(makeShellCmd.Stop())
	if err != nil {
		if err == util.ErrTimedOut {
			return "", errors.Errorf("creating remote directories timed out: %s",
				mkdirOutput.String())
		}
		return "", errors.Wrapf(err, "error creating directories on remote machine: %s",
			mkdirOutput.String())
	}

	// third, copy over the correct agent binary to the remote machine
	scpAgentOutput := newCappedOutputLog()
	scpAgentCmd := &subprocess.ScpCommand{
		Id:             fmt.Sprintf("scp%v", rand.Int()),
		Source:         filepath.Join(agbh.ExecutablesDir, executableSubPath(&hostObj.Distro)),
		Dest:           hostObj.Distro.WorkDir,
		Stdout:         scpAgentOutput,
		Stderr:         scpAgentOutput,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-P", hostInfo.Port}, sshOptions...),
	}

	// get the agent's revision before scp'ing over the executable
	preSCPAgentRevision, err := agbh.GetAgentRevision()
	grip.Error(errors.Wrap(err, "Error getting pre scp agent revision"))

	// run the command to scp the agent with a timeout
	ctx, cancel = context.WithTimeout(context.TODO(), SCPTimeout)
	defer cancel()
	err = scpAgentCmd.Run(ctx)
	grip.Notice(scpAgentCmd.Stop())
	if err != nil {
		if err == util.ErrTimedOut {
			return "", errors.Errorf("scp-ing agent binary timed out: %v", scpAgentOutput.String())
		}
		return "", errors.Errorf(
			"error copying agent binary to remote machine (%v): %v", err, scpAgentOutput.String())
	}

	// get the agent's revision after scp'ing over the executable
	postSCPAgentRevision, err := agbh.GetAgentRevision()
	grip.Error(errors.Wrap(err, "Error getting post scp agent revision"))
	grip.WarningWhenf(preSCPAgentRevision != postSCPAgentRevision,
		"Agent revision was %v before scp but is now %v. Using previous revision %v for host %v",
		preSCPAgentRevision, postSCPAgentRevision, preSCPAgentRevision, hostObj.Id)

	return preSCPAgentRevision, nil
}

const logAggregationEnabled = false

// Start the agent process on the specified remote host.
func startAgentOnRemote(settings *evergreen.Settings, hostObj *host.Host, sshOptions []string) error {
	// the path to the agent binary on the remote machine
	pathToExecutable := filepath.Join(hostObj.Distro.WorkDir, "evergreen")
	if isWindows(&hostObj.Distro) {
		pathToExecutable += ".exe"
	}

	agentCmdParts := []string{
		pathToExecutable,
		"agent",
		fmt.Sprintf("--api_server='%s'", settings.ApiUrl),
		fmt.Sprintf("--host_id='%s'", hostObj.Id),
		fmt.Sprintf("--host_secret='%s'", hostObj.Secret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(hostObj.Distro.WorkDir, agentFile)),
		fmt.Sprintf("--working_directory='%s'", hostObj.Distro.WorkDir),
	}
	if hostObj.Distro.SetupAsSudo {
		agentCmdParts = append(agentCmdParts, "--setup_as_sudo")
	}

	// build the command to run on the remote machine
	remoteCmd := strings.Join(agentCmdParts, " ")
	grip.Info(remoteCmd)

	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %v", hostObj.Host)
	}

	// run the command to kick off the agent remotely
	var startAgentLog bytes.Buffer
	startAgentCmd := &subprocess.RemoteCommand{
		Id:             fmt.Sprintf("startagent-%s-%d", hostObj.Id, rand.Int()),
		CmdString:      remoteCmd,
		Stdout:         &startAgentLog,
		Stderr:         &startAgentLog,
		RemoteHostName: hostInfo.Hostname,
		User:           hostObj.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
		Background:     true,
	}

	if sumoEndpoint, ok := settings.Credentials["sumologic"]; ok {
		startAgentCmd.EnvVars = []string{fmt.Sprintf("GRIP_SUMO_ENDPOINT='%s'", sumoEndpoint)}
	}

	if logAggregationEnabled {
		startAgentCmd.EnvVars = []string{
			fmt.Sprintf("GRIP_SPLUNK_SERVER_URL='%s'", settings.Splunk.ServerURL),
			fmt.Sprintf("GRIP_SPLUNK_CLIENT_TOKEN='%s'", settings.Splunk.Token),
		}
		if settings.Splunk.Channel != "" {
			startAgentCmd.EnvVars = append(startAgentCmd.EnvVars,
				fmt.Sprintf("GRIP_SPLUNK_CHANNEL='%s'", settings.Splunk.Channel))
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), StartAgentTimeout)
	defer cancel()
	err = startAgentCmd.Run(ctx)

	// run cleanup regardless of what happens.
	grip.Notice(startAgentCmd.Stop())

	if err != nil {
		if err == util.ErrTimedOut {
			return errors.Errorf("starting agent timed out on %s", hostObj.Id)
		}
		return errors.Wrapf(err, "error starting agent (%v): %v", hostObj.Id, startAgentLog.String())
	}
	return nil
}
