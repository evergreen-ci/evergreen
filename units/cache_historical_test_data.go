package units

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	cacheHistoricalTestDataName = "cache-historical-test-data"
	maxSyncDuration             = time.Hour * 24 // one day
)

func init() {
	registry.AddJobType(cacheHistoricalTestDataName,
		func() amboy.Job { return makeCacheHistoricalTestDataJob() })
}

type cacheHistoricalTestDataJob struct {
	ProjectID  string   `bson:"project_id" json:"project_id" yaml:"project_id"`
	Requesters []string `bson:"requesters" json:"requesters" yaml:"requesters"`
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
}

type dailyStatsRollup map[time.Time]map[string][]string

type generateStatsFn func(ctx context.Context, opts stats.GenerateOptions) error
type generateFunctions struct {
	HourlyFns map[string]generateStatsFn
	DailyFns  map[string]generateStatsFn
}

func NewCacheHistoricalTestDataJob(projectId string, id string) amboy.Job {
	j := makeCacheHistoricalTestDataJob()
	j.ProjectID = projectId
	j.Requesters = []string{evergreen.RepotrackerVersionRequester}
	j.SetID(fmt.Sprintf("%s.%s.%s", cacheHistoricalTestDataName, projectId, id))
	return j
}

func makeCacheHistoricalTestDataJob() *cacheHistoricalTestDataJob {
	j := &cacheHistoricalTestDataJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cacheHistoricalTestDataName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *cacheHistoricalTestDataJob) Run(ctx context.Context) {
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
		j.AddError(errors.Wrap(err, "error retrieving service flags"))
		return
	}
	if flags.CacheStatsJobDisabled {
		j.AddError(errors.New("cache stats job is disabled"))
		return
	}

	var statsStatus stats.StatsStatus
	timingMsg["status_check"] = reportTiming(func() {
		// Lookup last sync date for project
		statsStatus, err = stats.GetStatsStatus(j.ProjectID)
		j.AddError(errors.Wrap(err, "error retrieving last sync date"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	var tasksToIgnore []*regexp.Regexp
	timingMsg["tasks_to_ignore_lookup"] = reportTiming(func() {
		tasksToIgnore, err = getTasksToIgnore(j.ProjectID)
		j.AddError(errors.Wrap(err, "error retrieving project settings"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	jobContext := cacheHistoricalJobContext{
		ProjectID:     j.ProjectID,
		JobTime:       time.Now(),
		TasksToIgnore: tasksToIgnore,
		catcher:       grip.NewBasicCatcher(),
		ShouldFilterTasks: map[string]bool{
			"test": true,
			"task": false,
		},
	}

	syncFromTime := statsStatus.ProcessedTasksUntil
	syncToTime := findTargetTimeForSync(syncFromTime)
	timingMsg["sync_from"] = syncFromTime
	timingMsg["sync_to"] = syncToTime
	var statsToUpdate []stats.StatsToUpdate
	timingMsg["find_stats_to_update"] = reportTiming(func() {
		statsToUpdate, err = stats.FindStatsToUpdate(stats.FindStatsOptions{
			ProjectID:  j.ProjectID,
			Requesters: j.Requesters,
			Start:      syncFromTime,
			End:        syncToTime,
		})
		j.AddError(errors.Wrap(err, "error finding tasks to update"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	generateMap := generateFunctions{
		HourlyFns: map[string]generateStatsFn{
			"test": stats.GenerateHourlyTestStats,
		},
		DailyFns: map[string]generateStatsFn{
			"test": stats.GenerateDailyTestStatsFromHourly,
			"task": stats.GenerateDailyTaskStats,
		},
	}

	timingMsg["update_rollups"] = reportTiming(func() {
		timingInfo := jobContext.updateHourlyAndDailyStats(ctx, statsToUpdate, generateMap)
		for k, v := range timingInfo {
			timingMsg[k] = v.Seconds()
		}
	}).Seconds()
	ctxError := jobContext.catcher.Resolve()
	j.AddError(ctxError)
	if j.HasErrors() {
		errorMsg := j.Error().Error()
		// The following errors are known to recur. In these cases we continue to update syncToTime to prevent re-processing of the same error
		if !strings.Contains(errorMsg, evergreen.KeyTooLargeToIndexError) && !strings.Contains(errorMsg, evergreen.InvalidDivideInputError) {
			return
		}
	}

	timingMsg["save_stats_status"] = reportTiming(func() {
		// update last sync
		err = stats.UpdateStatsStatus(j.ProjectID, jobContext.JobTime, syncToTime, time.Since(startAt))
		j.AddError(errors.Wrap(err, "error updating last synced date"))
	}).Seconds()
	j.AddError(jobContext.catcher.Resolve())
	if j.HasErrors() {
		return
	}
}

type cacheHistoricalJobContext struct {
	ProjectID         string
	JobTime           time.Time
	TasksToIgnore     []*regexp.Regexp
	ShouldFilterTasks map[string]bool
	catcher           grip.Catcher
}

func reportTiming(fn func()) time.Duration {
	startAt := time.Now()
	fn()
	return time.Since(startAt)
}

func getTasksToIgnore(projectId string) ([]*regexp.Regexp, error) {
	ref, err := model.FindMergedProjectRef(projectId, "", true)
	if err != nil {
		return nil, errors.Wrap(err, "Could not get project ref")
	}
	if ref == nil {
		return nil, errors.Errorf("project ref '%s' not found", projectId)
	}

	filePatternsStr := ref.FilesIgnoredFromCache

	return createRegexpFromStrings(filePatternsStr)
}

func createRegexpFromStrings(filePatterns []string) ([]*regexp.Regexp, error) {
	var tasksToIgnore []*regexp.Regexp
	for _, patternStr := range filePatterns {
		pattern := strings.Trim(patternStr, " ")
		if pattern != "" {
			regexp, err := regexp.Compile(pattern)
			if err != nil {
				grip.Warningf("Could not compile regexp from '%s'", pattern)
				return nil, errors.Wrap(err, "Could not compile regexp")
			}
			tasksToIgnore = append(tasksToIgnore, regexp)
		}
	}

	return tasksToIgnore, nil
}

func (c *cacheHistoricalJobContext) updateHourlyAndDailyStats(ctx context.Context, statsToUpdate []stats.StatsToUpdate, generateFns generateFunctions) map[string]time.Duration {
	timingInfo := map[string]time.Duration{}
	var err error
	for name, genFn := range generateFns.HourlyFns {
		timingInfo[fmt.Sprintf("update_hourly_%s", name)] = reportTiming(func() {
			err = c.iteratorOverHourlyStats(ctx, statsToUpdate, genFn, name)
			c.catcher.Add(err)
		})
		if err != nil {
			c.catcher.Wrap(err, "error iterating over hourly stats")
		}
	}

	dailyStats := buildDailyStatsRollup(statsToUpdate)

	for name, genFn := range generateFns.DailyFns {
		timingInfo[fmt.Sprintf("update_daily_%s", name)] = reportTiming(func() {
			err = c.iteratorOverDailyStats(ctx, dailyStats, genFn, name)
			c.catcher.Add(err)
		})
		if err != nil {
			c.catcher.Wrap(err, "error iterating over daily stats")
		}
	}

	return timingInfo
}

func (c *cacheHistoricalJobContext) iteratorOverDailyStats(ctx context.Context, dailyStats dailyStatsRollup, fn generateStatsFn, queryType string) error {
	for day, stat := range dailyStats {
		for requester, tasks := range stat {
			taskList := c.filterIgnoredTasks(tasks, queryType)
			if len(taskList) > 0 {
				err := errors.Wrap(fn(ctx, stats.GenerateOptions{
					ProjectID: c.ProjectID,
					Requester: requester,
					Window:    day,
					Tasks:     taskList,
					Runtime:   c.JobTime,
				}), "Could not sync daily stats")
				grip.Warning(message.WrapError(err, message.Fields{
					"project_id": c.ProjectID,
					"sync_date":  day,
					"job_time":   c.JobTime,
					"query_type": queryType,
				}))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (c *cacheHistoricalJobContext) iteratorOverHourlyStats(ctx context.Context, stat []stats.StatsToUpdate, fn generateStatsFn, queryType string) error {
	for _, s := range stat {
		taskList := c.filterIgnoredTasks(s.Tasks, queryType)
		if len(taskList) > 0 {
			err := errors.Wrap(fn(ctx, stats.GenerateOptions{
				ProjectID: s.ProjectId,
				Requester: s.Requester,
				Window:    s.Hour,
				Tasks:     taskList,
				Runtime:   c.JobTime,
			}), "Could not sync hourly stats")
			grip.Warning(message.WrapError(err, message.Fields{
				"project_id": s.ProjectId,
				"sync_date":  s.Hour,
				"job_time":   c.JobTime,
				"query_type": queryType,
			}))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Certain tasks always generate unique names, so they will never have any history. Filter out
// those tasks, so we don't waste time/space tracking them.
func (c *cacheHistoricalJobContext) filterIgnoredTasks(taskList []string, queryType string) []string {
	if !c.ShouldFilterTasks[queryType] {
		return taskList
	}

	var filteredTaskList []string
	for _, task := range taskList {
		if !anyRegexMatch(task, c.TasksToIgnore) {
			filteredTaskList = append(filteredTaskList, task)
		}
	}

	return filteredTaskList
}

func anyRegexMatch(value string, regexList []*regexp.Regexp) bool {
	for _, regexp := range regexList {
		if regexp.MatchString(value) {
			return true
		}
	}

	return false
}

// Multiple hourly stats can belong to the same days, we roll up all work for one day into
// a single batch.
func buildDailyStatsRollup(hourlyStats []stats.StatsToUpdate) dailyStatsRollup {
	rollup := make(dailyStatsRollup)
	for _, stat := range hourlyStats {
		if rollup[stat.Day] == nil {
			rollup[stat.Day] = make(map[string][]string)
		}

		if rollup[stat.Day][stat.Requester] == nil {
			rollup[stat.Day][stat.Requester] = stat.Tasks
		} else {
			rollup[stat.Day][stat.Requester] = append(rollup[stat.Day][stat.Requester], stat.Tasks...)
		}
	}

	return rollup
}

// We only want to sync a max of 1 day of data at a time. So, if the
// previous sync was more than 1 day ago, only sync 1 day
// ahead. Otherwise, we can sync to now.
func findTargetTimeForSync(previousSyncTime time.Time) time.Time {
	now := time.Now()
	maxSyncTime := previousSyncTime.Add(maxSyncDuration)

	// Is the previous sync date within the max time we want to sync?
	if maxSyncTime.After(now) {
		return now
	}

	return maxSyncTime
}
