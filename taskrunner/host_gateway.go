package taskrunner

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/hostutil"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	// SSHTimeout defines the timeout for the SSH commands in this package.
	sshTimeout = 30 * time.Second
	agentFile  = "agent"
)

// HostGateway is responsible for kicking off tasks on remote machines.
type HostGateway interface {
	// run the specified task on the specified host, return the revision of the
	// agent running the task on that host
	StartAgentOnHost(context.Context, *evergreen.Settings, host.Host) error
	// gets the current revision of the agent
	GetAgentRevision() (string, error)
}

// Implementation of the HostGateway that builds and copies over the MCI
// agent to run tasks.
type AgentHostGateway struct {
	// Destination directory for the agent executables
	ExecutablesDir string
}

func getHostMessage(h host.Host) message.Fields {
	m := message.Fields{
		"message":  "starting agent on host",
		"runner":   "taskrunner",
		"host":     h.Host,
		"distro":   h.Distro.Id,
		"provider": h.Distro.Provider,
	}

	if h.InstanceType != "" {
		m["instance"] = h.InstanceType
	}

	sinceLCT := time.Since(h.LastCommunicationTime)
	if h.NeedsNewAgent {
		m["reason"] = "flagged for new agent"
	} else if h.LastCommunicationTime.IsZero() {
		m["reason"] = "new host"
	} else if sinceLCT > host.MaxLCTInterval {
		m["reason"] = "host has exceeded last communication threshold"
		m["threshold"] = host.MaxLCTInterval
		m["threshold_span"] = host.MaxLCTInterval.String()
		m["last_communication_at"] = sinceLCT
		m["last_communication_at_time"] = sinceLCT.String()
	}

	return m
}

// Start an agent on the host specified.  First runs any necessary
// preparation on the remote machine, then kicks off the agent process on the
// machine. Returns an error if any step along the way fails.
func (agbh *AgentHostGateway) StartAgentOnHost(ctx context.Context, settings *evergreen.Settings, hostObj host.Host) error {

	// get the host's SSH options
	cloudHost, err := cloud.GetCloudHost(&hostObj, settings)
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
	grip.Info(message.Fields{"runner": RunnerName,
		"message": "prepping host for agent",
		"host":    hostObj.Id})
	agentRevision, err := agbh.prepRemoteHost(ctx, hostObj, sshOptions, settings)
	if err != nil {
		return errors.Wrapf(err, "error prepping remote host %s", hostObj.Id)
	}
	grip.Info(message.Fields{"runner": RunnerName, "message": "prepping host finished successfully", "host": hostObj.Id})

	// generate the host secret if none exists
	if hostObj.Secret == "" {
		if err = hostObj.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", hostObj.Id)
		}
	}

	// Start agent to listen for tasks
	grip.Info(getHostMessage(hostObj))
	if err = startAgentOnRemote(settings, &hostObj, sshOptions); err != nil {
		// mark the host's provisioning as failed
		if err = hostObj.SetUnprovisioned(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"runner":  RunnerName,
				"host_id": hostObj.Id,
				"message": "unprovisioning host failed",
			}))
		}

		event.LogHostAgentDeployFailed(hostObj.Id)

		return errors.WithStack(err)
	}
	grip.Info(message.Fields{"runner": RunnerName, "message": "agent successfully started for host", "host": hostObj.Id})

	if err = hostObj.SetAgentRevision(agentRevision); err != nil {
		return errors.Wrapf(err, "error setting agent revision on host %s", hostObj.Id)
	}
	if err = hostObj.UpdateLastCommunicated(); err != nil {
		return errors.Wrapf(err, "error setting LCT on host %s", hostObj.Id)
	}
	if err = hostObj.SetNeedsNewAgent(false); err != nil {
		return errors.Wrapf(err, "error setting needs agent flag on host %s", hostObj.Id)
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

// Prepare the remote machine to run a task.
func (agbh *AgentHostGateway) prepRemoteHost(ctx context.Context, hostObj host.Host, sshOptions []string, settings *evergreen.Settings) (string, error) {
	// copy over the correct agent binary to the remote host
	if logs, err := hostutil.RunSSHCommand(ctx, hostutil.CurlCommand(settings.Ui.Url, &hostObj), sshOptions, hostObj); err != nil {
		return "", errors.Wrapf(err, "error downloading agent binary on remote host: %s", logs)
	}

	// return early if we do not need to run the setup script
	if hostObj.Distro.Setup == "" {
		return agbh.GetAgentRevision()
	}

	// run the setup script with the agent
	if logs, err := hostutil.RunSSHCommand(ctx, hostutil.SetupCommand(&hostObj), sshOptions, hostObj); err != nil {
		event.LogProvisionFailed(hostObj.Id, logs)

		grip.Error(message.WrapError(err, message.Fields{
			"message": "error running setup script",
			"host_id": hostObj.Id,
			"distro":  hostObj.Distro.Id,
			"runner":  RunnerName,
			"logs":    logs,
		}))

		// there is no guarantee setup scripts are idempotent, so we terminate the host if the setup script fails
		if disableErr := hostObj.DisablePoisonedHost(true); disableErr != nil {
			return "", errors.Wrapf(disableErr, "error terminating host %s", hostObj.Id)
		}

		return "", errors.Wrapf(err, "error running setup script on remote host: %s", logs)
	}

	return agbh.GetAgentRevision()
}

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
		"--cleanup",
	}

	// build the command to run on the remote machine
	remoteCmd := strings.Join(agentCmdParts, " ")
	grip.Info(message.Fields{
		"message": "starting agent on host",
		"host":    hostObj.Id,
		"command": remoteCmd,
		"runner":  RunnerName,
	})

	// compute any info necessary to ssh into the host
	hostInfo, err := util.ParseSSHInfo(hostObj.Host)
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %v", hostObj.Host)
	}

	// run the command to kick off the agent remotely
	env := map[string]string{}
	if sumoEndpoint, ok := settings.Credentials["sumologic"]; ok {
		env["GRIP_SUMO_ENDPOINT"] = sumoEndpoint
	}

	if settings.Splunk.Populated() {
		env["GRIP_SPLUNK_SERVER_URL"] = settings.Splunk.ServerURL
		env["GRIP_SPLUNK_CLIENT_TOKEN"] = settings.Splunk.Token

		if settings.Splunk.Channel != "" {
			env["GRIP_SPLUNK_CHANNEL"] = settings.Splunk.Channel
		}
	}

	startAgentCmd := subprocess.NewRemoteCommand(
		remoteCmd,
		hostInfo.Hostname,
		hostObj.User,
		env,
		true, // background
		append([]string{"-p", hostInfo.Port}, sshOptions...),
		false, // loggingDisabled
	)
	cmdOutBuff := &bytes.Buffer{}
	output := subprocess.OutputOptions{Output: cmdOutBuff, SendErrorToOutput: true}
	if err = startAgentCmd.SetOutput(output); err != nil {
		grip.Alert(message.WrapError(err, message.Fields{
			"runner":    RunnerName,
			"operation": "setting up copy cli config command",
			"distro":    hostObj.Distro.Id,
			"host":      hostObj.Host,
			"output":    output,
			"cause":     "programmer error",
		}))

		return errors.Wrap(err, "problem configuring command output")
	}

	ctx, cancel := context.WithTimeout(context.TODO(), sshTimeout)
	defer cancel()
	err = startAgentCmd.Run(ctx)

	// run cleanup regardless of what happens.
	grip.Notice(message.WrapError(startAgentCmd.Stop(), message.Fields{
		"runner":  RunnerName,
		"message": "cleaning command failed",
	}))

	if err != nil {
		return errors.Wrapf(err, "error starting agent (%v): %v", hostObj.Id, cmdOutBuff.String())
	}

	event.LogHostAgentDeployed(hostObj.Id)

	return nil
}
