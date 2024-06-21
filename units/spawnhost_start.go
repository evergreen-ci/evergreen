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
	spawnHostStartRetryLimit = 3
	spawnhostStartName       = "spawnhost-start"
)

func init() {
	registry.AddJobType(spawnhostStartName, func() amboy.Job {
		return makeSpawnhostStartJob()
	})
}

type spawnhostStartJob struct {
	CloudHostModification `bson:"cloud_host_modification" json:"cloud_host_modification" yaml:"cloud_host_modification"`
	job.Base              `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func makeSpawnhostStartJob() *spawnhostStartJob {
	j := &spawnhostStartJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostStartName,
				Version: 0,
			},
		},
	}
	return j
}

// NewSpawnhostStartJob returns a job to start a stopped spawn host.
func NewSpawnhostStartJob(h *host.Host, source evergreen.ModifySpawnHostSource, user, ts string) amboy.Job {
	j := makeSpawnhostStartJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStartName, user, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = h.Id
	j.CloudHostModification.UserID = user
	j.CloudHostModification.Source = source
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(spawnHostStartRetryLimit),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *spawnhostStartJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.HasErrors() && j.IsLastAttempt() {
			// Only log an error if the final job attempt errors. Otherwise, it
			// may retry and succeed on the next attempt.
			event.LogHostStartError(j.HostID, string(j.Source), j.Error().Error())
		}
	}()

	startCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		if j.Source == evergreen.ModifySpawnHostSleepSchedule && !h.IsSleepScheduleEnabled() {
			grip.Info(message.Fields{
				"message":             "no-oping scheduled start because sleep schedule is not enabled for this host",
				"host_id":             j.HostID,
				"host_status":         h.Status,
				"host_sleep_schedule": h.SleepSchedule,
				"user":                j.UserID,
				"job":                 j.ID(),
			})
			return nil
		}
		if j.Source == evergreen.ModifySpawnHostSleepSchedule && h.SleepSchedule.NextStartTime.After(time.Now().Add(host.PreStartThreshold)) {
			grip.Info(message.Fields{
				"message":         "no-oping because host is not scheduled to start yet",
				"host_id":         h.Id,
				"next_start_time": h.SleepSchedule.NextStartTime,
				"job":             j.ID(),
			})
			return nil
		}

		if err := mgr.StartInstance(ctx, h, user); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "error starting spawn host",
				"host_id":  h.Id,
				"host_tag": h.Tag,
				"distro":   h.Distro.Id,
				"job":      j.ID(),
			}))
			return errors.Wrap(err, "starting spawn host")
		}

		event.LogHostStartSucceeded(h.Id, string(j.Source))
		grip.Info(message.Fields{
			"message":    "started spawn host",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
			"host_tag":   h.Tag,
			"distro":     h.Distro.Id,
			"source":     j.Source,
			"job":        j.ID(),
		})

		if j.Source == evergreen.ModifySpawnHostSleepSchedule {
			grip.Warning(message.WrapError(j.setNextScheduledStart(ctx, h), message.Fields{
				"message":        "successfully started host for sleep schedule but could not set next scheduled start time",
				"host_id":        h.Id,
				"sleep_schedule": fmt.Sprintf("%#v", h.SleepSchedule),
				"job":            j.ID(),
			}))
		}

		return nil
	}

	if err := j.CloudHostModification.modifyHost(ctx, startCloudHost); err != nil {
		j.AddRetryableError(err)
		return
	}
}

func (j *spawnhostStartJob) setNextScheduledStart(ctx context.Context, h *host.Host) error {
	if j.Source != evergreen.ModifySpawnHostSleepSchedule {
		return nil
	}
	// Since hosts are started in advance for their sleep schedule, ensure that
	// the next start time is after the pre-start threshold.
	scheduleAfter := time.Now().Add(host.PreStartThreshold)
	if h.SleepSchedule.NextStartTime.After(scheduleAfter) {
		scheduleAfter = h.SleepSchedule.NextStartTime
	}
	nextStart, err := h.SleepSchedule.GetNextScheduledStartTime(scheduleAfter)
	if err != nil {
		return errors.Wrap(err, "calculating next scheduled start")
	}
	if err := h.SetNextScheduledStartTime(ctx, nextStart); err != nil {
		return errors.Wrapf(err, "setting next scheduled start to '%s'", nextStart)
	}
	return nil
}
