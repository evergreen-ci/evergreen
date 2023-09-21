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
	spawnHostStopRetryLimit = 10
	spawnhostStopName       = "spawnhost-stop"
)

func init() {
	registry.AddJobType(spawnhostStopName, func() amboy.Job {
		return makeSpawnhostStopJob()
	})
}

type spawnhostStopJob struct {
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeSpawnhostStopJob() *spawnhostStopJob {
	j := &spawnhostStopJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostStopName,
				Version: 0,
			},
		},
	}
	return j
}

// NewSpawnhostStopJob returns a job to stop a running spawn host.
func NewSpawnhostStopJob(h *host.Host, user, ts string) amboy.Job {
	j := makeSpawnhostStopJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStopName, user, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = h.Id
	j.CloudHostModification.UserID = user
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(spawnHostStopRetryLimit),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *spawnhostStopJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	stopCloudHost := func(mgr cloud.Manager, h *host.Host, user string) error {
		if err := mgr.StopInstance(ctx, h, user); err != nil {
			event.LogHostStopError(h.Id, err.Error())
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error stopping spawn host",
				"host_id":  h.Id,
				"host_tag": h.Tag,
				"distro":   h.Distro.Id,
				"user":     user,
			}))
			return errors.Wrap(err, "stopping spawn host")
		}

		event.LogHostStopSucceeded(h.Id)
		grip.Info(message.Fields{
			"message":  "stopped spawn host",
			"host_id":  h.Id,
			"host_tag": h.Tag,
			"distro":   h.Distro.Id,
			"user":     user,
		})

		return nil
	}
	if err := j.CloudHostModification.modifyHost(ctx, stopCloudHost); err != nil {
		j.AddError(err)
		return
	}
}
