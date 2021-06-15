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
	j.UpdateTimeInfo(amboy.JobTimeInfo{
		MaxTime: maxHostReprovisioningJobTime,
	})
	j.SetScopes([]string{reprovisioningJobScope(h.Id)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(15),
		WaitUntil:   utility.ToTimeDurationPtr(time.Minute),
	})
	j.SetID(fmt.Sprintf("%s.%s.%s", convertHostToNewProvisioningJobName, j.HostID, id))
	return j
}

func (j *convertHostToNewProvisioningJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(); err != nil {
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
			if j.RetryInfo().GetRemainingAttempts() == 0 && j.host.Provider == evergreen.ProviderNameStatic {
				j.AddError(j.host.SetStatusAtomically(evergreen.HostQuarantined, evergreen.User, "static host has run out of attempts to reprovision"))
			}

			event.LogHostConvertingProvisioningError(j.host.Id, j.Error())
		}
	}()

	// The host cannot be reprovisioned until the host's agent monitor has
	// stopped.
	if !j.host.NeedsNewAgentMonitor || j.host.RunningTask != "" {
		j.AddRetryableError(errors.New("cannot reprovision the host while the host's agent monitor is still running"))
		return
	}

	if err := j.host.UpdateLastCommunicated(); err != nil {
		j.AddError(errors.Wrap(err, "updating host last communication time"))
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

	if err := j.host.MarkAsReprovisioned(); err != nil {
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

func (j *convertHostToNewProvisioningJob) populateIfUnset() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			return errors.Wrapf(err, "could not find host '%s'", j.HostID)
		}
		j.host = h
	}

	return nil
}
