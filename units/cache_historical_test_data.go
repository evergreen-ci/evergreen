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
	cacheHistoricalTestDataName = "cache-historical-test-data"
	maxSyncDuration             = time.Hour * 24 // one day
)

func init() {
	registry.AddJobType(cacheHistoricalTestDataName,
		func() amboy.Job { return makeCacheHistoricalTestDataJob() })
}

type cacheHistoricalTestDataJob struct {
	ProjectID       string   `bson:"project_id" json:"project_id" yaml:"project_id"`
	Requesters      []string `bson:"requesters" json:"requesters" yaml:"requesters"`
	DisableOldTasks bool     `bson:"disable_old_tasks" json:"disable_old_tasks" yaml:"disable_old_tasks"`
	EnableMerge     bool     `bson:"enable_merge,omitempty" json:"enable_merge,omitempty" yaml:"enable_merge,omitempty"`
	MergeBatchLimit int      `bson:"merge_batch_limit,omitempty" json:"merge_batch_limit,omitempty" yaml:"merge_batch_limit,omitempty"`
	job.Base        `bson:"job_base" json:"job_base" yaml:"job_base"`
}

type dailyStatsRollup map[time.Time]map[string][]string

type historicalJob interface {
	Run(ctx context.Context) error
	Name() string
}

// NewCacheHistoricalTestDataJob create a job to generate the historical data.
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

	j.DisableOldTasks = flags.CacheStatsOldTasksDisabled
	j.EnableMerge = flags.CacheStatsMergeEnabled
	j.MergeBatchLimit = flags.CacheStatsMergeBatchLimit

	var statsStatus stats.StatsStatus
	timingMsg["status_check"] = reportTiming(func() {
		// Lookup last sync date for project
		var err error
		statsStatus, err = stats.GetStatsStatus(j.ProjectID)
		j.AddError(errors.Wrap(err, "error retrieving last sync date"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	var tasksToIgnore []*regexp.Regexp
	timingMsg["tasks_to_ignore_lookup"] = reportTiming(func() {
		var err error
		tasksToIgnore, err = getTasksToIgnore(j.ProjectID)
		j.AddError(errors.Wrap(err, "error retrieving project settings"))
	}).Seconds()
	if j.HasErrors() {
		return
	}

	jobs := []historicalJob{}

	syncFromTime := statsStatus.ProcessedTasksUntil
	syncToTime, backfill := findTargetTimeForSync(syncFromTime)

	timingMsg["sync_from"] = syncFromTime
	timingMsg["sync_to"] = syncToTime

	timingMsg["find_stats_to_update"] = reportTiming(func() {
		var err error

		taskStatsToUpdate, err := stats.FindStatsToUpdate(stats.FindStatsOptions{
			ProjectID:       j.ProjectID,
			Requesters:      j.Requesters,
			Start:           syncFromTime,
			End:             syncToTime,
			DisableOldTasks: j.DisableOldTasks,
		})
		j.AddError(errors.Wrap(err, "error finding tasks to update"))

		jobs = append(jobs, newHourlyTestStatsJob(j, taskStatsToUpdate, j.Requesters, tasksToIgnore, syncFromTime, startAt, backfill))
		jobs = append(jobs, newDailyTestStatsJob(j, taskStatsToUpdate, j.Requesters, tasksToIgnore, syncFromTime, startAt, backfill))
		jobs = append(jobs, newDailyTaskStatsLookupJob(j, taskStatsToUpdate, tasksToIgnore, syncFromTime, startAt, backfill))

	}).Seconds()
	if j.HasErrors() {
		return
	}

	catcher := grip.NewBasicCatcher()
	timingMsg["update_rollups"] = reportTiming(func() {
		var err error

		for _, job := range jobs {
			timingMsg[job.Name()] = reportTiming(func() {
				err = job.Run(ctx)
			}).Seconds()
			if err != nil {
				catcher.Add(err)
				return
			}
		}
	}).Seconds()
	j.AddError(catcher.Resolve())
	if j.HasErrors() {
		return
	}

	timingMsg["save_stats_status"] = reportTiming(func() {
		// update last sync
		err = stats.UpdateStatsStatus(j.ProjectID, startAt, syncToTime, time.Since(startAt))
		j.AddError(errors.Wrap(err, "error updating last synced date"))
	}).Seconds()
	if j.HasErrors() {
		return
	}
}

func reportTiming(fn func()) time.Duration {
	startAt := time.Now()
	fn()
	return time.Since(startAt)
}

func getTasksToIgnore(projectId string) ([]*regexp.Regexp, error) {
	ref, err := model.FindOneProjectRef(projectId)
	if err != nil {
		return nil, errors.Wrap(err, "Could not get project ref")
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

// Certain tasks always generate unique names, so they will never have any history. Filter out
// those tasks, so we don't waste time/space tracking them.
func filterIgnoredTasks(taskList []string, tasksToIgnore []*regexp.Regexp) []string {
	var filteredTaskList []string
	for _, task := range taskList {
		if !anyRegexMatch(task, tasksToIgnore) {
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

// newHourlyTestStatsJob is a factory to create the configured hourly test stats job.
func newHourlyTestStatsJob(j *cacheHistoricalTestDataJob, taskStatsToUpdate []stats.StatsToUpdate, requesters []string, tasksToIgnore []*regexp.Regexp, syncFromTime time.Time, jobTime time.Time, backfill bool) historicalJob {
	if j.EnableMerge {
		return newHourlyTestStatsMergeJob(j, requesters, tasksToIgnore, syncFromTime, jobTime, backfill)
	}
	return newHourlyTestStatsLookupJob(j, taskStatsToUpdate, tasksToIgnore, syncFromTime, jobTime)
}

// newDailyTestStatsJob is a factory to create the configured daily test stats job.
func newDailyTestStatsJob(j *cacheHistoricalTestDataJob, taskStatsToUpdate []stats.StatsToUpdate, requesters []string, tasksToIgnore []*regexp.Regexp, syncFromTime time.Time, jobTime time.Time, backfill bool) historicalJob {
	if j.EnableMerge {
		return newDailyTestStatsMergeJob(j, requesters, tasksToIgnore, syncFromTime, jobTime, backfill)
	}
	return newDailyTestStatsLookupJob(j, taskStatsToUpdate, tasksToIgnore, syncFromTime, jobTime)
}

// --------------------------------------->
type hourlyTestStatsMergeJob struct {
	ProjectID         string
	JobTime           time.Time
	TasksToIgnore     []*regexp.Regexp
	MergeBatchLimit   int
	TestStatsToUpdate []stats.StatsToUpdate
	Backfill          bool
}

// newHourlyTestStatsMergeJob creates an Hourly Test stats job.
func newHourlyTestStatsMergeJob(j *cacheHistoricalTestDataJob, requesters []string, tasksToIgnore []*regexp.Regexp, fromTime time.Time, jobTime time.Time, backfill bool) *hourlyTestStatsMergeJob {

	// Merge run per project from a start point in time to an
	// end point in time. Tasks are excluded rather than included
	testStatsToUpdate := []stats.StatsToUpdate{}

	// Process up to the top of the last hour. The _id are probably
	// coming from the driver in the client, so be conservative to
	// ensure that we don't end up skipping testresults that are
	// inserted later (on an earlier time) or a situation
	// where the time is not properly synced.
	hour := util.GetUTCHour(time.Now().Truncate(time.Hour)) //.Truncate(time.Hour)
	fromTime = util.GetUTCHour(fromTime.Truncate(time.Hour))
	for _, requester := range requesters {
		for i := 0; i < 24; i++ {
			boundary := fromTime.Add(time.Duration(i) * time.Hour)
			if !boundary.Before(hour) {
				break
			}
			testStatsToUpdate = append(testStatsToUpdate,
				stats.StatsToUpdate{
					ProjectId: j.ProjectID,
					Requester: requester,
					Hour:      boundary,
					Day:       boundary.Truncate(24 * time.Hour),
					Tasks:     []string{},
				})
		}
	}

	return &hourlyTestStatsMergeJob{
		ProjectID:         j.ProjectID,
		JobTime:           jobTime,
		TasksToIgnore:     tasksToIgnore,
		MergeBatchLimit:   j.MergeBatchLimit,
		TestStatsToUpdate: testStatsToUpdate,
		Backfill:          backfill,
	}
}

// fastforward skips over test results that are too old to process.
func (j *hourlyTestStatsMergeJob) fastforward(ctx context.Context) error {

	if j.Backfill {
		seen := make(map[string]map[string]bool)

		for _, stat := range j.TestStatsToUpdate {
			projectID := stat.ProjectId
			requester := stat.Requester
			if _, ok := seen[projectID]; !ok {
				seen[projectID] = make(map[string]bool)
			}
			if _, ok := seen[projectID][requester]; !ok {
				err := stats.FastforwardHourlyTestStats(stat)
				if err != nil {
					return errors.Wrap(err, "Failed to Get lastest hourly test doc")
				}

			}
			seen[projectID][requester] = true
		}

	}
	return nil
}

// Run generates the Hourly test Stats.
func (j *hourlyTestStatsMergeJob) Run(ctx context.Context) error {

	err := j.fastforward(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to fast forward")
	}

	for _, s := range j.TestStatsToUpdate {
		err := errors.Wrap(stats.GenerateHourlyTestStatsUsingMerge(ctx, stats.GenerateOptions{
			ProjectID:       s.ProjectId,
			Requester:       s.Requester,
			Window:          s.Hour,
			TasksToIgnore:   j.TasksToIgnore,
			Runtime:         j.JobTime,
			MergeBatchLimit: j.MergeBatchLimit,
		}), "Could not sync hourly stats")
		grip.Warning(message.WrapError(err, message.Fields{
			"project_id": s.ProjectId,
			"sync_date":  s.Hour,
			"job_time":   j.JobTime,
			"query_type": "test",
		}))
		if err != nil {
			return err
		}
	}

	return nil
}

// Name returns a friendly name for this job.
func (j *hourlyTestStatsMergeJob) Name() string {
	return "update_hourly_test"
}

type hourlyTestStatsLookupJob struct {
	ProjectID         string
	JobTime           time.Time
	TasksToIgnore     []*regexp.Regexp
	DisableOldTasks   bool
	TestStatsToUpdate []stats.StatsToUpdate
}

// newHourlyTestStatsLookupJob creates a new Hourly Test stats job.
func newHourlyTestStatsLookupJob(j *cacheHistoricalTestDataJob, testStatsToUpdate []stats.StatsToUpdate, tasksToIgnore []*regexp.Regexp, syncFromTime time.Time, jobTime time.Time) *hourlyTestStatsLookupJob {
	return &hourlyTestStatsLookupJob{
		ProjectID:         j.ProjectID,
		JobTime:           jobTime,
		TasksToIgnore:     tasksToIgnore,
		DisableOldTasks:   j.DisableOldTasks,
		TestStatsToUpdate: testStatsToUpdate,
	}
}

// Name returns a friendly name for this job.
func (j *hourlyTestStatsLookupJob) Name() string {
	return "update_hourly_test"
}

// Run generates the Hourly Task Stats for the given range of values.
func (j *hourlyTestStatsLookupJob) Run(ctx context.Context) error {
	for _, s := range j.TestStatsToUpdate {
		taskList := filterIgnoredTasks(s.Tasks, j.TasksToIgnore)
		if len(taskList) > 0 {
			err := errors.Wrap(stats.GenerateHourlyTestStats(ctx, stats.GenerateOptions{
				ProjectID:       s.ProjectId,
				Requester:       s.Requester,
				Window:          s.Hour,
				Tasks:           taskList,
				Runtime:         j.JobTime,
				DisableOldTasks: j.DisableOldTasks,
			}), "Could not sync hourly stats")
			grip.Warning(message.WrapError(err, message.Fields{
				"project_id": s.ProjectId,
				"sync_date":  s.Hour,
				"job_time":   j.JobTime,
				"query_type": "test",
			}))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type dailyTestStatsMergeJob struct {
	ProjectID       string
	JobTime         time.Time
	TasksToIgnore   []*regexp.Regexp
	MergeBatchLimit int
	Rollup          dailyStatsRollup
	Backfill        bool
}

// newDailyTestStatsMergeJob creates a Daily Test stats job.
func newDailyTestStatsMergeJob(j *cacheHistoricalTestDataJob, requesters []string, tasksToIgnore []*regexp.Regexp, syncFromTime time.Time, jobTime time.Time, backfill bool) *dailyTestStatsMergeJob {
	// Process up to the top of the last hour. The _id are probably
	// coming from the driver in the client, so be conservative to
	// ensure that we don't end up skipping testresults that are
	// inserted later (on an earlier time) or a situation
	// where the time is not properly synced.
	hour := util.GetUTCHour(time.Now().Truncate(time.Hour)) //.Truncate(time.Hour)
	rollup := make(dailyStatsRollup)

	syncFromTime = util.GetUTCDay(syncFromTime)
	if !syncFromTime.After(hour) {
		for _, requester := range requesters {

			if rollup[syncFromTime] == nil {
				rollup[syncFromTime] = make(map[string][]string)
			}
			if rollup[syncFromTime][requester] == nil {
				rollup[syncFromTime][requester] = []string{}
			} else {
				rollup[syncFromTime][requester] = append(rollup[syncFromTime][requester], []string{}...)
			}
		}
	}

	return &dailyTestStatsMergeJob{
		ProjectID:       j.ProjectID,
		JobTime:         jobTime,
		TasksToIgnore:   tasksToIgnore,
		MergeBatchLimit: j.MergeBatchLimit,
		Rollup:          rollup,
		Backfill:        backfill,
	}
}

// Name returns a friendly name for this job.
func (j *dailyTestStatsMergeJob) Name() string {
	return "update_daily_test"
}

// fastforward skips over test results that are too old to process.
func (j dailyTestStatsMergeJob) fastforward(ctx context.Context) error {
	if j.Backfill {
		for day, stat := range j.Rollup {
			for requester := range stat {
				err := stats.FastforwardDailyTestStats(j.ProjectID, requester, day)
				if err != nil {
					return errors.Wrap(err, "Failed to Get lastest hourly test doc")
				}
			}
		}
	}
	return nil
}

// Run generates the Daily Test Stats for the daily rollup.
func (j *dailyTestStatsMergeJob) Run(ctx context.Context) error {
	err := j.fastforward(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to fast forward")
	}

	for day, stat := range j.Rollup {
		for requester := range stat {
			err := errors.Wrap(stats.GenerateDailyTestStatsUsingMerge(ctx, stats.GenerateOptions{
				ProjectID:       j.ProjectID,
				Requester:       requester,
				Window:          day,
				TasksToIgnore:   j.TasksToIgnore,
				Runtime:         j.JobTime,
				MergeBatchLimit: j.MergeBatchLimit,
			}), "Could not sync daily stats")
			grip.Warning(message.WrapError(err, message.Fields{
				"project_id": j.ProjectID,
				"sync_date":  day,
				"job_time":   j.JobTime,
				"query_type": "test",
			}))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type dailyTestStatsLookupJob struct {
	ProjectID       string
	JobTime         time.Time
	TasksToIgnore   []*regexp.Regexp
	DisableOldTasks bool
	DailyStats      dailyStatsRollup
}

// newDailyTestStatsLookupJob creates a new Daily Test stats job.
func newDailyTestStatsLookupJob(j *cacheHistoricalTestDataJob, hourlyStats []stats.StatsToUpdate, tasksToIgnore []*regexp.Regexp, syncFromTime time.Time, jobTime time.Time) *dailyTestStatsLookupJob {
	return &dailyTestStatsLookupJob{
		ProjectID:       j.ProjectID,
		JobTime:         jobTime,
		TasksToIgnore:   tasksToIgnore,
		DisableOldTasks: j.DisableOldTasks,
		DailyStats:      buildDailyStatsRollup(hourlyStats),
	}
}

// Run generates the Daily Test Stats for the daily rollup.
func (j *dailyTestStatsLookupJob) Run(ctx context.Context) error {
	for day, stat := range j.DailyStats {
		for requester, tasks := range stat {
			taskList := filterIgnoredTasks(tasks, j.TasksToIgnore)
			if len(taskList) > 0 {
				err := errors.Wrap(stats.GenerateDailyTestStatsFromHourly(ctx, stats.GenerateOptions{
					ProjectID:       j.ProjectID,
					Requester:       requester,
					Window:          day,
					Tasks:           taskList,
					Runtime:         j.JobTime,
					DisableOldTasks: j.DisableOldTasks,
				}), "Could not sync daily stats")
				grip.Warning(message.WrapError(err, message.Fields{
					"project_id": j.ProjectID,
					"sync_date":  day,
					"job_time":   j.JobTime,
					"query_type": "test",
				}))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Name returns a friendly name for this job.
func (j *dailyTestStatsLookupJob) Name() string {
	return "update_daily_test"
}

type dailyTaskStatsLookupJob struct {
	ProjectID       string
	JobTime         time.Time
	TasksToIgnore   []*regexp.Regexp
	DisableOldTasks bool
	Rollup          dailyStatsRollup
}

func newDailyTaskStatsLookupJob(j *cacheHistoricalTestDataJob, taskStatsToUpdate []stats.StatsToUpdate, tasksToIgnore []*regexp.Regexp, syncFromTime time.Time, jobTime time.Time, fastforward bool) historicalJob {
	return &dailyTaskStatsLookupJob{
		ProjectID:       j.ProjectID,
		JobTime:         jobTime,
		TasksToIgnore:   tasksToIgnore,
		DisableOldTasks: j.DisableOldTasks,
		Rollup:          buildDailyStatsRollup(taskStatsToUpdate),
	}
}

// Run generates the Daily Task Stats.
func (j *dailyTaskStatsLookupJob) Run(ctx context.Context) error {
	for day, stat := range j.Rollup {
		for requester, tasks := range stat {
			if len(tasks) > 0 {
				err := errors.Wrap(stats.GenerateDailyTaskStats(ctx, stats.GenerateOptions{
					ProjectID:       j.ProjectID,
					Requester:       requester,
					Window:          day,
					Tasks:           tasks,
					Runtime:         j.JobTime,
					DisableOldTasks: j.DisableOldTasks,
				}), "Could not sync daily stats")
				grip.Warning(message.WrapError(err, message.Fields{
					"project_id": j.ProjectID,
					"sync_date":  day,
					"job_time":   j.JobTime,
					"query_type": "task",
				}))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil

}

// Name returns a friendly name for this job.
func (j *dailyTaskStatsLookupJob) Name() string {
	return "update_daily_task"
}

// We only want to sync a max of 1 day of data at a time. So, if the
// previous sync was more than 1 day ago, only sync 1 day
// ahead. Otherwise, we can sync to now. The boolean returns value
// is true if this is backfilling old data.
func findTargetTimeForSync(previousSyncTime time.Time) (time.Time, bool) {
	now := time.Now()
	maxSyncTime := previousSyncTime.Add(maxSyncDuration)

	// Is the previous sync date within the max time we want to sync?
	if maxSyncTime.After(now) {
		return now.Truncate(time.Hour), false
	}

	return maxSyncTime, true
}
