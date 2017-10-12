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
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// SSHTimeout defines the timeout for the SSH commands in this package.
	SSHTimeout = 1 * time.Minute
	agentFile  = "agent"
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

// Start an agent on the host specified.  First runs any necessary
// preparation on the remote machine, then kicks off the agent process on the
// machine. Returns an error if any step along the way fails.
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
	agentRevision, err := agbh.prepRemoteHost(hostObj, sshOptions, settings)
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

	// Start agent to listen for tasks
	err = startAgentOnRemote(settings, &hostObj, sshOptions)

	if err != nil {
		// mark the host's provisioning as failed
		if err := hostObj.SetUnprovisioned(); err != nil {
			grip.Errorf("unprovisioning host %s failed: %+v", hostObj.Id, err)
		}
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

func newCappedOutputLog() *util.CappedWriter {
	// store up to 1MB of streamed command output to print if a command fails
	return &util.CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1MB
	}
}

// Prepare the remote machine to run a task.
// 1. Creating the directories on the remote host
// 2. Copying the agent into that directory.
// 3. Running the agent in setup_only mode to run the setup script.
func (agbh *AgentHostGateway) prepRemoteHost(hostObj host.Host, sshOptions []string, settings *evergreen.Settings) (string, error) {
	// create the necessary sandbox of directories on the remote host
	if err := runSSHCommand("mkdir", fmt.Sprintf("mkdir -m 777 -p %v", hostObj.Distro.WorkDir), sshOptions, hostObj); err != nil {
		return "", errors.Wrap(err, "failed to create directories on remote host")
	}

	// copy over the correct agent binary to the remote host
	if err := runSSHCommand("curl", hostutil.CurlCommand(settings.Ui.Url, &hostObj), sshOptions, hostObj); err != nil {
		return "", errors.Wrap(err, "error downloading agent binary on remote host")
	}

	// run the setup script with the agent
	if err := runSSHCommand("setup", hostutil.SetupCommand(&hostObj), sshOptions, hostObj); err != nil {
		return "", errors.Wrap(err, "error running setup script on remote host")
	}

	return agbh.GetAgentRevision()
}

func runSSHCommand(id, cmd string, sshOptions []string, host host.Host) error {
	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(host.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %v", host.Host)
	}

	output := newCappedOutputLog()
	shellCmd := &subprocess.RemoteCommand{
		Id:             fmt.Sprintf("%s-%s-%d", id, host.Id, rand.Int()),
		CmdString:      cmd,
		Stdout:         output,
		Stderr:         output,
		RemoteHostName: hostInfo.Hostname,
		User:           host.User,
		Options:        append([]string{"-p", hostInfo.Port}, sshOptions...),
	}
	grip.Infof("running command over ssh: %+v")

	ctx, cancel := context.WithTimeout(context.Background(), SSHTimeout)
	defer cancel()
	err = shellCmd.Run(ctx)

	grip.Notice(shellCmd.Stop())
	if err != nil {
		return errors.Errorf("error running shell cmd: %s (%v)", output.String(), err)
	}
	return nil
}

const logAggregationEnabled = false

// Start the agent process on the specified remote host.
func startAgentOnRemote(settings *evergreen.Settings, hostObj *host.Host, sshOptions []string) error {
	// the path to the agent binary on the remote machine
	pathToExecutable := filepath.Join("~", "evergreen")
	if hostutil.IsWindows(&hostObj.Distro) {
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
	// build the command to run on the remote machine
	remoteCmd := strings.Join(agentCmdParts, " ")
	cmdId := fmt.Sprintf("startagent-%s-%d", hostObj.Id, rand.Int())
	grip.Info(message.Fields{
		"id":      cmdId,
		"message": "starting agent on host",
		"host":    hostObj.Id,
		"command": remoteCmd,
	})

	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %v", hostObj.Host)
	}

	// run the command to kick off the agent remotely
	var startAgentLog bytes.Buffer
	startAgentCmd := &subprocess.RemoteCommand{
		Id:             cmdId,
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

	ctx, cancel := context.WithTimeout(context.TODO(), SSHTimeout)
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
