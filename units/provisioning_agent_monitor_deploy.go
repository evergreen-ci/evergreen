package units

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/credentials"
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
	"github.com/mongodb/jasper/rpc"
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

	host     *host.Host
	env      evergreen.Environment
	settings *evergreen.Settings
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

	if err = j.populateIfUnset(); err != nil {
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
				grip.Error(message.WrapError(err, message.Fields{
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

		if err := j.sshRunSetupScript(ctx, sshOpts); err != nil {
			j.AddError(err)
			return
		}

		if err := j.sshStartAgentMonitor(ctx, sshOpts); err != nil {
			j.AddError(err)
			return
		}
	} else if j.host.Distro.CommunicationMethod == distro.CommunicationMethodRPC {
		client, err := j.rpcClient(ctx)
		if err != nil {
			j.AddError(err)
			return
		}

		if err := j.rpcFetchClient(ctx, client); err != nil {
			j.AddError(err)
			return
		}

		if err := j.rpcRunSetupScript(ctx, client); err != nil {
			j.AddError(err)
			return
		}

		if err := j.rpcStartAgentMonitor(ctx, client); err != nil {
			j.AddError(err)
			return
		}
	}

	event.LogHostAgentMonitorDeployed(j.host.Id)
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

	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	return nil
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

	job := NewDecoHostNotifyJob(j.env, j.host, nil, reason)
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

func (j *agentMonitorDeployJob) clientURL() string {
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(j.settings.Ui.Url, "/"), j.settings.ClientBinariesDir, j.host.Distro.ExecutableSubPath())
}

func (j *agentMonitorDeployJob) clientPath() string {
	return filepath.Join(j.host.Distro.ClientDir, j.host.Distro.BinaryName())
}

// agentMonitorArgs returns the slice of arguments used to start the agent
// monitor.
func (j *agentMonitorDeployJob) agentMonitorArgs() []string {
	binary := filepath.Join("~", j.host.Distro.BinaryName())

	return []string{
		binary,
		"agent",
		fmt.Sprintf("--api_server='%s'", j.settings.ApiUrl),
		fmt.Sprintf("--host_id='%s'", j.host.Id),
		fmt.Sprintf("--host_secret='%s'", j.host.Secret),
		fmt.Sprintf("--log_prefix='%s'", filepath.Join(j.host.Distro.WorkDir, "agent")),
		fmt.Sprintf("--working_directory='%s'", j.host.Distro.WorkDir),
		fmt.Sprintf("--logkeeper_url='%s'", j.settings.LoggerConfig.LogkeeperURL),
		"--cleanup",
		"monitor",
		fmt.Sprintf("--client_url='%s'", j.clientURL()),
		fmt.Sprintf("--client_path='%s'", j.clientPath()),
	}
}

// agentEnv returns the agent environment variables.
func (j *agentMonitorDeployJob) agentEnv(settings *evergreen.Settings) map[string]string {
	env := map[string]string{
		"S3_KEY":    j.settings.Providers.AWS.S3Key,
		"S3_SECRET": j.settings.Providers.AWS.S3Secret,
		"S3_BUCKET": j.settings.Providers.AWS.Bucket,
	}

	if sumoEndpoint, ok := j.settings.Credentials["sumologic"]; ok {
		env["GRIP_SUMO_ENDPOINT"] = sumoEndpoint
	}

	if j.settings.Splunk.Populated() {
		env["GRIP_SPLUNK_SERVER_URL"] = j.settings.Splunk.ServerURL
		env["GRIP_SPLUNK_CLIENT_TOKEN"] = j.settings.Splunk.Token
		if j.settings.Splunk.Channel != "" {
			env["GRIP_SPLUNK_CHANNEL"] = j.settings.Splunk.Channel
		}
	}

	return env
}

// deployMessage builds the message containing information preceding an agent
// monitor deploy.
func (j *agentMonitorDeployJob) deployMessage() message.Fields {
	m := message.Fields{
		"message":  "starting agent monitor on host",
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

// fetchClientMessage builds the message containing information before the
// evergreen client is fetched on the host.
func (j *agentMonitorDeployJob) fetchClientMessage() message.Fields {
	return message.Fields{
		"message":              "fetching latest evergreen client version on host",
		"host":                 j.host.Id,
		"distro":               j.host.Distro.Id,
		"job":                  j.ID(),
		"jasper_communication": j.host.Distro.CommunicationMethod,
	}
}

// runSetupScriptMessage builds the message containing information before the
// setup script is run on the host.
func (j *agentMonitorDeployJob) runSetupScriptMessage() message.Fields {
	return message.Fields{
		"message":              "running setup script on host",
		"host":                 j.host.Id,
		"distro":               j.host.Distro.Id,
		"job":                  j.ID(),
		"jasper_communication": j.host.Distro.CommunicationMethod,
	}
}

// sshOptions gets this host's SSH options.
func (j *agentMonitorDeployJob) sshOptions(ctx context.Context) ([]string, error) {
	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.settings)
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
	grip.Info(j.fetchClientMessage())

	if logs, err := j.doSSHFetchClient(ctx, sshOpts); err != nil {
		return errors.Wrapf(err, "error downloading agent monitor binary on remote host: %s", logs)
	}

	return nil
}

// doSSHFetchClient fetches the client on the host over SSH.
func (j *agentMonitorDeployJob) doSSHFetchClient(ctx context.Context, sshOpts []string) (string, error) {
	cmd := j.host.CurlCommand(j.settings.Ui.Url)
	input := jaspercli.CommandInput{Commands: [][]string{[]string{cmd}}}

	output, err := j.host.RunSSHJasperRequest(ctx, j.env, jaspercli.CreateProcessCommand, input, sshOpts)
	if err != nil {
		return output, errors.Wrap(err, "problem creating command")
	}

	if _, err := jaspercli.ExtractOutcomeResponse([]byte(output)); err != nil {
		return output, errors.Wrap(err, "error in request outcome")
	}

	return output, nil
}

// sshRunSetupScript runs the setup script on the host over SSH.
func (j *agentMonitorDeployJob) sshRunSetupScript(ctx context.Context, sshOpts []string) error {
	grip.Info(j.runSetupScriptMessage())

	if j.host.Distro.Setup == "" {
		return nil
	}

	if logs, err := j.doSSHRunSetupScript(ctx, sshOpts); err != nil {
		event.LogProvisionFailed(j.host.Id, logs)

		grip.Error(message.WrapError(err, message.Fields{
			"message": "error running setup script on host over SSH",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    logs,
		}))

		// There is no guarantee setup scripts are idempotent, so we terminate
		// the host if the setup script fails.
		if err = j.disableHost(ctx, err.Error()); err != nil {
			return errors.Wrapf(err, "error marking host %s for termination", j.host.Id)
		}

		return err
	}

	return nil
}

// doSSHRunSetupScript runs the setup script on the host over SSH.
func (j *agentMonitorDeployJob) doSSHRunSetupScript(ctx context.Context, sshOpts []string) (string, error) {
	cmd := j.host.SetupCommand()
	input := jaspercli.CommandInput{Commands: [][]string{[]string{cmd}}}

	output, err := j.host.RunSSHJasperRequest(ctx, j.env, jaspercli.CreateCommand, input, sshOpts)
	if err != nil {
		return output, errors.Wrap(err, "problem creating command")
	}

	if _, err := jaspercli.ExtractOutcomeResponse([]byte(output)); err != nil {
		return output, errors.Wrap(err, "error in request outcome")
	}

	return output, nil
}

// sshStartAgentMonitor starts the agent monitor on the host over SSH.
func (j *agentMonitorDeployJob) sshStartAgentMonitor(ctx context.Context, sshOpts []string) error {
	if j.host.Secret == "" {
		if err := j.host.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", j.host.Id)
		}
	}

	grip.Info(j.deployMessage())
	output, err := j.host.RunSSHJasperRequest(ctx, j.env, jaspercli.CreateCommand, j.agentMonitorRequestInput(), sshOpts)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to start agent monitor on host over SSH",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
			"logs":    output,
		}))
		return errors.Wrap(err, "failed to run agent monitor deploy command")
	}

	return nil
}

// agentMonitorRequestInput assembles the input to a Jasper request to create
// the agent monitor.
func (j *agentMonitorDeployJob) agentMonitorRequestInput() jaspercli.CommandInput {
	input := jaspercli.CommandInput{
		Commands:      [][]string{j.agentMonitorArgs()},
		CreateOptions: jasper.CreateOptions{Environment: j.agentEnv(j.settings)},
		Background:    true,
	}
	return input
}

// rpcClient returns an RPC Jasper client with credentials.
func (j *agentMonitorDeployJob) rpcClient(ctx context.Context) (jasper.RemoteClient, error) {
	creds, err := credentials.ForJasperClient(ctx, j.env)
	if err != nil {
		return nil, errors.Wrap(err, "could not get Jasper client credentials")
	}

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", j.host.IP, j.settings.HostJasper.Port))
	if err != nil {
		return nil, errors.Wrap(err, "could not resolve Jasper service address")
	}

	client, err := rpc.NewClient(ctx, addr, creds)
	if err != nil {
		return nil, errors.Wrap(err, "problem initializing Jasper client")
	}

	return client, nil
}

// rpcFetchClient fetches the latest version of the client over RPC.
func (j *agentMonitorDeployJob) rpcFetchClient(ctx context.Context, client jasper.RemoteClient) error {
	grip.Info(j.fetchClientMessage())

	return errors.Wrap(client.DownloadFile(ctx, jasper.DownloadInfo{
		URL:  j.clientURL(),
		Path: j.clientPath(),
	}), "problem downloading evergreen client on host")
}

// rpcRunSetupScript runs the setup script on the host through the host's Jasper
// service over RPC.
func (j *agentMonitorDeployJob) rpcRunSetupScript(ctx context.Context, client jasper.RemoteClient) error {
	grip.Info(j.runSetupScriptMessage())

	if err := client.CreateCommand(ctx).Append(j.host.SetupCommand()).Run(ctx); err != nil {
		event.LogProvisionFailed(j.host.Id, err.Error())

		grip.Error(message.WrapError(err, message.Fields{
			"message": "error running setup script on host over RPC",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
		}))

		// There is no guarantee setup scripts are idempotent, so we terminate
		// the host if the setup script fails.
		if err = j.disableHost(ctx, err.Error()); err != nil {
			return errors.Wrapf(err, "error marking host %s for termination", err)
		}
		return err
	}

	return nil
}

// rpcStartAgentMonitor starts the agent monitor on the host over RPC.
func (j *agentMonitorDeployJob) rpcStartAgentMonitor(ctx context.Context, client jasper.RemoteClient) error {
	if j.host.Secret == "" {
		if err := j.host.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", j.host.Id)
		}
	}

	grip.Info(j.deployMessage())
	if err := client.CreateCommand(ctx).Add(j.agentMonitorArgs()).Run(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to start agent monitor on host via RPC",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return errors.Wrap(err, "failed to run agent monitor deploy command")
	}

	return nil
}
