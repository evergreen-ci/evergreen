package units

import (
	"context"
	"fmt"
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

const cacheHistoricalTaskDataName = "cache-historical-task-data"

func init() {
	registry.AddJobType(cacheHistoricalTaskDataName,
		func() amboy.Job { return makeCacheHistoricalTaskDataJob() })
}

type cacheHistoricalTaskDataJob struct {
	ProjectID  string   `bson:"project_id" json:"project_id" yaml:"project_id"`
	Requesters []string `bson:"requesters" json:"requesters" yaml:"requesters"`
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
}

func NewCacheHistoricalTaskDataJob(id, projectID string) amboy.Job {
	j := makeCacheHistoricalTaskDataJob()
	j.ProjectID = projectID
	j.Requesters = []string{evergreen.RepotrackerVersionRequester}
	j.SetID(fmt.Sprintf("%s.%s.%s", cacheHistoricalTaskDataName, projectID, id))
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

	startAt := time.Now()
	timingMsg := message.Fields{
		"job_id":       j.ID(),
		"project":      j.ProjectID,
		"job_type":     j.Type().Name,
		"message":      "timing-info",
		"run_start_at": startAt,
	}
	defer func() {
		timingMsg["has_errors"] = j.HasErrors()
		timingMsg["aborted"] = ctx.Err() != nil
		timingMsg["total"] = time.Since(startAt).Seconds()
		timingMsg["run_end_at"] = time.Now()
		grip.Info(timingMsg)
	}()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
		return
	}
	if flags.CacheStatsJobDisabled {
		j.AddError(errors.New("cache stats job is disabled"))
		return
	}

	var statsStatus taskstats.StatsStatus
	timingMsg["status_check"] = reportTiming(func() {
		statsStatus, err = taskstats.GetStatsStatus(ctx, j.ProjectID)
		j.AddError(errors.Wrap(err, "getting daily task stats status"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	// Calculate the window of time within which we would like to check for
	// stats to update, starting with ProcessedTasksUntil (the time before
	// which all finished tasks have been processed for this project) up
	// until now.
	updateWindowStart, updateWindowEnd := statsStatus.GetUpdateWindow()
	timingMsg["stats_update_window_start"] = updateWindowStart
	timingMsg["stats_update_window_end"] = updateWindowEnd

	var statsToUpdate []taskstats.StatsToUpdate
	timingMsg["find_task_stats_to_update"] = reportTiming(func() {
		statsToUpdate, err = taskstats.FindStatsToUpdate(ctx, taskstats.FindStatsToUpdateOptions{
			ProjectID:  j.ProjectID,
			Requesters: j.Requesters,
			Start:      updateWindowStart,
			End:        updateWindowEnd,
		})
		j.AddError(errors.Wrap(err, "finding daily task stats to update"))
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
					Date:      toUpdate.Day,
					Tasks:     toUpdate.Tasks,
				}), "generating daily task stats")
				grip.Warning(message.WrapError(err, message.Fields{
					"job_id":         j.ID(),
					"project":        j.ProjectID,
					"job_type":       j.Type().Name,
					"job_start_time": startAt,
					"task_date":      utility.GetUTCDay(toUpdate.Day),
				}))
				if err != nil {
					j.AddError(err)
					return
				}
			}
		}
	}).Seconds()
	if j.HasErrors() {
		errMsg := j.Error().Error()
		// The following errors are known to recur. In these cases we
		// continue to update the task stats status to prevent
		// re-processing of the same error.
		if !strings.Contains(errMsg, evergreen.KeyTooLargeToIndexError) && !strings.Contains(errMsg, evergreen.InvalidDivideInputError) {
			return
		}
	}

	timingMsg["save_stats_status"] = reportTiming(func() {
		j.AddError(errors.Wrap(taskstats.UpdateStatsStatus(ctx, j.ProjectID, startAt, updateWindowEnd, time.Since(startAt)), "updating daily task stats status"))
	}).Seconds()
}

func reportTiming(fn func()) time.Duration {
	startAt := time.Now()
	fn()
	return time.Since(startAt)
}
