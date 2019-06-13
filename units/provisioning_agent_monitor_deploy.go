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
	agentMonitorDeployJobName = "agent-monitor-deploy"
	agentMonitorPutRetries    = 75
)

func init() {
	registry.AddJobType(agentMonitorDeployJobName, func() amboy.Job {
		return makeAgentMonitorDeployJob()
	})
}

type agentMonitorDeployJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeAgentMonitorDeployJob() *agentMonitorDeployJob {
	j := &agentMonitorDeployJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    agentMonitorDeployJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

// NewAgentMonitorDeployJob creates a job that deploys the agent monitor to the
// host.
func NewAgentMonitorDeployJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeAgentMonitorDeployJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", agentMonitorDeployJobName, j.HostID, id))

	return j
}

func (j *agentMonitorDeployJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	disabled, err := j.agentStartDisabled()
	if err != nil {
		j.AddError(err)
		return
	} else if disabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"host":     j.HostID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.hostDown() {
		grip.Debug(message.Fields{
			"host_id": j.host.Id,
			"status":  j.host.Status,
			"message": "host already down, not attempting to deploy agent monitor",
		})
		return
	}

	// An atomic update on the needs new agent monitor field will cause
	// concurrent jobs to fail early here. Updating the last communicated time
	// prevents PopulateAgentMonitorDeployJobs from immediately running unless
	// MaxLCTInterval passes.
	if err = j.host.SetNeedsNewAgentMonitorAtomically(false); err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"message": "needs new agent monitor flag is already false, not deploying new agent monitor",
			"distro":  j.host.Distro,
			"host":    j.host.Id,
			"job":     j.ID(),
		}))
		return
	}
	if err = j.host.UpdateLastCommunicated(); err != nil {
		j.AddError(errors.Wrapf(err, "error setting LCT on host %s", j.host.Id))
	}

	defer func() {
		if j.HasErrors() {
			event.LogHostAgentMonitorDeployFailed(j.host.Id, j.Error())

			noRetries, err := j.checkNoRetries()
			if err != nil {
				j.AddError(err)
			} else if noRetries {
				if err := j.disableHost(ctx, fmt.Sprintf("failed %d times to put agent monitor on host", agentMonitorPutRetries)); err != nil {
					j.AddError(errors.Wrapf(err, "error marking host %s for termination", j.host.Id))
					return
				}
			}
			if err := j.host.SetNeedsNewAgentMonitor(true); err != nil {
				grip.Info(message.WrapError(err, message.Fields{
					"message": "problem setting needs new agent monitor flag to true",
					"distro":  j.host.Distro,
					"host":    j.host.Id,
					"job":     j.ID(),
				}))
			}
		}
	}()

	if j.host.Distro.CommunicationMethod == distro.CommunicationMethodSSH {
		sshOpts, err := j.sshOptions(ctx)
		if err != nil {
			j.AddError(err)
			return
		}

		if err := j.sshFetchClient(ctx, sshOpts); err != nil {
			j.AddError(err)
			return
		}

		if err := j.sshRunSetup(ctx, sshOpts); err != nil {
			j.AddError(err)
			return
		}

		j.AddError(j.sshStartAgentMonitor(ctx, sshOpts))
	} else {
		// TODO: EVG-6232: allow agent monitor to be redeployed over RPC.
	}

}

// hostDown checks if the host is down.
func (j *agentMonitorDeployJob) hostDown() bool {
	return util.StringSliceContains(evergreen.DownHostStatus, j.host.Status)
}

// disableHost changes the host so that it is down and enqueues a job to
// terminate it.
func (j *agentMonitorDeployJob) disableHost(ctx context.Context, reason string) error {
	if err := j.host.DisablePoisonedHost(reason); err != nil {
		return errors.Wrapf(err, "error terminating host %s", j.host.Id)
	}

	job := NewDecoHostNotifyJob(j.env, j.host, nil, errMsg)
	grip.Error(message.WrapError(j.env.RemoteQueue().Put(ctx, job), message.Fields{
		"message": fmt.Sprintf("tried %d times to start agent monitor on host", agentMonitorPutRetries),
		"host":    j.host.Id,
		"distro":  j.host.Distro,
	}))

	return nil
}

// checkNoRetries checks if the job has exhausted the maximum allowed attempts
// to deploy the agent monitor.
func (j *agentMonitorDeployJob) checkNoRetries() (bool, error) {
	stat, err := event.GetRecentAgentMonitorDeployStatuses(j.HostID, agentMonitorPutRetries)
	if err != nil {
		return false, errors.Wrap(err, "could not get recent agent monitor deploy statuses")
	}

	return stat.LastAttemptFailed() && stat.AllAttemptsFailed() && stat.Count >= agentMonitorPutRetries, nil
}

// sshOptions gets this host's SSH options.
func (j *agentMonitorDeployJob) sshOptions(ctx context.Context) ([]string, error) {
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env.Settings())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get cloud host for %s", j.host.Id)
	}

	sshOpts, err := cloudHost.GetSSHOptions()
	if err != nil {
		return nil, errors.Wrapf(err, "error getting ssh options for host %s", j.host.Id)
	}
	return sshOpts, nil
}

// sshFetchClient gets the latest version of the client on the host over SSH.
func (j *agentMonitorDeployJob) sshFetchClient(ctx context.Context, sshOpts []string) error {
	grip.Info(message.Fields{
		"message": "fetching latest evergreen client version",
		"host":    j.host.Id,
	})

	if logs, err := j.fetchClient(ctx, sshOpts); err != nil {
		return errors.Wrapf(err, "error downloading agent monitor binary on remote host: %s", logs)
	}

	return nil
}

// fetchClient fetches the client on the host through the host's Jasper service.
func (j *agentMonitorDeployJob) fetchClient(ctx context.Context, sshOpts []string) (string, error) {
	settings := j.env.Settings()
	cmd := j.host.CurlCommand(settings.Ui.Url)
	input := jaspercli.CommandInput{Commands: [][]string{[]string{cmd}}}

	output, err := j.host.RunSSHJasperRequest(ctx, j.env, "create-process", input, sshOpts)
	if err != nil {
		return output, errors.Wrap(err, "problem creating command")
	}

	if _, err := jaspercli.ExtractOutcomeResponse([]byte(output)); err != nil {
		return output, errors.Wrap(err, "error in request outcome")
	}

	return output, nil
}

// sshRunSetup runs the setup script on the host over SSH.
func (j *agentMonitorDeployJob) sshRunSetup(ctx context.Context, sshOpts []string) error {
	grip.Info(message.Fields{
		"message": "running setup script",
		"host":    j.host.Id,
	})

	if logs, err := j.runSetupScript(ctx, sshOpts); err != nil {
		event.LogProvisionFailed(j.host.Id, logs)

		errMsg := "error running setup script on host"
		msg := message.Fields{
			"message": errMsg,
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    logs,
		}
		grip.Error(message.WrapError(err, msg))

		// There is no guarantee setup scripts are idempotent, so we terminate
		// the host if the setup script fails.
		if err = j.disableHost(ctx, err.Error()); err != nil {
			return errors.Wrapf(err, "error marking host %s for termination", j.host.Id)
		}

		return err
	}

	return nil
}

// runSetupScript runs the setup script on the host through the host's Jasper
// service.
func (j *agentMonitorDeployJob) runSetupScript(ctx context.Context, sshOpts []string) (string, error) {
	cmd := j.host.SetupCommand()
	input := jaspercli.CommandInput{Commands: [][]string{[]string{cmd}}}

	output, err := j.host.RunSSHJasperRequest(ctx, j.env, "create-command", input, sshOpts)
	if err != nil {
		return output, errors.Wrap(err, "problem creating command")
	}

	if _, err := jaspercli.ExtractOutcomeResponse([]byte(output)); err != nil {
		return output, errors.Wrap(err, "error in request outcome")
	}

	return output, nil
}

/// sshStartAgentMonitor starts the agent monitor on the host.
func (j *agentMonitorDeployJob) sshStartAgentMonitor(ctx context.Context, sshOpts []string) error {

	grip.Info(message.Fields{
		"message": "prepping host for agent monitor",
		"host":    j.host.Id,
	})

	grip.Info(message.Fields{
		"message": "prepping host finished successfully",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
	})

	// Generate the host secret if none exists.
	if j.host.Secret == "" {
		if err := j.host.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", j.host.Id)
		}
	}

	grip.Info(j.deployMessage())
	output, err := j.host.RunSSHJasperRequest(ctx, j.env, "create-command", j.agentMonitorRequestInput(), sshOpts)
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

// agentMonitorRequestInput assembles the input to a Jasper request to create
// the agent monitor.
func (j *agentMonitorDeployJob) agentMonitorRequestInput() jaspercli.CommandInput {
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
		fmt.Sprintf("--client_path='%s'", filepath.Join(j.host.Distro.ClientDir, j.host.Distro.BinaryName())),
	}

	input := jaspercli.CommandInput{
		Commands:      [][]string{agentMonitorParams},
		CreateOptions: jasper.CreateOptions{Environment: j.agentEnv(settings)},
		Background:    true,
	}
	return input
}

// agentEnv returns the agent environment variables.
func (j *agentMonitorDeployJob) agentEnv(settings *evergreen.Settings) map[string]string {
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

// deployMessage builds the message containing information preceding an agent
// monitor deploy.
func (j *agentMonitorDeployJob) deployMessage() message.Fields {
	m := message.Fields{
		"message":  "starting agent monitor on host",
		"runner":   "taskrunner",
		"host":     j.host.Host,
		"distro":   j.host.Distro.Id,
		"provider": j.host.Distro.Provider,
	}

	if j.host.InstanceType != "" {
		m["instance"] = j.host.InstanceType
	}

	sinceLCT := time.Since(j.host.LastCommunicationTime)
	if j.host.NeedsNewAgentMonitor {
		m["reason"] = "flagged for new agent monitor"
	} else if j.host.LastCommunicationTime.IsZero() {
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

// agentStartDisabled checks if the agent (and consequently, the agent monitor)
// should be deployed.
func (j *agentMonitorDeployJob) agentStartDisabled() (bool, error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return false, errors.New("could not get service flags")
	}

	return flags.AgentStartDisabled, nil
}

// populateIfUnset populates the unset job fields.
func (j *agentMonitorDeployJob) populateIfUnset() error {
	if j.host == nil {
		host, err := host.FindOneId(j.HostID)
		if err != nil || host == nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.TaskID)
		}
		j.host = host
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	return nil
}
