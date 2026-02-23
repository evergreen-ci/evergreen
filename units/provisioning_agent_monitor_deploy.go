package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	agentMonitorDeployJobName    = "agent-monitor-deploy"
	maxAgentMonitorDeployJobTime = 10 * time.Minute

	staticHostCurlNumRetries = 5
	staticHostCurlMaxSecs    = 300
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
	return j
}

// NewAgentMonitorDeployJob creates a job that deploys the agent monitor to the
// host.
func NewAgentMonitorDeployJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeAgentMonitorDeployJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetScopes([]string{fmt.Sprintf("%s.%s", agentMonitorDeployJobName, j.HostID)})
	j.SetEnqueueAllScopes(true)
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxAgentMonitorDeployJobTime,
	})
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(j.getRetriesForHost(h)),
		WaitUntil:   utility.ToTimeDurationPtr(15 * time.Second),
	})
	j.SetID(fmt.Sprintf("%s.%s.%s", agentMonitorDeployJobName, j.HostID, id))

	return j
}

// getRetriesForHost returns the number of times to retry the agent monitor deploy job.
// Should return a large number if the host is static, otherwise we shouldn't need as many retries
// (and many retries may indicate we're in a bad state).
func (j *agentMonitorDeployJob) getRetriesForHost(h host.Host) int {
	const (
		agentMonitorStaticHostRetries = 25
		agentMonitorDefaultRetries    = 1
	)

	if h.Provider == evergreen.ProviderNameStatic {
		return agentMonitorStaticHostRetries
	}
	return agentMonitorDefaultRetries
}

func (j *agentMonitorDeployJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "getting service flags"))
		return
	}
	if flags.AgentStartDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"host_id":  j.HostID,
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if err = j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
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

	if !j.host.NeedsNewAgentMonitor {
		return
	}

	if err = j.host.UpdateLastCommunicated(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not update host communication time",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
	}

	defer func() {
		if !j.HasErrors() {
			return
		}
		event.LogHostAgentMonitorDeployFailed(ctx, j.host.Id, j.Error())
		grip.Error(message.WrapError(j.Error(), message.Fields{
			"message":  "agent monitor deploy failed",
			"host_id":  j.host.Id,
			"host_tag": j.host.Tag,
			"distro":   j.host.Distro.Id,
			"provider": j.host.Provider,
		}))

		if j.host.Status != evergreen.HostRunning {
			return
		}
		if !j.IsLastAttempt() {
			return
		}

		var externallyTerminated bool
		externallyTerminated, err = handleExternallyTerminatedHost(ctx, j.ID(), j.env, j.host)
		j.AddError(errors.Wrapf(err, "checking if host '%s' was externally terminated", j.HostID))
		if externallyTerminated {
			return
		}

		if disableErr := HandlePoisonedHost(ctx, j.env, j.host, fmt.Sprintf("failed %d times to put agent monitor on host", j.RetryInfo().MaxAttempts)); disableErr != nil {
			j.AddError(errors.Wrapf(disableErr, "terminating poisoned host '%s'", j.host.Id))
		}
	}()

	var alive bool
	alive, err = j.checkAgentMonitor(ctx)
	if err != nil {
		j.AddRetryableError(err)
		return
	}
	if alive {
		grip.Info(message.Fields{
			"message": "not deploying a new agent monitor because it is still alive",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job_id":  j.ID(),
		})
		j.AddRetryableError(j.host.SetNeedsNewAgentMonitor(ctx, false))
		return
	}

	settings := *j.env.Settings()
	settings.ServiceFlags = *flags

	if err = j.fetchClient(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	if err = j.runSetupScript(ctx); err != nil {
		j.AddError(err)
		return
	}

	if err := j.startAgentMonitor(ctx, &settings); err != nil {
		j.AddRetryableError(err)
		return
	}

	j.AddError(j.host.SetNeedsNewAgentMonitor(ctx, false))
}

// hostDown checks if the host is down.
func (j *agentMonitorDeployJob) hostDown() bool {
	return utility.StringSliceContains(evergreen.DownHostStatus, j.host.Status)
}

// disableHost changes the host so that it is down and enqueues a job to
// terminate it.
func (j *agentMonitorDeployJob) disableHost(ctx context.Context, reason string) error {
	return errors.Wrapf(HandlePoisonedHost(ctx, j.env, j.host, reason), "terminating host '%s'", j.host.Id)
}

// checkAgentMonitor returns whether or not an agent monitor is already running
// on the host.
func (j *agentMonitorDeployJob) checkAgentMonitor(ctx context.Context) (bool, error) {
	var alive bool
	err := j.host.WithAgentMonitor(ctx, j.env, func(procs []jasper.Process) error {
		var numRunning int
		for _, proc := range procs {
			if proc.Running(ctx) {
				alive = true
				numRunning++
			}
		}
		grip.WarningWhen(numRunning > 1, message.Fields{
			"message": fmt.Sprintf("host should be running at most one agent monitor, but found %d", len(procs)),
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		})

		return nil
	})
	return alive, errors.Wrap(err, "checking agent monitor status")
}

// fetchClient fetches the client on the host through the host's Jasper service.
func (j *agentMonitorDeployJob) fetchClient(ctx context.Context) error {
	grip.Info(message.Fields{
		"message":       "fetching latest Evergreen binary for agent monitor",
		"host_id":       j.host.Id,
		"distro":        j.host.Distro.Id,
		"communication": j.host.Distro.BootstrapSettings.Communication,
		"job":           j.ID(),
	})

	var cancel context.CancelFunc
	var cmd string
	var err error
	if j.host.Provider == evergreen.ProviderNameStatic {
		cmd, err = j.host.CurlCommandWithRetry(j.env, staticHostCurlNumRetries, staticHostCurlMaxSecs)
		if err != nil {
			return errors.Wrap(err, "creating command to curl agent monitor binary")
		}
		ctx, cancel = context.WithTimeout(ctx, evergreenStaticHostCurlTimeout)
		defer cancel()
	} else {
		cmd, err = j.host.CurlCommand(j.env)
		if err != nil {
			return errors.Wrap(err, "creating command to curl agent monitor binary")
		}
		ctx, cancel = context.WithTimeout(ctx, evergreenCurlTimeout)
		defer cancel()
	}

	opts := &options.Create{
		Args: []string{j.host.Distro.ShellBinary(), "-l", "-c", cmd},
	}

	output, err := j.host.RunJasperProcess(ctx, j.env, opts)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error fetching agent monitor binary on host",
			"host_id":       j.host.Id,
			"distro":        j.host.Distro.Id,
			"logs":          output,
			"communication": j.host.Distro.BootstrapSettings.Communication,
			"job":           j.ID(),
		}))
		return errors.WithStack(err)
	}
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "curling agent monitor binary")
	}

	return nil
}

// runSetupScript runs the setup script on the host through the host's Jasper
// service.
func (j *agentMonitorDeployJob) runSetupScript(ctx context.Context) error {
	if j.host.Distro.Setup == "" {
		return nil
	}

	grip.Info(message.Fields{
		"message":       "running setup script on host",
		"host_id":       j.host.Id,
		"distro":        j.host.Distro.Id,
		"communication": j.host.Distro.BootstrapSettings.Communication,
		"job":           j.ID(),
	})

	opts := &options.Create{
		Args: []string{j.host.Distro.ShellBinary(), "-l", "-c", j.host.SetupCommand()},
	}
	output, err := j.host.RunJasperProcess(ctx, j.env, opts)
	if err != nil {
		reason := "running setup script on host"
		grip.Error(message.WrapError(err, message.Fields{
			"message": reason,
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    output,
			"job":     j.ID(),
		}))
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(err, "%s: %s", reason, output)
		catcher.Wrap(j.disableHost(ctx, reason), "disabling host after setup script failed")
		return catcher.Resolve()
	}

	return nil
}

// startAgentMonitor starts the agent monitor on the host through the host's
// Jasper service.
func (j *agentMonitorDeployJob) startAgentMonitor(ctx context.Context, settings *evergreen.Settings) error {
	// Generate the host secret if none exists.
	if j.host.Secret == "" {
		if err := j.host.CreateSecret(ctx, false); err != nil {
			return errors.Wrapf(err, "creating secret for host '%s'", j.host.Id)
		}
	}

	grip.Info(j.deployMessage())
	if _, err := j.host.StartJasperProcess(ctx, j.env, j.host.AgentMonitorOptions(settings)); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to start agent monitor on host",
			"host_id": j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return errors.Wrap(err, "creating agent monitor command")
	}

	event.LogHostAgentMonitorDeployed(ctx, j.host.Id)
	grip.Info(message.Fields{
		"message":       "agent monitor deployed",
		"host_id":       j.host.Id,
		"host_tag":      j.host.Tag,
		"distro":        j.host.Distro.Id,
		"provider":      j.host.Provider,
		"is_fresh_host": j.host.LastTask == "",
		"attempt_num":   j.RetryInfo().CurrentAttempt,
	})

	return nil
}

// deployMessage builds the message containing information preceding an agent
// monitor deploy.
func (j *agentMonitorDeployJob) deployMessage() message.Fields {
	m := message.Fields{
		"message":  "starting agent monitor on host",
		"host_id":  j.host.Host,
		"distro":   j.host.Distro.Id,
		"provider": j.host.Distro.Provider,
		"job":      j.ID(),
	}

	if j.host.InstanceType != "" {
		m["instance"] = j.host.InstanceType
	}

	sinceLCT := time.Since(j.host.LastCommunicationTime)
	if j.host.NeedsNewAgentMonitor {
		m["reason"] = "flagged for new agent monitor"
	} else if j.host.LastCommunicationTime.IsZero() {
		m["reason"] = "new host"
	} else if sinceLCT > host.MaxAgentMonitorUnresponsiveInterval {
		m["reason"] = "host has exceeded last communication threshold"
		m["threshold"] = host.MaxAgentMonitorUnresponsiveInterval
		m["threshold_span"] = host.MaxAgentMonitorUnresponsiveInterval.String()
		m["last_communication_at"] = sinceLCT
		m["last_communication_at_time"] = sinceLCT.String()
	}

	return m
}

// populateIfUnset populates the unset job fields.
func (j *agentMonitorDeployJob) populateIfUnset(ctx context.Context) error {
	if j.host == nil {
		h, err := host.FindOneId(ctx, j.HostID)
		if err != nil {
			return errors.Wrapf(err, "finding host '%s'", j.HostID)
		}
		if h == nil {
			return errors.Errorf("host '%s' not found", j.HostID)
		}
		j.host = h
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	return nil
}
