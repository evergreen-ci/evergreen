package units

import (
	"context"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	cacheHistoricalTaskDataName = "cache-historical-task-data"
	maxSyncDuration             = time.Hour * 24
)

func init() {
	registry.AddJobType(cacheHistoricalTaskDataName,
		func() amboy.Job { return makeCacheHistoricalTaskDataJob() })
}

type cacheHistoricalTaskDataJob struct {
	ProjectID  string   `bson:"project_id" json:"project_id" yaml:"project_id"`
	Requesters []string `bson:"requesters" json:"requesters" yaml:"requesters"`
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func NewCacheHistoricalTaskDataJob(projectId string, id string) amboy.Job {
	j := makeCacheHistoricalTaskDataJob()
	j.ProjectID = projectId
	j.Requesters = []string{evergreen.RepotrackerVersionRequester}
	return j
}

func makeCacheHistoricalTaskDataJob() *cacheHistoricalTaskDataJob {
	j := &cacheHistoricalTaskDataJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cacheHistoricalTaskDataName,
				Version: 0,
			},
		},
	}
	return j
}

func (j *cacheHistoricalTaskDataJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	timingMsg := message.Fields{
		"job_id":       j.ID(),
		"project":      j.ProjectID,
		"job_type":     j.Type().Name,
		"message":      "timing-info",
		"run_start_at": time.Now(),
	}
	startAt := time.Now()
	defer func() {
		timingMsg["has_errors"] = j.HasErrors()
		timingMsg["aborted"] = ctx.Err() != nil
		timingMsg["total"] = time.Since(startAt).Seconds()
		timingMsg["run_end_at"] = time.Now()
		grip.Info(timingMsg)
	}()

	// Check for degraded mode flag.
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "retrieving service flags"))
		return
	}
	if flags.CacheStatsJobDisabled {
		j.AddError(errors.New("cache stats job is disabled"))
		return
	}

	var statsStatus taskstats.StatsStatus
	timingMsg["status_check"] = reportTiming(func() {
		// Lookup last sync date for project.
		statsStatus, err = taskstats.GetStatsStatus(j.ProjectID)
		j.AddError(errors.Wrap(err, "retrieving last sync date"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	syncFromTime := statsStatus.ProcessedTasksUntil
	syncToTime := findTargetTimeForSync(syncFromTime)
	timingMsg["sync_from"] = syncFromTime
	timingMsg["sync_to"] = syncToTime
	var statsToUpdate []taskstats.StatsToUpdate
	timingMsg["find_stats_to_update"] = reportTiming(func() {
		statsToUpdate, err = taskstats.FindStatsToUpdate(taskstats.FindStatsToUpdateOptions{
			ProjectID:  j.ProjectID,
			Requesters: j.Requesters,
			Start:      syncFromTime,
			End:        syncToTime,
		})
		j.AddError(errors.Wrap(err, "finding stats to update"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	timingMsg["update_daily_task_stats"] = reportTiming(func() {
		for _, toUpdate := range statsToUpdate {
			if len(toUpdate.Tasks) > 0 {
				err := errors.Wrap(taskstats.GenerateStats(ctx, taskstats.GenerateStatsOptions{
					ProjectID: j.ProjectID,
					Requester: toUpdate.Requester,
					Date:      syncFromTime,
					Tasks:     toUpdate.Tasks,
				}), "generating daily task stats")
				grip.Warning(message.WrapError(err, message.Fields{
					"project_id": j.ProjectID,
					"sync_date":  utility.GetUTCDay(syncFromTime),
					"job_time":   startAt,
				}))
				if err != nil {
					j.AddError(err)
					return
				}
			}
		}
	}).Seconds()
	if j.HasErrors() {
		errorMsg := j.Error().Error()
		// The following errors are known to recur. In these cases we
		// continue to update syncToTime to prevent re-processing of
		// the same error.
		if !strings.Contains(errorMsg, evergreen.KeyTooLargeToIndexError) && !strings.Contains(errorMsg, evergreen.InvalidDivideInputError) {
			return
		}
	}

	timingMsg["save_stats_status"] = reportTiming(func() {
		// Update last sync.
		err = taskstats.UpdateStatsStatus(j.ProjectID, startAt, syncToTime, time.Since(startAt))
		j.AddError(errors.Wrap(err, "updating last synced date"))
	}).Seconds()
}

func reportTiming(fn func()) time.Duration {
	startAt := time.Now()
	fn()
	return time.Since(startAt)
}

// We only want to sync a max of 1 day of data at a time. So, if the previous
// sync was more than 1 day ago, only sync 1 day ahead. Otherwise, we can sync
// to now.
func findTargetTimeForSync(previousSyncTime time.Time) time.Time {
	now := time.Now()
	maxSyncTime := previousSyncTime.Add(maxSyncDuration)

	// Check if the previous sync date is within the max time we want to
	// sync.
	if maxSyncTime.After(now) {
		return now
	}

	return maxSyncTime
}
