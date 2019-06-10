package units

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/shlex"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	jaspercli "github.com/mongodb/jasper/cli"
	"github.com/pkg/errors"
)

const (
	agentDeployJobName = "agent-deploy"
	agentPutRetries    = 75
)

func init() {
	registry.AddJobType(agentDeployJobName, func() amboy.Job {
		return makeAgentDeployJob()
	})
}

type agentDeployJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeAgentDeployJob() *agentDeployJob {
	j := &agentDeployJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    agentDeployJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewAgentDeployJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeAgentDeployJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", agentDeployJobName, j.HostID, id))

	return j
}

func (j *agentDeployJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.AgentStartDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"host":     j.HostID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}
	if util.StringSliceContains(evergreen.DownHostStatus, j.host.Status) {
		grip.Debug(message.Fields{
			"host_id": j.host.Id,
			"status":  j.host.Status,
			"message": "host already down, not attempting to deploy agent",
		})
		return
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	// Occasionally an agent deploy job will be dispatched around the same time that
	// PopulateAgentDeployJobs creates another job. These jobs can race. An atomic update on the
	// needs new agent field will cause the job to fail early here. Updating the last
	// communicated time prevents the other branches of the PopulateAgentDeployJobs from
	// immediately applying. Instead MaxLCTInterval must pass.
	if err = j.host.SetNeedsNewAgentAtomically(false); err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"distro":  j.host.Distro,
			"host":    j.host.Id,
			"job":     j.ID(),
			"message": "needs new agent flag already false, not deploying new agent",
		}))
		return
	}
	if err = j.host.UpdateLastCommunicated(); err != nil {
		j.AddError(errors.Wrapf(err, "error setting LCT on host %s", j.host.Id))
	}
	defer func() {
		if j.HasErrors() {
			if err = j.host.SetNeedsNewAgent(true); err != nil {
				grip.Info(message.WrapError(err, message.Fields{
					"distro":  j.host.Distro,
					"host":    j.host.Id,
					"job":     j.ID(),
					"message": "problem setting needs agent flag to true",
				}))
			}
		}
	}()

	if j.host.Distro.BootstrapMethod == distro.BootstrapMethodSSH {
		j.AddError(j.startAgentMonitorOnHost(ctx))
	} else {
		j.AddError(j.startAgentOnHost(ctx, settings, *j.host))
	}

	stat, err := event.GetRecentAgentDeployStatuses(j.HostID, agentPutRetries)
	j.AddError(err)
	if err != nil {
		return
	}

	if stat.LastAttemptFailed() && stat.AllAttemptsFailed() && stat.Count >= agentPutRetries {
		if disableErr := j.host.DisablePoisonedHost(fmt.Sprintf("failed %d times to put agent on host", agentPutRetries)); disableErr != nil {
			j.AddError(errors.Wrapf(disableErr, "error terminating host %s", j.host.Id))
			return
		}

		job := NewDecoHostNotifyJob(j.env, j.host, nil, "error starting agent on host")
		grip.Error(message.WrapError(j.env.RemoteQueue().Put(ctx, job),
			message.Fields{
				"message": fmt.Sprintf("tried %d times to put agent on host", agentPutRetries),
				"host_id": j.host.Id,
				"distro":  j.host.Distro,
			}))
	}
}

// SSHTimeout defines the timeout for the SSH commands in this package.
const sshTimeout = 25 * time.Second

func (j *agentDeployJob) getHostMessage(h host.Host) message.Fields {
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
func (j *agentDeployJob) startAgentOnHost(ctx context.Context, settings *evergreen.Settings, hostObj host.Host) error {

	// get the host's SSH options
	cloudHost, err := cloud.GetCloudHost(ctx, &hostObj, settings)
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
	hostObj.Distro = d

	// prep the remote host
	grip.Info(message.Fields{
		"runner":  "taskrunner",
		"message": "prepping host for agent",
		"host":    hostObj.Id})
	if err = j.prepRemoteHost(ctx, hostObj, sshOptions, settings); err != nil {
		event.LogHostAgentDeployFailed(hostObj.Id, err)
		grip.Info(message.Fields{
			"message": "error prepping remote host",
			"host":    j.HostID,
			"job":     j.ID(),
			"error":   err.Error(),
		})
		return nil
	}

	grip.Info(message.Fields{"runner": "taskrunner", "message": "prepping host finished successfully", "host": hostObj.Id})

	// generate the host secret if none exists
	if hostObj.Secret == "" {
		if err = hostObj.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", hostObj.Id)
		}
	}

	// Start agent to listen for tasks
	grip.Info(j.getHostMessage(hostObj))
	if err = j.startAgentOnRemote(ctx, settings, &hostObj, sshOptions); err != nil {
		event.LogHostAgentDeployFailed(hostObj.Id, err)
		grip.Info(message.WrapError(err, message.Fields{
			"message": "error starting agent on remote",
			"host":    j.HostID,
			"job":     j.ID(),
		}))
		return nil
	}
	grip.Info(message.Fields{"runner": "taskrunner", "message": "agent successfully started for host", "host": hostObj.Id})

	if err = hostObj.SetAgentRevision(evergreen.BuildRevision); err != nil {
		return errors.Wrapf(err, "error setting agent revision on host %s", hostObj.Id)
	}
	return nil
}

// Prepare the remote machine to run a task.
func (j *agentDeployJob) prepRemoteHost(ctx context.Context, hostObj host.Host, sshOptions []string, settings *evergreen.Settings) error {
	// copy over the correct agent binary to the remote host
	if logs, err := hostObj.RunSSHCommand(ctx, hostObj.CurlCommand(settings.Ui.Url), sshOptions); err != nil {
		return errors.Wrapf(err, "error downloading agent binary on remote host: %s", logs)
	}

	// run the setup script with the agent
	if hostObj.Distro.Setup == "" {
		return nil
	}

	if logs, err := hostObj.RunSSHCommand(ctx, hostObj.SetupCommand(), sshOptions); err != nil {
		event.LogProvisionFailed(hostObj.Id, logs)

		grip.Error(message.WrapError(err, message.Fields{
			"message": "error running setup script",
			"host_id": hostObj.Id,
			"distro":  hostObj.Distro.Id,
			"runner":  "taskrunner",
			"logs":    logs,
		}))

		// there is no guarantee setup scripts are idempotent, so we terminate the host if the setup script fails
		if disableErr := hostObj.DisablePoisonedHost(err.Error()); disableErr != nil {
			return errors.Wrapf(disableErr, "error terminating host %s", hostObj.Id)
		}

		job := NewDecoHostNotifyJob(j.env, j.host, nil, "error running setup script on host")
		grip.Error(message.WrapError(j.env.RemoteQueue().Put(ctx, job),
			message.Fields{
				"message": fmt.Sprintf("tried %d times to put agent on host", agentPutRetries),
				"host_id": hostObj.Id,
				"distro":  hostObj.Distro,
			}))

		return errors.Wrapf(err, "error running setup script on remote host: %s", logs)
	}

	return nil
}

// Start the agent process on the specified remote host.
func (j *agentDeployJob) startAgentOnRemote(ctx context.Context, settings *evergreen.Settings, hostObj *host.Host, sshOptions []string) error {
	// the path to the agent binary on the remote machine
	pathToExecutable := filepath.Join("~", "evergreen")
	if hostObj.Distro.IsWindows() {
		pathToExecutable += ".exe"
	}

	agentCmdParts := []string{
		pathToExecutable,
		"agent",
		fmt.Sprintf("--api_server='%s'", settings.ApiUrl),
		fmt.Sprintf("--host_id='%s'", hostObj.Id),
		fmt.Sprintf("--host_secret='%s'", hostObj.Secret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(hostObj.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory='%s'", hostObj.Distro.WorkDir),
		fmt.Sprintf("--logkeeper_url='%s'", settings.LoggerConfig.LogkeeperURL),
		"--cleanup",
	}

	// build the command to run on the remote machine
	remoteCmd := strings.Join(agentCmdParts, " ")
	grip.Info(message.Fields{
		"message": "starting agent on host",
		"host":    hostObj.Id,
		"command": remoteCmd,
		"runner":  "taskrunner",
	})

	// compute any info necessary to ssh into the host
	hostInfo, err := hostObj.GetSSHInfo()
	if err != nil {
		return errors.Wrapf(err, "error parsing ssh info %v", hostObj.Host)
	}

	// run the command to kick off the agent remotely
	env := j.agentEnv(settings)

	ctx, cancel := context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	remoteCmd = fmt.Sprintf("nohup %s > /tmp/start 2>1 &", remoteCmd)

	startAgentCmd := j.env.JasperManager().CreateCommand(ctx).Environment(env).Append(remoteCmd).
		User(hostObj.User).Host(hostInfo.Hostname).ExtendSSHArgs("-p", hostInfo.Port).ExtendSSHArgs(sshOptions...)

	if err = startAgentCmd.Run(ctx); err != nil {
		return errors.Wrapf(err, "error starting agent (%v)", hostObj.Id)
	}

	event.LogHostAgentDeployed(hostObj.Id)

	return nil
}

func (j *agentDeployJob) startAgentMonitorOnHost(ctx context.Context) error {
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env.Settings())
	if err != nil {
		return errors.Wrapf(err, "Failed to get cloud host for %s", j.host.Id)
	}

	sshOptions, err := cloudHost.GetSSHOptions()
	if err != nil {
		return errors.Wrapf(err, "Error getting ssh options for host %s", j.host.Id)
	}

	d, err := distro.FindOne(distro.ById(j.host.Distro.Id))
	if err != nil {
		return errors.Wrapf(err, "error finding distro %s", j.host.Distro.Id)
	}
	j.host.Distro = d

	// prep the remote host
	grip.Info(message.Fields{
		"message": "prepping host for agent monitor",
		"host":    j.host.Id,
	})

	/* if err = j.prepRemoteHost(ctx, j.host, sshOptions, settings); err != nil {
	 *     event.LogHostAgentDeployFailed(j.host.Id, err)
	 *     grip.Info(message.Fields{
	 *         "message": "error prepping remote host",
	 *         "host":    j.HostID,
	 *         "job":     j.ID(),
	 *         "error":   err.Error(),
	 *     })
	 *     return nil
	 * } */

	if logs, err := j.jasperRunSetupScript(ctx, sshOptions); err != nil {
		event.LogProvisionFailed(j.host.Id, logs)
		errMsg := "error running setup script on host"
		msg := message.Fields{
			"message": errMsg,
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    logs,
		}
		grip.Error(message.WrapError(err, msg))

		// there is no guarantee setup scripts are idempotent, so we terminate the host if the setup script fails
		if err := j.host.DisablePoisonedHost(err.Error()); err != nil {
			return errors.Wrapf(err, "error terminating host %s", j.host.Id)
		}

		job := NewDecoHostNotifyJob(j.env, j.host, nil, errMsg)
		grip.Error(message.WrapError(j.env.RemoteQueue().Put(job), message.Fields{
			"message": fmt.Sprintf("tried %d times to start agent monitor on host", agentPutRetries),
			"host":    j.host.Id,
			"distro":  j.host.Distro,
		}))

		event.LogHostAgentMonitorDeployFailed(j.host.Id, err)
		return err
	}

	grip.Info(message.Fields{
		"message": "prepping host finished successfully",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
	})

	// generate the host secret if none exists
	if j.host.Secret == "" {
		if err = j.host.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", j.host.Id)
		}
	}

	// Start agent to listen for tasks
	/*     grip.Info(j.getHostMessage(j.host))
	 *     if err = j.startAgentOnRemote(ctx, settings, &j.host, sshOptions); err != nil {
	 *         event.LogHostAgentDeployFailed(j.host.Id, err)
	 *         grip.Info(message.WrapError(err, message.Fields{
	 *             "message": "error starting agent on remote",
	 *             "host":    j.HostID,
	 *             "job":     j.ID(),
	 *         }))
	 *         return nil
	 *     }
	 *     grip.Info(message.Fields{
	 *         "runner": "taskrunner",
	 *         "message": "agent successfully started for host",
	 *         "host": j.host.Id,
	 *     })
	 *
	 *     if err = j.host.SetAgentRevision(evergreen.BuildRevision); err != nil {
	 *         return errors.Wrapf(err, "error setting agent revision on host %s", j.host.Id)
	 *     }
	 *     return nil */

	// kim: in startAgentOnRemote():
	/*     // build the command to run on the remote machine
	 *     remoteCmd := strings.Join(agentCmdParts, " ")
	 *     grip.Info(message.Fields{
	 *         "message": "starting agent on host",
	 *         "host":    hostObj.Id,
	 *         "command": remoteCmd,
	 *         "runner":  "taskrunner",
	 *     })
	 *
	 *     // compute any info necessary to ssh into the host
	 *     hostInfo, err := hostObj.GetSSHInfo()
	 *     if err != nil {
	 *         return errors.Wrapf(err, "error parsing ssh info %v", hostObj.Host)
	 *     }
	 *
	 *     // run the command to kick off the agent remotely
	 *     env := map[string]string{}
	 *     if sumoEndpoint, ok := settings.Credentials["sumologic"]; ok {
	 *         env["GRIP_SUMO_ENDPOINT"] = sumoEndpoint
	 *     }
	 *
	 *     if settings.Splunk.Populated() {
	 *         env["GRIP_SPLUNK_SERVER_URL"] = settings.Splunk.ServerURL
	 *         env["GRIP_SPLUNK_CLIENT_TOKEN"] = settings.Splunk.Token
	 *
	 *         if settings.Splunk.Channel != "" {
	 *             env["GRIP_SPLUNK_CHANNEL"] = settings.Splunk.Channel
	 *         }
	 *     }
	 *
	 *     env["S3_KEY"] = settings.Providers.AWS.S3Key
	 *     env["S3_SECRET"] = settings.Providers.AWS.S3Secret
	 *     env["S3_BUCKET"] = settings.Providers.AWS.Bucket
	 *
	 *     ctx, cancel := context.WithTimeout(ctx, sshTimeout)
	 *     defer cancel()
	 *
	 *     remoteCmd = fmt.Sprintf("nohup %s > /tmp/start 2>1 &", remoteCmd)
	 *
	 *     startAgentCmd := j.env.JasperManager().CreateCommand(ctx).Environment(env).Append(remoteCmd).
	 *         User(hostObj.User).Host(hostInfo.Hostname).ExtendSSHArgs("-p", hostInfo.Port).ExtendSSHArgs(sshOptions...)
	 *
	 *     if err = startAgentCmd.Run(ctx); err != nil {
	 *         return errors.Wrapf(err, "error starting agent (%v)", hostObj.Id)
	 *     }
	 *
	 *     event.LogHostAgentDeployed(hostObj.Id)
	 *
	 *     return nil */

	// TODO: figure out how API server ensures there actually is an agent
	// running on the host. It seems like it doesn't.
	grip.Info(j.getHostMessage(*j.host))
	output, err := j.host.RunSSHJasperRequest(ctx, j.env, "create-command", j.agentMonitorRequestInput(), sshOptions)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to start agent monitor on host",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
			"logs":    output,
		}))
		return errors.Wrap(err, "failed to create command")
	}
	event.LogHostAgentMonitorDeployed(j.host.Id)

	return nil
}

func (j *agentDeployJob) jasperRunSetupScript(ctx context.Context, sshOptions []string) (string, error) {
	cmd, err := shlex.Split(j.host.SetupCommand())
	if err != nil {
		return "", errors.Wrap(err, "problem parsing setup command")
	}

	input := jasper.CreateOptions{Args: cmd}
	output, err := j.host.RunSSHJasperRequest(ctx, j.env, "create-process", input, sshOptions)
	if err != nil {
		return output, errors.Wrap(err, "failed to create command")
	}

	if _, err := jaspercli.ExtractOutcomeResponse(output); err != nil {
		return output, errors.Wrap(err, "error getting command outcome")
	}

	return output, nil
}

func (j *agentDeployJob) agentEnv(settings *evergreen.Settings) map[string]string {
	env := map[string]string{
		"S3_KEY":    settings.Providers.AWS.S3Key,
		"S3_SECRET": settings.Providers.AWS.S3Secret,
		"S3_BUCKET": settings.Providers.AWS.Bucket,
	}

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

	return env
}

// agentMonitorRequestInput assembles the input to a Jasper request to create
// the agent monitor.
func (j *agentDeployJob) agentMonitorRequestInput() jaspercli.CommandInput {
	settings := j.env.Settings()
	binary := filepath.Join("~", j.host.Distro.BinaryName())
	clientURL := fmt.Sprintf("%s/clients/%s", strings.TrimRight(settings.Ui.Url, "/"), j.host.Distro.ExecutableSubPath())

	agentMonitorParams := []string{
		binary,
		"agent",
		fmt.Sprintf("--api_server='%s'", settings.ApiUrl),
		fmt.Sprintf("--host_id='%s'", j.host.Id),
		fmt.Sprintf("--host_secret='%s'", j.host.Secret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(j.host.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory='%s'", j.host.Distro.WorkDir),
		fmt.Sprintf("--logkeeper_url='%s'", settings.LoggerConfig.LogkeeperURL),
		"--cleanup",
		"monitor",
		fmt.Sprintf("--client_url='%s'", clientURL),
		fmt.Sprintf("--client_path='%s'", "/usr/local/bin/evergreen"),
	}

	input := jaspercli.CommandInput{
		Commands:      [][]string{agentMonitorParams},
		CreateOptions: jasper.CreateOptions{Environment: j.agentEnv(settings)},
		Background:    true,
	}
	return input
}
