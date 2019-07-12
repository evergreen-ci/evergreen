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
	"github.com/mongodb/jasper"
	jaspercli "github.com/mongodb/jasper/cli"
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
	// If set, the commands will be run as regular SSH commands without
	// communicating with Jasper at all.
	DeployThroughJasper bool `bson:"deploy_through_jasper" json:"deploy_through_jasper" yaml:"deploy_through_jasper"`

	env  evergreen.Environment
	host *host.Host
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
func NewJasperDeployJob(env evergreen.Environment, h host.Host, expiration time.Time, deployThroughJasper bool, id string) amboy.Job {
	j := makeJasperDeployJob()
	j.env = env
	j.host = &h
	j.SetID(fmt.Sprintf("%s.%s.%s", jasperDeployJobName, j.HostID, id))
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
			if j.DeployThroughJasper && j.shouldRetryDeploy() && !j.credentialsExpireBefore(time.Hour) {
				if err := j.requeueDeployThroughJasper(ctx); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message":  "could not requeue job with deploy through Jasper",
						"host":     j.host.Id,
						"distro":   j.host.Distro.Id,
						"expires":  j.CredentialsExpiration,
						"attempts": j.host.JasperDeployAttempts,
						"job":      j.ID(),
					}))
				}
				return
			}

			if j.DeployThroughJasper {
				if err := j.host.ResetJasperDeployAttempts(); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message":  "could not reset Jasper deploy attempts",
						"host":     j.host.Distro.Id,
						"distro:":  j.host.Distro.Id,
						"expires":  j.CredentialsExpiration,
						"attempts": j.host.JasperDeployAttempts,
						"job":      j.ID(),
					}))
					return
				}
			}

			if j.shouldRetryDeploy() {
				if err := j.requeueDeployThroughSSH(ctx); err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message":  "could not requeue job without deploy through Jasper",
						"host":     j.host.Distro.Id,
						"distro:":  j.host.Distro.Id,
						"expires":  j.CredentialsExpiration,
						"attempts": j.host.JasperDeployAttempts,
						"job":      j.ID(),
					}))
				}
				return
			}

			grip.Error(message.Fields{
				"message":  "no more Jasper deploy attempts remaining",
				"host":     j.host.Id,
				"distro":   j.host.Distro.Id,
				"expires":  j.CredentialsExpiration,
				"attempts": j.host.JasperDeployAttempts,
				"job":      j.ID(),
			})
		}
	}()

	settings := j.env.Settings()

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

	writeCredentialsCmd, err := j.host.WriteJasperCredentialsFileCommand(settings.HostJasper, creds)
	if err != nil {
		grip.Error(message.Fields{
			"message": "could not build command to write Jasper credentials file",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		})
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
		// We use this ID to verify the current running Jasper service. When
		// Jasper is redeployed, its ID should be different to indicate it is
		// a new Jasper service.
		serviceID := client.ID()

		writeCredentialsOpts := &jasper.CreateOptions{
			Args: []string{
				"bash", "-c", writeCredentialsCmd,
			},
		}
		if output, err := j.host.RunJasperProcess(ctx, j.env, writeCredentialsOpts); err != nil {
			grip.Error(message.Fields{
				"message": "could not replace existing Jasper credentials on host",
				"logs":    output,
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			})
			j.AddError(err)
			return
		}

		// We have to kill the Jasper service from within a process that it
		// creates so that the system restarts the service with the new
		// credentials file. This will not work on Windows.
		restartJasperOpts := &jasper.CreateOptions{
			Args: []string{
				"bash", "-c",
				fmt.Sprintf("pgrep -f '%s' | kill", strings.Join(jaspercli.BuildServiceCommand(settings.HostJasper.BinaryName), " ")),
			},
		}

		if err := j.host.StartJasperProcess(ctx, j.env, restartJasperOpts); err != nil {
			grip.Error(message.Fields{
				"message": "could not restart Jasper service",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			})
			j.AddError(err)
			return
		}

		// We have no means of directly checking that the Jasper service
		// actually restarted with the new credentials file, so the only sanity
		// check that we can perform to verify this is to check that either
		// 1. there are no processes or
		// 2. there are processes, but they have a different manager ID from the
		//    original Jasper service.
		// There is also no guarantee that we will be able to connect to Jasper
		// immediately after terminating it.
		// kim: TODO: wait for MAKE-854 merge

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
		} else if newServiceID == serviceID {
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
		/*         client, err := j.host.JasperClient(ctx, j.env)
		 *         if err != nil {
		 *         }
		 *
		 *         procs, err := client.List(ctx, jasper.All)
		 *         if err != nil {
		 *             grip.Error(message.WrapError(err, message.Fields{
		 *                 "message": "could not list running Jasper processes on host",
		 *                 "host":    j.host.Id,
		 *                 "distro":  j.host.Distro.Id,
		 *                 "id":      j.ID(),
		 *             }))
		 *             j.AddError(err)
		 *             return
		 *         }
		 *
		 *         // kim: TODO: wait for MAKE-854 merge.
		 *         if len(procs) != 0 {
		 *             managerIsNew := false
		 *             for _, proc := range procs {
		 *                 info := proc.Info(ctx)
		 *                 if len(info.Options.Environment) != 0 && info.Options.Environment[jasper.ManagerEnvironID] != "TODO" {
		 *                     managerIsNew = true
		 *                     break
		 *                 }
		 *             }
		 *             if !managerIsNew {
		 *                 err := errors.New("new Jasper process still shows old service is running")
		 *                 grip.Error(message.WrapError(err, message.Fields{
		 *                     "message": "Jasper service should have been started, but old one is still running",
		 *                     "host":    j.host.Id,
		 *                     "distro":  j.host.Distro.Id,
		 *                     "job":     j.ID(),
		 *                 }))
		 *                 j.AddError(err)
		 *                 return
		 *             }
		 *             return
		 *         } */
	} else {
		sshOpts, err := j.host.GetSSHOptions(settings)
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

		if output, err := j.host.RunSSHCommand(ctx, j.host.RestartJasperCommand(settings.HostJasper), sshOpts); err != nil {
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

	event.LogHostJasperDeployed(j.host.Id, settings.HostJasper.Version)

	// If job succeeded, reset jasper deploy count.
	if err := j.host.ResetJasperDeployAttempts(); err != nil {
		grip.Error(message.Fields{
			"message": "could not reset Jasper deploy attempts",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		})
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

	// Note: can't use pkill/pgrep because it doesn't return itself as a
	// process.
	// Call bash -c 'echo $PPID | xargs kill -9', or something equivalent.

	// Kill Jasper command:
	// Windows: "bash -c 'curator jasper service force-reinstall rpc'"
	// MacOS: bash -c "pgrep -f 'curator jasper service | xargs kill'
	// Linux: bash -c "echo $PPID | xargs kill" (maybe can just use same as
	// MacOS)
}

// func (j *jasperDeployJob) putNewCredentials(ctx context.Context) error {
//     return nil
// }

func (j *jasperDeployJob) requeueDeployThroughJasper(ctx context.Context) error {
	return j.requeue(ctx, true)
}

func (j *jasperDeployJob) requeueDeployThroughSSH(ctx context.Context) error {
	return j.requeue(ctx, false)
}

func (j *jasperDeployJob) requeue(ctx context.Context, deployThroughJasper bool) error {
	job := NewJasperDeployJob(j.env, *j.host, j.CredentialsExpiration, deployThroughJasper, fmt.Sprintf("attempt-%d", j.host.JasperDeployAttempts))
	job.UpdateTimeInfo(amboy.JobTimeInfo{
		WaitUntil: time.Now().Add(time.Minute),
	})

	if err := j.env.RemoteQueue().Put(ctx, job); err != nil {
		return err
	}

	return nil
}

func (j *jasperDeployJob) shouldRetryDeploy() bool {
	return j.HasErrors() && j.host.JasperDeployAttempts < jasperDeployRetryLimit
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

	if j.CredentialsExpiration.IsZero() {
		expiration, err := j.host.JasperCredentialsExpiration(ctx, j.env)
		if err != nil {
			return errors.Wrapf(err, "could not get credentials expiration time for host %s in job %s", j.HostID, j.ID())
		}
		j.CredentialsExpiration = expiration
	}

	return nil
}
