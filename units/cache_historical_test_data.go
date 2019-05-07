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
	maxSyncDuration             = time.Hour * 24 * 7 // one week
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

type generateStatsFn func(projectId string, requester string, timePeriod time.Time, tasks []string, jobDate time.Time) error
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

	// Lookup last sync date for project
	statsStatus, err := stats.GetStatsStatus(j.ProjectID)
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving last sync date"))
		return
	}

	tasksToIgnore, err := getTasksToIgnore(j.ProjectID)
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving project settings"))
		return
	}

	jobContext := cacheHistoricalJobContext{
		ProjectID:     j.ProjectID,
		JobTime:       time.Now(),
		TasksToIgnore: tasksToIgnore,
		ShouldFilterTasks: map[string]bool{
			"test": true,
			"task": false,
		},
	}

	syncFromTime := statsStatus.ProcessedTasksUntil
	syncToTime := findTargetTimeForSync(syncFromTime)

	grip.Info(message.Fields{
		"job_id":    j.ID(),
		"sync_from": syncFromTime,
		"sync_to":   syncToTime,
		"message":   "running sync",
	})

	statsToUpdate, err := stats.FindStatsToUpdate(j.ProjectID, j.Requesters, syncFromTime, syncToTime)
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding tasks to update"))
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

	err = jobContext.updateHourlyAndDailyStats(statsToUpdate, generateMap)
	if err != nil {
		j.AddError(errors.Wrap(err, "error generating hourly test stats"))
		return
	}

	// update last sync
	err = stats.UpdateStatsStatus(j.ProjectID, jobContext.JobTime, syncToTime)
	if err != nil {
		j.AddError(errors.Wrap(err, "error updating last synced date"))
		return
	}
}

type cacheHistoricalJobContext struct {
	ProjectID         string
	JobTime           time.Time
	TasksToIgnore     []*regexp.Regexp
	ShouldFilterTasks map[string]bool
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

func (c *cacheHistoricalJobContext) updateHourlyAndDailyStats(statsToUpdate []stats.StatsToUpdate, generateFns generateFunctions) error {
	for name, genFn := range generateFns.HourlyFns {
		err := c.iteratorOverHourlyStats(statsToUpdate, genFn, name)
		if err != nil {
			return err
		}
	}

	dailyStats := buildDailyStatsRollup(statsToUpdate)

	for name, genFn := range generateFns.DailyFns {
		err := c.iteratorOverDailyStats(dailyStats, genFn, name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *cacheHistoricalJobContext) iteratorOverDailyStats(dailyStats dailyStatsRollup, fn generateStatsFn, queryType string) error {
	for day, stats := range dailyStats {
		for requester, tasks := range stats {
			taskList := c.filterIgnoredTasks(tasks, queryType)
			if len(taskList) > 0 {
				err := errors.Wrap(fn(c.ProjectID, requester, day, taskList, c.JobTime), "Could not sync daily stats")
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

func (c *cacheHistoricalJobContext) iteratorOverHourlyStats(stats []stats.StatsToUpdate, fn generateStatsFn, queryType string) error {
	for _, stat := range stats {
		taskList := c.filterIgnoredTasks(stat.Tasks, queryType)
		if len(taskList) > 0 {
			err := errors.Wrap(fn(stat.ProjectId, stat.Requester, stat.Hour, taskList, c.JobTime), "Could not sync hourly stats")
			grip.Warning(message.WrapError(err, message.Fields{
				"project_id": stat.ProjectId,
				"sync_date":  stat.Hour,
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

// We only want to sync a max of 1 week of data at a time. So, if the previous sync was more than
// 1 week ago, only sync 1 week ahead. Otherwise, we can sync to now.
func findTargetTimeForSync(previousSyncTime time.Time) time.Time {
	now := time.Now()
	maxSyncTime := previousSyncTime.Add(maxSyncDuration)

	// Is the previous sync date within the max time we want to sync?
	if maxSyncTime.After(now) {
		return now
	}

	return maxSyncTime
}
