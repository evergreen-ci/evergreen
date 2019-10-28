package units

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
			"host":    j.host.Id,
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
		if j.HasErrors() && j.host.Status == evergreen.HostRunning {
			var noRetries bool
			noRetries, err = j.checkNoRetries()
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "could not check whether host can retry agent deploy",
					"host":    j.host.Id,
					"distro":  j.host.Distro.Id,
					"job":     j.ID(),
				}))
				j.AddError(err)
			} else if noRetries {
				var externallyTerminated bool
				externallyTerminated, err = handleExternallyTerminatedHost(ctx, j.ID(), j.env, j.host)
				j.AddError(errors.Wrapf(err, "can't check if host '%s' was externally terminated", j.HostID))
				if externallyTerminated {
					return
				}

				if disableErr := j.host.DisablePoisonedHost(fmt.Sprintf("failed %d times to put agent on host", agentPutRetries)); disableErr != nil {
					j.AddError(errors.Wrapf(disableErr, "error terminating host %s", j.host.Id))
					return
				}

				job := NewDecoHostNotifyJob(j.env, j.host, nil, "error starting agent on host")
				grip.Error(message.WrapError(j.env.RemoteQueue().Put(ctx, job),
					message.Fields{
						"message": fmt.Sprintf("tried %d times to put agent on host", agentPutRetries),
						"host":    j.host.Id,
						"distro":  j.host.Distro,
					}))

				return
			}

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

	j.AddError(j.startAgentOnHost(ctx, settings, *j.host))
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
	sshOptions, err := hostObj.GetSSHOptions(settings)
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
		"host":    hostObj.Id,
		"job":     j.ID(),
	})
	if err = j.prepRemoteHost(ctx, hostObj, sshOptions, settings); err != nil {
		grip.Info(message.Fields{
			"message": "error prepping remote host",
			"host":    j.HostID,
			"job":     j.ID(),
			"error":   err.Error(),
		})
		return errors.Wrap(err, "could not prep remote host")
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
		return errors.Wrap(err, "could not start agent on remote")
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
	if logs, err := hostObj.RunSSHCommand(ctx, hostObj.CurlCommand(settings), sshOptions); err != nil {
		event.LogHostAgentDeployFailed(hostObj.Id, err)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error fetching agent",
			"host":    hostObj.Id,
			"job":     j.ID(),
			"logs":    logs,
		}))
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
			"runner":  "taskrunner",
			"host":    hostObj.Id,
			"distro":  hostObj.Distro.Id,
			"logs":    logs,
			"job":     j.ID(),
		}))

		// there is no guarantee setup scripts are idempotent, so we terminate the host if the setup script fails
		if disableErr := hostObj.DisablePoisonedHost(err.Error()); disableErr != nil {
			return errors.Wrapf(disableErr, "error terminating host %s", hostObj.Id)
		}

		job := NewDecoHostNotifyJob(j.env, j.host, nil, "error running setup script on host")
		grip.Error(message.WrapError(j.env.RemoteQueue().Put(ctx, job),
			message.Fields{
				"message": fmt.Sprintf("tried %d times to put agent on host", agentPutRetries),
				"host":    hostObj.Id,
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

	env["S3_KEY"] = settings.Providers.AWS.S3Key
	env["S3_SECRET"] = settings.Providers.AWS.S3Secret
	env["S3_BUCKET"] = settings.Providers.AWS.Bucket

	ctx, cancel := context.WithTimeout(ctx, sshTimeout)
	defer cancel()

	remoteCmd = fmt.Sprintf("nohup %s > /tmp/start 2>1 &", remoteCmd)

	startAgentCmd := j.env.JasperManager().CreateCommand(ctx).Environment(env).Append(remoteCmd).
		User(hostObj.User).Host(hostInfo.Hostname).ExtendRemoteArgs("-p", hostInfo.Port).ExtendRemoteArgs(sshOptions...)

	if err = startAgentCmd.Run(ctx); err != nil {
		return errors.Wrapf(err, "error starting agent (%v)", hostObj.Id)
	}

	event.LogHostAgentDeployed(hostObj.Id)

	return nil
}

func (j *agentDeployJob) checkNoRetries() (bool, error) {
	stat, err := event.GetRecentAgentDeployStatuses(j.HostID, agentPutRetries)
	if err != nil {
		return false, errors.Wrap(err, "could not get recent agent deploy statuses")
	}

	return stat.LastAttemptFailed() && stat.AllAttemptsFailed() && stat.Count >= agentPutRetries, nil
}
