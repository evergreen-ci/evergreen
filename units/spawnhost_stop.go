package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
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
	spawnHostStopRetryLimit = 3
	spawnhostStopName       = "spawnhost-stop"
)

func init() {
	registry.AddJobType(spawnhostStopName, func() amboy.Job {
		return makeSpawnhostStopJob()
	})
}

type spawnhostStopJob struct {
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	ShouldKeepOff         bool `bson:"should_keep_off" json:"should_keep_off" yaml:"should_keep_off"`
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
func NewSpawnhostStopJob(h *host.Host, shouldKeepOff bool, source evergreen.ModifySpawnHostSource, user, ts string) amboy.Job {
	j := makeSpawnhostStopJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStopName, user, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = h.Id
	j.CloudHostModification.UserID = user
	j.ShouldKeepOff = shouldKeepOff
	j.CloudHostModification.Source = source
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(spawnHostStopRetryLimit),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *spawnhostStopJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// kim: TODO: check isSleepScheduleEnabledQuery here to ensure it's still
	// enabled before stopping a host for the sleep schedule. Same for the spawn
	// host start job.
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "getting service flags"))
		return
	}
	if j.Source == evergreen.ModifySpawnHostSleepSchedule && flags.SleepScheduleDisabled {
		// kim: TODO: test
		grip.Notice(message.Fields{
			"message": "no-oping scheduled stop because sleep schedule is disabled",
			"host_id": j.HostID,
			"user":    j.UserID,
			"job":     j.ID(),
		})
		return
	}

	stopCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		if j.Source == evergreen.ModifySpawnHostSleepSchedule && h.SleepSchedule.NextStopTime.After(time.Now()) {
			grip.Info(message.Fields{
				"message":        "no-oping because host is not scheduled to stop yet",
				"host_id":        h.Id,
				"next_stop_time": h.SleepSchedule.NextStopTime,
				"job":            j.ID(),
			})
			return nil
		}

		if err := mgr.StopInstance(ctx, h, j.ShouldKeepOff, user); err != nil {
			event.LogHostStopError(h.Id, err.Error())
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error stopping spawn host",
				"host_id":  h.Id,
				"host_tag": h.Tag,
				"distro":   h.Distro.Id,
				"job":      j.ID(),
			}))
			return errors.Wrap(err, "stopping spawn host")
		}

		event.LogHostStopSucceeded(h.Id)
		grip.Info(message.Fields{
			"message":    "stopped spawn host",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
			"host_tag":   h.Tag,
			"distro":     h.Distro.Id,
			"job":        j.ID(),
		})

		if j.Source == evergreen.ModifySpawnHostSleepSchedule {
			grip.Warning(message.WrapError(j.setNextScheduledStop(ctx, h), message.Fields{
				"message":        "successfully stopped host for sleep schedule but could not set next scheduled stop time",
				"host_id":        h.Id,
				"started_by":     h.StartedBy,
				"sleep_schedule": fmt.Sprintf("%#v", h.SleepSchedule),
				"job":            j.ID(),
			}))
		}

		return nil
	}

	if err := j.CloudHostModification.modifyHost(ctx, stopCloudHost); err != nil {
		j.AddRetryableError(err)
		return
	}
}

func (j *spawnhostStopJob) setNextScheduledStop(ctx context.Context, h *host.Host) error {
	if j.Source != evergreen.ModifySpawnHostSleepSchedule {
		return nil
	}
	scheduleAfter := time.Now()
	nextStop, err := h.SleepSchedule.GetNextScheduledStopTime(scheduleAfter)
	if err != nil {
		return errors.Wrap(err, "calculating next scheduled stop time")
	}
	if err := h.SetNextScheduledStop(ctx, nextStop); err != nil {
		return errors.Wrapf(err, "setting next scheduled stop to '%s'", nextStop)
	}
	return nil
}
