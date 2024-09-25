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
	"github.com/pkg/errors"
)

const convertHostToLegacyProvisioningJobName = "convert-host-to-legacy-provisioning"

func init() {
	registry.AddJobType(convertHostToLegacyProvisioningJobName, func() amboy.Job {
		return makeConvertHostToLegacyProvisioningJob()
	})
}

type convertHostToLegacyProvisioningJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

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
	return j
}

// NewConvertHostToLegacyProvisioningJob converts a host from a legacy provisioned
// host to a non-legacy provisioned host.
func NewConvertHostToLegacyProvisioningJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeConvertHostToLegacyProvisioningJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxHostReprovisioningJobTime,
	})
	j.SetScopes([]string{reprovisioningJobScope(h.Id)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(maxProvisioningConversionAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(time.Minute),
	})
	j.SetID(fmt.Sprintf("%s.%s.%s", convertHostToLegacyProvisioningJobName, j.HostID, id))
	return j
}

func (j *convertHostToLegacyProvisioningJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	if j.host.NeedsReprovision != host.ReprovisionToLegacy || (j.host.Status != evergreen.HostProvisioning && j.host.Status != evergreen.HostRunning) || j.host.RunningTask != "" {
		return
	}

	defer func() {
		if j.HasErrors() {
			// Static hosts should be quarantined if they've run out of attempts
			// to reprovision.
			if j.IsLastAttempt() && j.host.Provider == evergreen.ProviderNameStatic {
				if err := DisableAndNotifyPoisonedHost(ctx, j.env, j.host, false, "static host has run out of attempts to reprovision"); err != nil {
					j.AddError(errors.Wrap(err, "quarantining static host that could not reprovision"))
				}
			}

			event.LogHostConvertingProvisioningError(j.host.Id, j.Error())
			grip.Info(message.WrapError(j.Error(), message.Fields{
				"message":  "failed to convert host to legacy provisioning",
				"host_id":  j.host.Id,
				"host_tag": j.host.Tag,
				"distro":   j.host.Distro.Id,
				"provider": j.host.Provider,
			}))
		}
	}()

	// The host cannot be reprovisioned until the host's agent has
	// stopped.
	if !j.host.NeedsNewAgent || j.host.RunningTask != "" {
		j.AddRetryableError(errors.Errorf("cannot reprovision host '%s' while the host's agent monitor is still running", j.host.Id))
		return
	}

	if err := j.host.UpdateLastCommunicated(ctx); err != nil {
		j.AddError(errors.Wrapf(err, "updating last communication time for host '%s'", j.host.Id))
	}

	settings := j.env.Settings()
	// This is a best-effort attempt to uninstall Jasper, but it will silently
	// fail to uninstall Jasper if the expected Jasper binary path does not
	// match its actual path on the remote host.
	if logs, err := j.host.RunSSHCommand(ctx, fmt.Sprintf("[ -a \"%s\" ] && %s", j.host.JasperBinaryFilePath(settings.HostJasper), j.host.QuietUninstallJasperCommand(settings.HostJasper))); err != nil {
		j.AddRetryableError(errors.Wrapf(err, "uninstalling Jasper service: %s", logs))
		return
	}

	if err := j.host.DeleteJasperCredentials(ctx, j.env); err != nil {
		j.AddRetryableError(errors.Wrap(err, "deleting Jasper credentials"))
		return
	}

	if err := j.host.MarkAsReprovisioned(ctx); err != nil {
		j.AddRetryableError(errors.Wrap(err, "marking host as reprovisioned"))
		return
	}

	event.LogHostConvertedProvisioning(j.host.Id, j.host.Distro.BootstrapSettings.Method)

	grip.Info(message.Fields{
		"message": "successfully converted host from non-legacy to legacy provisioning",
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"job":     j.ID(),
	})
}

func (j *convertHostToLegacyProvisioningJob) populateIfUnset(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

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

	return nil
}
