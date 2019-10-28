package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	jcli "github.com/mongodb/jasper/cli"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

const (
	jasperDeployJobName    = "jasper-deploy"
	expirationCutoff       = 7 * 24 * time.Hour // 1 week
	jasperDeployRetryLimit = 50
)

func init() {
	registry.AddJobType(jasperDeployJobName, func() amboy.Job { return makeJasperDeployJob() })
}

type jasperDeployJob struct {
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID                string    `bson:"host_id" json:"host_id" yaml:"host_id"`
	CredentialsExpiration time.Time `bson:"credentials_expiration" json:"credentials_expiration" yaml:"credentials_expiration"`
	// If set, the deploy will be done by sending requests to the Jasper
	// service. Otherwise, the commands will be run as regular SSH commands
	// without communicating with Jasper at all.
	DeployThroughJasper bool `bson:"deploy_through_jasper" json:"deploy_through_jasper" yaml:"deploy_through_jasper"`

	env      evergreen.Environment
	settings *evergreen.Settings
	host     *host.Host
}

func makeJasperDeployJob() *jasperDeployJob {
	j := &jasperDeployJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    jasperDeployJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewJasperDeployJob creates a job that deploys a new Jasper service with new
// credentials to a host currently running a Jasper service.
func NewJasperDeployJob(env evergreen.Environment, h *host.Host, expiration time.Time, deployThroughJasper bool, id string) amboy.Job {
	j := makeJasperDeployJob()
	j.env = env
	j.settings = env.Settings()
	j.HostID = h.Id
	j.host = h
	j.CredentialsExpiration = expiration
	j.DeployThroughJasper = deployThroughJasper
	jobID := fmt.Sprintf("%s.%s.%s", jasperDeployJobName, j.HostID, id)
	if deployThroughJasper {
		jobID += ".deploy-through-jasper"
	}
	j.SetID(jobID)
	return j
}

// Run deploys new Jasper credentials to a host.
func (j *jasperDeployJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not populate required fields",
			"host":    j.HostID,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	defer func() {
		if j.HasErrors() {
			if err := j.tryRequeueDeploy(ctx); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":  "could not requeue Jasper deploy job",
					"host":     j.host.Distro.Id,
					"distro:":  j.host.Distro.Id,
					"expires":  j.CredentialsExpiration,
					"attempts": j.host.JasperDeployAttempts,
					"job":      j.ID(),
				}))
				return
			}
		}
	}()

	// Update LCT to prevent other jobs from trying to terminate this host,
	// which is about to kill the agent.
	if err := j.host.UpdateLastCommunicated(); err != nil {
		j.AddError(errors.Wrapf(err, "error setting LCT on host %s", j.host.Id))
	}

	grip.Info(message.Fields{
		"message": "deploying Jasper service to host",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
		"job":     j.ID(),
	})

	creds, err := j.host.GenerateJasperCredentials(ctx, j.env)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem generating new Jasper credentials",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	writeCredentialsCmd, err := j.host.WriteJasperCredentialsFilesCommands(j.settings.Splunk, creds)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not build command to write Jasper credentials file",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	if j.DeployThroughJasper {
		client, err := j.host.JasperClient(ctx, j.env)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get Jasper client",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
		defer client.CloseConnection()

		// We use this ID to later verify the current running Jasper service.
		// When Jasper is redeployed, its ID should be different to indicate it
		// is a new Jasper service.
		serviceID := client.ID()

		writeCredentialsOpts := &options.Create{
			Args: []string{
				"bash", "-c", writeCredentialsCmd,
			},
		}
		var output string
		if output, err = j.host.RunJasperProcess(ctx, j.env, writeCredentialsOpts); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not replace existing Jasper credentials on host",
				"logs":    output,
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		// We have to kill the Jasper service from within a process that it
		// creates so that the system restarts the service with the new
		// credentials file. This will not work on Windows.
		restartJasperOpts := &options.Create{
			Args: []string{
				"bash", "-c",
				fmt.Sprintf("pgrep -f '%s' | xargs kill", strings.Join(jcli.BuildServiceCommand(j.settings.HostJasper.BinaryName), " ")),
			},
		}

		if err = j.host.StartJasperProcess(ctx, j.env, restartJasperOpts); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not restart Jasper service",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		// Verify that the ID of the service has changed to indicate that the
		// service restarted.
		if client, err = j.host.JasperClient(ctx, j.env); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get Jasper client",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
		defer client.CloseConnection()

		newServiceID := client.ID()
		if newServiceID == "" {
			err := errors.New("new service ID returned empty")
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "new service ID should be non-empty",
				"host":      j.host.Id,
				"distro":    j.host.Distro.Id,
				"jasper_id": serviceID,
				"job":       j.ID(),
			}))
			j.AddError(err)
			return
		}
		if newServiceID == serviceID {
			err := errors.New("new service ID should not match current ID")
			grip.Error(message.WrapError(err, message.Fields{
				"message":   "Jasper service should have been restarted, but service is still showing same ID",
				"host":      j.host.Id,
				"distro":    j.host.Distro.Id,
				"jasper_id": serviceID,
				"job":       j.ID(),
			}))
			j.AddError(err)
			return
		}
	} else {
		sshOpts, err := j.host.GetSSHOptions(j.settings)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not get SSH options",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		if output, err := j.host.RunSSHCommand(ctx, writeCredentialsCmd, sshOpts); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run SSH command to write credentials file",
				"output":  output,
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}

		if output, err := j.host.RunSSHCommand(ctx, j.host.RestartJasperCommand(j.settings.HostJasper), sshOpts); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run SSH command to restart Jasper",
				"output":  output,
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
	}

	// We can only save the Jasper credentials with the new expiration once we
	// have reasonable confidence that the host has a Jasper service running
	// with the new credentials and the agent monitor will be deployed.
	if err := j.host.SaveJasperCredentials(ctx, j.env, creds); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "problem saving new Jasper credentials",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	grip.Info(message.Fields{
		"message": "deployed Jasper service with new credentials",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
		"version": j.settings.HostJasper.Version,
	})
	event.LogHostJasperDeployed(j.host.Id, j.settings.HostJasper.Version)

	// If job succeeded, reset jasper deploy count.
	if err := j.host.ResetJasperDeployAttempts(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not reset Jasper deploy attempts",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
	}

	// Set NeedsNewAgentMonitor to true to make the agent monitor deploy job
	// run.
	if err := j.host.SetNeedsNewAgentMonitor(true); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host as needing new agent monitor",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return
	}

	if j.host.RunningTask != "" {
		grip.Error(message.WrapError(model.ClearAndResetStrandedTask(j.host), message.Fields{
			"message": "could not clear stranded task",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"task":    j.host.RunningTask,
		}))
	}
}

// credentialsExpireBefore returns whether or not the host's Jasper credentials
// expire before the given cutoff.
func (j *jasperDeployJob) credentialsExpireBefore(cutoff time.Duration) bool {
	return time.Now().Add(cutoff).After(j.CredentialsExpiration)
}

// populateIfUnset populates the unset job fields.
func (j *jasperDeployJob) populateIfUnset(ctx context.Context) error {
	if j.host == nil {
		host, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		j.host = host
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.settings == nil {
		j.settings = j.env.Settings()
	}

	if j.CredentialsExpiration.IsZero() {
		expiration, err := j.host.JasperCredentialsExpiration(ctx, j.env)
		if err != nil {
			return errors.Wrapf(err, "could not get credentials expiration time for host %s in job %s", j.HostID, j.ID())
		}
		j.CredentialsExpiration = expiration
	}

	return nil
}

// tryRequeueDeploy attempts to requeue the deploy. If DeployThroughJasper is
// set, it tries to requeue the deploy with Jasper again. However, if it cannot
// do so on the next try, it instead requeues the deploy without using Jasper
// (i.e. SSH).
func (j *jasperDeployJob) tryRequeueDeploy(ctx context.Context) error {
	if err := j.host.IncJasperDeployAttempts(); err != nil {
		return errors.Wrap(err, "could not increment Jasper deploy attempt")
	}

	if j.DeployThroughJasper && j.canRetryDeploy() && !j.credentialsExpireBefore(time.Hour) {
		return errors.Wrap(j.requeueDeployThroughJasper(ctx, true), "could not requeue job with deploy through Jasper")
	}

	if j.DeployThroughJasper {
		if err := j.host.ResetJasperDeployAttempts(); err != nil {
			return errors.Wrap(err, "could not reset Jasper deploy attempts")
		}
	}

	if j.canRetryDeploy() {
		return errors.Wrap(j.requeueDeployThroughJasper(ctx, false), "could not requeue job without deploy through Jasper")
	}

	return errors.New("no more Jasper deploy attempts remaining")
}

func (j *jasperDeployJob) requeueDeployThroughJasper(ctx context.Context, deployThroughJasper bool) error {
	job := NewJasperDeployJob(j.env, j.host, j.CredentialsExpiration, deployThroughJasper, fmt.Sprintf("attempt-%d", j.host.JasperDeployAttempts))
	job.UpdateTimeInfo(amboy.JobTimeInfo{
		WaitUntil: time.Now().Add(time.Minute),
	})

	if err := j.env.RemoteQueue().Put(ctx, job); err != nil {
		return err
	}

	return nil
}

func (j *jasperDeployJob) canRetryDeploy() bool {
	return j.host.JasperDeployAttempts < jasperDeployRetryLimit
}
