package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/cloud"
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
	spawnHostRebootRetryLimit = 3
	spawnhostRebootName       = "spawnhost-reboot"
)

func init() {
	registry.AddJobType(spawnhostRebootName, func() amboy.Job {
		return makeSpawnhostRebootJob()
	})
}

type spawnhostRebootJob struct {
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeSpawnhostRebootJob() *spawnhostRebootJob {
	j := &spawnhostRebootJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostRebootName,
				Version: 0,
			},
		},
	}
	return j
}

// NewSpawnhostRebootJob returns a job to reboot a spawn host.
func NewSpawnhostRebootJob(opts SpawnHostModifyJobOptions) amboy.Job {
	j := makeSpawnhostRebootJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostRebootName, opts.User, opts.Host.Id, opts.Timestamp))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, opts.Host.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = opts.Host.Id
	j.CloudHostModification.UserID = opts.User
	j.CloudHostModification.Source = opts.Source
	j.SetTimeInfo(amboy.JobTimeInfo{
		WaitUntil: opts.WaitUntil,
	})
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(spawnHostRebootRetryLimit),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *spawnhostRebootJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.HasErrors() && j.IsLastAttempt() {
			// Only log an error if the final job attempt errors. Otherwise, it
			// may retry and succeed on the next attempt.
			event.LogHostRebootError(ctx, j.HostID, string(j.Source), j.Error().Error())
			grip.Error(message.WrapError(j.Error(), message.Fields{
				"message": "no attempts remaining to reboot spawn host",
				"host_id": j.HostID,
				"source":  j.Source,
				"job":     j.ID(),
			}))
		}
	}()

	rebootCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		if err := mgr.RebootInstance(ctx, h, user); err != nil {
			return errors.Wrapf(err, "rebooting spawn host '%s'", j.HostID)
		}
		event.LogHostRebootSucceeded(ctx, h.Id, string(j.Source))
		grip.Info(message.Fields{
			"message":    "rebooted spawn host",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
			"host_tag":   h.Tag,
			"distro":     h.Distro.Id,
			"source":     j.Source,
			"job":        j.ID(),
		})
		return nil
	}

	if err := j.CloudHostModification.modifyHost(ctx, rebootCloudHost); err != nil {
		j.AddRetryableError(err)
		return
	}
}
