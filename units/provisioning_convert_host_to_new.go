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
	job.Base       `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID         string `bson:"host_id" json:"host_id" yaml:"host_id"`
	CurrentAttempt int    `bson:"current_attempt" json:"current_attempt" yaml:"current_attempt"`

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
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: host.MaxLCTInterval,
	})
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewConvertHostToNewProvisioningJob converts a host from a legacy provisioned
// host to a non-legacy provisioned host.
func NewConvertHostToNewProvisioningJob(env evergreen.Environment, h host.Host, id string, attempt int) amboy.Job {
	j := makeConvertHostToNewProvisioningJob()
	j.env = env
	j.host = &h
	j.HostID = h.Id
	j.CurrentAttempt = attempt
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", convertHostToNewProvisioningJobName, j.HostID, id))
	return j
}

func (j *convertHostToNewProvisioningJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
		j.AddError(err)
		return
	}

	if j.host.NeedsReprovision != host.ReprovisionToNew || (j.host.Status != evergreen.HostProvisioning && j.host.Status != evergreen.HostRunning) {
		return
	}

	// The host cannot be reprovisioned until the host's agent monitor has
	// stopped.
	if !j.host.NeedsNewAgentMonitor {
		grip.Error(message.WrapError(j.tryRequeue(ctx), message.Fields{
			"message": "could not enqueue job to retry provisioning conversion when host's agent monitor is still running",
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
		if err := j.host.SetReprovisioningLocked(false); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not clear host reprovisioning lock",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
			j.AddError(err)
		}
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

	// Update LCT to prevent other provisioning jobs from running.
	if err := j.host.UpdateLastCommunicated(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not update host communication time",
			"host":    j.host.Id,
			"distro":  j.host.Distro.Id,
			"job":     j.ID(),
		}))
	}

	defer func() {
		if j.HasErrors() {
			grip.Error(message.WrapError(j.host.DeleteJasperCredentials(ctx, j.env), message.Fields{
				"message": "could not delete Jasper credentials after failed provision attempt",
				"host":    j.host.Id,
				"distro":  j.host.Distro.Id,
				"job":     j.ID(),
			}))
		}
	}()

	if err := setupJasper(ctx, j.env, j.env.Settings(), j.host); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not set up Jasper service",
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
		"message": "successfully converted host from legacy to non-legacy provisioning",
		"host":    j.host.Id,
		"distro":  j.host.Distro.Id,
		"job":     j.ID(),
	})
}

func (j *convertHostToNewProvisioningJob) populateIfUnset() error {
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

func (j *convertHostToNewProvisioningJob) tryRequeue(ctx context.Context) error {
	if j.CurrentAttempt >= maxProvisioningConversionAttempts {
		return errors.New("exceeded max provisioning conversion attempts")
	}
	ts := util.RoundPartOfMinute(0).Format(TSFormat)
	retryJob := NewConvertHostToNewProvisioningJob(j.env, *j.host, fmt.Sprintf("%s.attempt-%d", ts, j.CurrentAttempt+1), j.CurrentAttempt+1)
	retryJob.UpdateTimeInfo(amboy.JobTimeInfo{
		WaitUntil: time.Now().Add(time.Minute),
	})
	return j.env.RemoteQueue().Put(ctx, retryJob)
}
