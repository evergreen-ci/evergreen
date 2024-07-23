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

// SpawnHostModifyJobOptions represents common options for creating spawn host
// modification jobs.
type SpawnHostModifyJobOptions struct {
	Host      *host.Host                      `bson:"-" json:"-" yaml:"-"`
	Source    evergreen.ModifySpawnHostSource `bson:"-" json:"-" yaml:"-"`
	User      string                          `bson:"-" json:"-" yaml:"-"`
	Timestamp string                          `bson:"-" json:"-" yaml:"-"`
	WaitUntil time.Time                       `bson:"-" json:"-" yaml:"-"`
}

// NewSpawnhostStopJob returns a job to stop a running spawn host.
// kim: TODO: refactor into common options
func NewSpawnhostStopJob(opts SpawnHostModifyJobOptions, shouldKeepOff bool) amboy.Job {
	j := makeSpawnhostStopJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", spawnhostStopName, opts.User, opts.Host.Id, opts.Timestamp))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", spawnHostStatusChangeScopeName, opts.Host.Id)})
	j.SetEnqueueAllScopes(true)
	j.CloudHostModification.HostID = opts.Host.Id
	j.CloudHostModification.UserID = opts.User
	j.ShouldKeepOff = shouldKeepOff
	j.CloudHostModification.Source = opts.Source
	// kim: TODO: check if Amboy will behave fine if I set the zero time here.
	j.SetTimeInfo(amboy.JobTimeInfo{
		WaitUntil: opts.WaitUntil,
	})
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(spawnHostStopRetryLimit),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

func (j *spawnhostStopJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.HasErrors() && j.IsLastAttempt() {
			// Only log an error if the final job attempt errors. Otherwise, it
			// may retry and succeed on the next attempt.
			event.LogHostStopError(j.HostID, string(j.Source), j.Error().Error())
		}
	}()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "getting service flags"))
		return
	}
	if j.Source == evergreen.ModifySpawnHostSleepSchedule && flags.SleepScheduleDisabled {
		grip.Notice(message.Fields{
			"message": "no-oping scheduled stop because sleep schedule service flag is disabled",
			"host_id": j.HostID,
			"user":    j.UserID,
			"job":     j.ID(),
		})
		return
	}

	stopCloudHost := func(ctx context.Context, mgr cloud.Manager, h *host.Host, user string) error {
		if j.Source == evergreen.ModifySpawnHostSleepSchedule && !h.IsSleepScheduleEnabled() {
			grip.Info(message.Fields{
				"message":             "no-oping scheduled stop because sleep schedule is not enabled for this host",
				"host_id":             j.HostID,
				"host_status":         h.Status,
				"host_sleep_schedule": h.SleepSchedule,
				"user":                j.UserID,
				"job":                 j.ID(),
			})
			return nil
		}
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
			return errors.Wrapf(err, "stopping spawn host '%s'", j.HostID)
		}

		event.LogHostStopSucceeded(h.Id, string(j.Source))
		grip.Info(message.Fields{
			"message":    "stopped spawn host",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
			"host_tag":   h.Tag,
			"distro":     h.Distro.Id,
			"source":     j.Source,
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
	if err := h.SetNextScheduledStopTime(ctx, nextStop); err != nil {
		return errors.Wrapf(err, "setting next scheduled stop to '%s'", nextStop)
	}
	return nil
}
