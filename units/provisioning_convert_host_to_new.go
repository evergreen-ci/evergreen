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

const (
	convertHostToNewProvisioningJobName = "convert-host-to-new-provisioning"
	maxProvisioningConversionAttempts   = 15
)

func init() {
	registry.AddJobType(convertHostToNewProvisioningJobName, func() amboy.Job {
		return makeConvertHostToNewProvisioningJob()
	})
}

type convertHostToNewProvisioningJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	env  evergreen.Environment
	host *host.Host
}

func makeConvertHostToNewProvisioningJob() *convertHostToNewProvisioningJob {
	j := &convertHostToNewProvisioningJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    convertHostToNewProvisioningJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewConvertHostToNewProvisioningJob converts a host from a legacy provisioned
// host to a non-legacy provisioned host.
func NewConvertHostToNewProvisioningJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeConvertHostToNewProvisioningJob()
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
	j.SetID(fmt.Sprintf("%s.%s.%s", convertHostToNewProvisioningJobName, j.HostID, id))
	return j
}

func (j *convertHostToNewProvisioningJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	if j.host.NeedsReprovision != host.ReprovisionToNew || (j.host.Status != evergreen.HostProvisioning && j.host.Status != evergreen.HostRunning) {
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
				"message":  "failed to convert host to new provisioning",
				"host_id":  j.host.Id,
				"host_tag": j.host.Tag,
				"distro":   j.host.Distro.Id,
				"provider": j.host.Provider,
			}))
		}
	}()

	// The host cannot be reprovisioned until the host's agent monitor has
	// stopped.
	if !j.host.NeedsNewAgentMonitor || j.host.RunningTask != "" {
		j.AddRetryableError(errors.Errorf("cannot reprovision host '%s' while the host's agent monitor is still running", j.host.Id))
		return
	}

	if err := j.host.UpdateLastCommunicated(ctx); err != nil {
		j.AddError(errors.Wrapf(err, "updating last communication time for host '%s'", j.host.Id))
	}

	defer func() {
		if j.HasErrors() {
			j.AddError(errors.Wrap(j.host.DeleteJasperCredentials(ctx, j.env), "deleting Jasper credentials after job errored"))
		}
	}()

	if err := setupJasper(ctx, j.env, j.env.Settings(), j.host); err != nil {
		j.AddRetryableError(err)
		return
	}

	if err := j.host.MarkAsReprovisioned(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	event.LogHostConvertedProvisioning(j.host.Id, j.host.Distro.BootstrapSettings.Method)

	grip.Info(message.Fields{
		"message": "successfully converted host from legacy to non-legacy provisioning",
		"host_id": j.host.Id,
		"distro":  j.host.Distro.Id,
		"job":     j.ID(),
	})
}

func (j *convertHostToNewProvisioningJob) populateIfUnset(ctx context.Context) error {
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
