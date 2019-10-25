package units

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper/options"
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
	}
	if disabled {
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
			"distro":  j.host.Distro.Id,
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

			if j.host.Status != evergreen.HostRunning {
				return
			}

			var noRetries bool
			noRetries, err = j.checkNoRetries()
			if err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "could not check whether host can retry agent monitor deploy",
					"host":    j.host.Id,
					"distro":  j.host.Distro.Id,
					"job":     j.ID(),
				}))
				j.AddError(err)
			} else if noRetries {
				var externallyTerminated bool
				externallyTerminated, err = handleExternallyTerminatedHost(ctx, j.ID(), j.env, j.host)
				j.AddError(errors.Wrapf(err, "can't check if host %s was externally terminated", j.host.Id))
				if externallyTerminated {
					return
				}

				if err = j.disableHost(ctx, fmt.Sprintf("failed %d times to put agent monitor on host", agentMonitorPutRetries)); err != nil {
					j.AddError(errors.Wrapf(err, "error marking host %s for termination", j.host.Id))
					return
				}
			}
			if err = j.host.SetNeedsNewAgentMonitor(true); err != nil {
				grip.Info(message.WrapError(err, message.Fields{
					"message": "problem setting needs new agent monitor flag to true",
					"distro":  j.host.Distro.Id,
					"host":    j.host.Id,
					"job":     j.ID(),
				}))
			}
		}
	}()

	settings := j.env.Settings()

	if err = j.fetchClient(ctx, settings); err != nil {
		j.AddError(err)
		return
	}

	if err = j.runSetupScript(ctx, settings); err != nil {
		j.AddError(err)
		return
	}

	j.AddError(j.startAgentMonitor(ctx, settings))
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
		"distro":  j.host.Distro.Id,
	}))

	return nil
}

// checkNoRetries checks if the job has exhausted the maximum allowed attempts
// to deploy the agent monitor.
func (j *agentMonitorDeployJob) checkNoRetries() (bool, error) {
	stat, err := event.GetRecentAgentMonitorDeployStatuses(j.host.Id, agentMonitorPutRetries)
	if err != nil {
		return false, errors.Wrap(err, "could not get recent agent monitor deploy statuses")
	}

	return stat.LastAttemptFailed() && stat.AllAttemptsFailed() && stat.Count >= agentMonitorPutRetries, nil
}

// fetchClient fetches the client on the host through the host's Jasper service.
func (j *agentMonitorDeployJob) fetchClient(ctx context.Context, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"message":       "fetching latest evergreen binary for agent monitor",
		"host":          j.host.Id,
		"distro":        j.host.Distro.Id,
		"communication": j.host.Distro.BootstrapSettings.Communication,
		"job":           j.ID(),
	})

	opts := &options.Create{
		Args: []string{filepath.Join(j.host.Distro.BootstrapSettings.RootDir, j.host.Distro.BootstrapSettings.ShellPath), "-l", "-c", j.host.CurlCommand(settings)},
	}
	output, err := j.host.RunJasperProcess(ctx, j.env, opts)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":       "error fetching agent monitor binary on host",
			"host":          j.host.Id,
			"distro":        j.host.Distro.Id,
			"output":        output,
			"communication": j.host.Distro.BootstrapSettings.Communication,
			"job":           j.ID(),
		}))
		return errors.WithStack(err)
	}

	return nil
}

// runSetupScript runs the setup script on the host through the host's Jasper
// service.
func (j *agentMonitorDeployJob) runSetupScript(ctx context.Context, settings *evergreen.Settings) error {
	grip.Info(message.Fields{
		"message":       "running setup script on host",
		"host":          j.host.Id,
		"distro":        j.host.Distro.Id,
		"communication": j.host.Distro.BootstrapSettings.Communication,
		"job":           j.ID(),
	})

	opts := &options.Create{
		Args: []string{filepath.Join(j.host.Distro.BootstrapSettings.RootDir, j.host.Distro.BootstrapSettings.ShellPath), "-l", "-c", j.host.SetupCommand()},
	}
	output, err := j.host.RunJasperProcess(ctx, j.env, opts)
	if err != nil {
		reason := "error running setup script on host"
		grip.Error(message.WrapError(err, message.Fields{
			"message": reason,
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"logs":    output,
			"job":     j.ID(),
		}))
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(err, "%s: %s", reason, output)
		catcher.Wrap(j.disableHost(ctx, reason), "failed to disable host after setup script failed")
		return catcher.Resolve()
	}

	return nil
}

// startAgentMonitor starts the agent monitor on the host through the host's
// Jasper service.
func (j *agentMonitorDeployJob) startAgentMonitor(ctx context.Context, settings *evergreen.Settings) error {
	// Generate the host secret if none exists.
	if j.host.Secret == "" {
		if err := j.host.CreateSecret(); err != nil {
			return errors.Wrapf(err, "creating secret for %s", j.host.Id)
		}
	}

	grip.Info(j.deployMessage())
	if err := j.host.StartJasperProcess(ctx, j.env, j.host.AgentMonitorOptions(settings)); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to start agent monitor on host",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return errors.Wrap(err, "failed to create command")
	}

	event.LogHostAgentMonitorDeployed(j.host.Id)

	return nil
}

// deployMessage builds the message containing information preceding an agent
// monitor deploy.
func (j *agentMonitorDeployJob) deployMessage() message.Fields {
	m := message.Fields{
		"message":  "starting agent monitor on host",
		"host":     j.host.Host,
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
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		if h == nil {
			return errors.Errorf("could not find host %s for job %s", j.HostID, j.ID())
		}
		j.host = h
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	return nil
}
