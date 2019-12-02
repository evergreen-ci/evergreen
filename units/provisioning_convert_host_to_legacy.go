package units

import (
	"context"
	"fmt"
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
	"github.com/pkg/errors"
)

const convertHostToLegacyProvisioningJobName = "convert-host-to-legacy-provisioning"

func init() {
	registry.AddJobType(convertHostToLegacyProvisioningJobName, func() amboy.Job {
		return makeConvertHostToLegacyProvisioningJob()
	})
}

type convertHostToLegacyProvisioningJob struct {
	job.Base       `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID         string `bson:"host_id" json:"host_id" yaml:"host_id"`
	CurrentAttempt int    `bson:"current_attempt" json:"current_attempt" yaml:"current_attempt"`

	env  evergreen.Environment
	host *host.Host
}

func makeConvertHostToLegacyProvisioningJob() *convertHostToLegacyProvisioningJob {
	j := &convertHostToLegacyProvisioningJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    convertHostToLegacyProvisioningJobName,
				Version: 0,
			},
		},
	}
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: host.MaxLCTInterval,
	})
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewConvertHostToLegacyProvisioningJob converts a host from a legacy provisioned
// host to a non-legacy provisioned host.
func NewConvertHostToLegacyProvisioningJob(env evergreen.Environment, h host.Host, id string, attempt int) amboy.Job {
	j := makeConvertHostToLegacyProvisioningJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.CurrentAttempt = attempt
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", convertHostToLegacyProvisioningJobName, j.HostID, id))
	return j
}

func (j *convertHostToLegacyProvisioningJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.host.NeedsReprovision != host.ReprovisionToLegacy || (j.host.Status != evergreen.HostProvisioning && j.host.Status != evergreen.HostRunning) {
		return
	}

	// The host cannot be reprovisioned until the host's agent has
	// stopped.
	if !j.host.NeedsNewAgent {
		grip.Error(message.WrapError(j.tryRequeue(ctx), message.Fields{
			"message": "could not enqueue job to retry provisioning conversion when host's agent is still running",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return
	}

	defer func() {
		if j.HasErrors() {
			event.LogHostConvertingProvisioningError(j.host.Id, j.Error())

			if err := j.tryRequeue(ctx); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message": "could not enqueue job to retry provisioning conversion",
					"host":    j.host.Id,
					"distro":  j.host.Distro.Id,
					"job":     j.ID(),
				}))
				j.AddError(err)
			}
		}
		grip.Error(message.WrapError(j.host.SetReprovisioningLocked(false), message.Fields{
			"message": "could not clear host reprovisioning lock",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
	}()

	// Lock the provisioning state to prevent other provisioning jobs from
	// running.
	if err := j.host.SetReprovisioningLockedAtomically(true); err != nil {
		grip.Info(message.WrapError(err, message.Fields{
			"message": "reprovisioning job currently in progress, returning from job",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		return
	}

	if err := j.host.UpdateLastCommunicated(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not update host communication time",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
	}

	settings := j.env.Settings()
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
	// This is a best-effort attempt to uninstall Jasper, but it will silently
	// fail to uninstall Jasper if the Jasper binary path does not match its
	// actual path on the remote host.
	if logs, err := j.host.RunSSHCommandLiterally(ctx, fmt.Sprintf("[ -a \"%s\" ] && %s", j.host.JasperBinaryFilePath(settings.HostJasper), j.host.QuietUninstallJasperCommand(settings.HostJasper)), sshOpts); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not uninstall Jasper service",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
			"logs":    logs,
		}))
		j.AddError(err)
		return
	}

	if err := j.host.DeleteJasperCredentials(ctx, j.env); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not delete Jasper credentials",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	if err := j.host.MarkAsReprovisioned(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not mark host as provisioned",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	event.LogHostConvertedProvisioning(j.host.Id, j.host.Distro.BootstrapSettings.Method)

	grip.Info(message.Fields{
		"message": "successfully converted host from non-legacy to legacy provisioning",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
		"job":     j.ID(),
	})
}

func (j *convertHostToLegacyProvisioningJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host %s for job %s", j.HostID, j.ID())
		}
		j.host = h
	}

	return nil
}

func (j *convertHostToLegacyProvisioningJob) tryRequeue(ctx context.Context) error {
	if j.CurrentAttempt >= maxProvisioningConversionAttempts {
		return errors.New("exceeded max provisioning conversion attempts")
	}
	ts := util.RoundPartOfMinute(0).Format(TSFormat)
	retryJob := NewConvertHostToLegacyProvisioningJob(j.env, *j.host, fmt.Sprintf("%s.attempt-%d", ts, j.CurrentAttempt+1), j.CurrentAttempt+1)
	retryJob.UpdateTimeInfo(amboy.JobTimeInfo{
		WaitUntil: time.Now().Add(time.Minute),
	})
	return j.env.RemoteQueue().Put(ctx, retryJob)
}
