// Package stats provides functions to generate and query pre-computed test and task statistics.
// The statistics are aggregated per day and a combination of (test, task, variant, distro, project, requester) for
// tests and a combination of (task, variant, distro,  project, requester) for tasks.
// For tests intermediate hourly statistics are also stored to avoid some re-computation.
package stats

import (
	"context"
	"math/rand"
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen/model/testresult"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	defaultBackFillPeriod = 4 * 7 * 24 * time.Hour
)

// StatsStatus represents the status for stats pre-computations for a project.
// LastJobRun is the start date of the last successful pre-computation job that ran for the project.
// ProcessedTasksUntil is the date before which all finished tasks have been processed. It is usually
// the same as LastJobRun unless a previous job has failed and the stats computation has not caught up yet.
type StatsStatus struct {
	ProjectId           string        `bson:"_id"`
	LastJobRun          time.Time     `bson:"last_job_run"`
	ProcessedTasksUntil time.Time     `bson:"processed_tasks_until"`
	Runtime             time.Duration `bson:"runtime"`
}

// createDefaultStatsStatus creates a StatsStatus for projects that don't have a status in the DB yet.
func createDefaultStatsStatus(projectId string) StatsStatus {
	defaultBackFillStart := util.GetUTCDay(time.Now().Add(-defaultBackFillPeriod))
	return StatsStatus{
		ProjectId:           projectId,
		LastJobRun:          defaultBackFillStart,
		ProcessedTasksUntil: defaultBackFillStart,
	}
}

////////////////////////////
// Stats status functions //
////////////////////////////

// GetStatsStatus retrieves the status of the stats pre-computations for a project.
func GetStatsStatus(projectId string) (StatsStatus, error) {
	status := StatsStatus{}
	query := statsStatusQuery(projectId)
	err := db.FindOne(dailyStatsStatusCollection, query, db.NoProjection, db.NoSort, &status)
	if adb.ResultsNotFound(err) {
		return createDefaultStatsStatus(projectId), nil
	}
	if err != nil {
		return status, errors.Wrap(err, "Failed to retrieve test stats status")
	}
	return status, nil
}

// UpdateStatsStatus updates the status of the stats pre-computations for a project.
func UpdateStatsStatus(projectId string, lastJobRun time.Time, processedTasksUntil time.Time, runtime time.Duration) error {
	status := StatsStatus{
		ProjectId:           projectId,
		LastJobRun:          lastJobRun,
		ProcessedTasksUntil: processedTasksUntil,
		Runtime:             runtime,
	}
	_, err := db.Upsert(dailyStatsStatusCollection, bson.M{"_id": projectId}, status)
	if err != nil {
		return errors.Wrap(err, "Failed to update test stats status")
	}
	return nil
}

//////////////////////////////////////////////////////
// Hourly and daily test stats generation functions //
//////////////////////////////////////////////////////

type GenerateOptions struct {
	ProjectID       string
	Requester       string
	Tasks           []string
	TasksToIgnore   []*regexp.Regexp
	Window          time.Time
	Runtime         time.Time
	DisableOldTasks bool
	EnableMerge     bool
	MergeBatchLimit int
}

// GenerateHourlyTestStats aggregates task and testresults present in the database and saves the
// resulting hourly test stats documents for the project, requester, hour, and tasks specified.
// The hour covered is the UTC hour corresponding to the given `hour` parameter.
func GenerateHourlyTestStats(ctx context.Context, opts GenerateOptions) error {
	grip.Info(message.Fields{
		"message":   "Generating hourly test stats",
		"project":   opts.ProjectID,
		"requester": opts.Requester,
		"hour":      opts.Window,
		"tasks":     opts.Tasks,
	})
	start := util.GetUTCHour(opts.Window)
	end := start.Add(time.Hour)

	pipeline := hourlyTestStatsPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
	err := aggregateIntoCollection(ctx, task.Collection, pipeline, hourlyTestStatsCollection)

	if err != nil {
		return errors.Wrap(err, "Failed to generate hourly stats")
	}

	if !opts.DisableOldTasks {
		grip.Info(message.Fields{
			"message":   "Generating hourly test stats from old tasks",
			"project":   opts.ProjectID,
			"requester": opts.Requester,
			"hour":      opts.Window,
			"tasks":     opts.Tasks,
		})
		// Generate/Update the stats for old tasks.
		pipeline = hourlyTestStatsForOldTasksPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
		err = aggregateIntoCollection(ctx, task.OldCollection, pipeline, hourlyTestStatsCollection)
		if err != nil {
			return errors.Wrap(err, "Failed to generate hourly stats for old tasks")
		}
	}
	return nil
}

// GenerateHourlyTestStatsUsingMerge aggregates testresults present in the database and saves the
// resulting hourly test stats documents for the project, requester, hour, and tasks specified.
// The hour covered is the UTC hour corresponding to the given `hour` parameter.
func GenerateHourlyTestStatsUsingMerge(ctx context.Context, opts GenerateOptions) error {
	grip.Info(message.Fields{
		"message":         "Generating hourly test stats using merge",
		"project":         opts.ProjectID,
		"requester":       opts.Requester,
		"hour":            opts.Window,
		"tasks":           opts.Tasks,
		"ignore":          opts.TasksToIgnore,
		"MergeBatchLimit": opts.MergeBatchLimit,
	})
	start := util.GetUTCHour(opts.Window)
	end := start.Add(time.Hour)

	// start and endID define the execute bounds of the aggregation. currentID captures the
	// progress through the range.
	currentID := mgobson.NewObjectIdWithTime(start)
	endID := mgobson.NewObjectIdWithTime(end)

	testStats, err := GetLatestHourlyTestDoc(opts.ProjectID, []string{opts.Requester})
	if err != nil {
		return errors.Wrap(err, "Failed to Get lastest hourly test doc")
	}

	// If we have already processed the object ids in this range. Say the
	// task died before it updated the daily stats status.
	if testStats != nil {
		if testStats.LastID >= endID {
			return nil
		}
		currentID = testStats.LastID
	}

	for {
		grip.Info(message.Fields{
			"message":         "Generating hourly test stats using merge progress",
			"project":         opts.ProjectID,
			"requester":       opts.Requester,
			"hour":            opts.Window,
			"tasks":           opts.Tasks,
			"ignore":          opts.TasksToIgnore,
			"MergeBatchLimit": opts.MergeBatchLimit,
			"start":           start,
			"current":         currentID.Time(),
			"end":             endID.Time(),
		})

		pipeline := hourlyTestStatsMergePipeline([]string{opts.ProjectID}, []string{opts.Requester}, opts.TasksToIgnore, opts.Runtime, opts.MergeBatchLimit, currentID, endID)
		output := []bson.M{}
		err = db.Aggregate(testresult.Collection, pipeline, &output)

		if err != nil {
			return errors.Wrap(err, "Failed to generate hourly stats")
		}

		// Get the last processed Test Doc
		testStats, err = GetLatestHourlyTestDoc(opts.ProjectID, []string{opts.Requester})
		if err != nil {
			return errors.Wrap(err, "Failed to Get lastest daily test doc")
		}

		if testStats == nil {
			break
		}

		if testStats.LastID == currentID {
			break
		}
		currentID = testStats.LastID
		time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
	}

	return nil
}

// FastforwardHourlyTestStats is used to skip past TestResults that are too old
// to safely process.
func FastforwardHourlyTestStats(stat StatsToUpdate) error {

	start := util.GetUTCHour(stat.Hour)

	testStats, err := GetLatestHourlyTestDoc(stat.ProjectId, []string{stat.Requester})
	if err != nil {
		return errors.Wrap(err, "Failed to Get lastest hourly test doc")
	}

	// If we have already processed the object ids in this range. Say the
	// task died before it updated the daily stats status.
	if testStats != nil {
		from := start.Add(-time.Hour).Truncate(time.Hour)
		endID := mgobson.NewObjectIdWithTime(from)

		if testStats.LastID < endID {
			if err := UpdateHourlyTestDocLastID(testStats, endID); err != nil {
				return errors.Wrap(err, "Failed to Set lastest daily test doc")
			}
		}

	}
	return nil
}

// GenerateDailyTestStatsFromHourly aggregates the hourly test stats present in the database and
// saves the resulting daily test stats documents for the project, requester, day, and tasks specified.
// The day covered is the UTC day corresponding to the given `day` parameter.
func GenerateDailyTestStatsFromHourly(ctx context.Context, opts GenerateOptions) error {
	grip.Info(message.Fields{
		"message":   "Generating daily test stats",
		"project":   opts.ProjectID,
		"requester": opts.Requester,
		"hour":      opts.Window,
		"tasks":     opts.Tasks,
	})
	start := util.GetUTCDay(opts.Window)
	end := start.Add(24 * time.Hour)
	pipeline := dailyTestStatsFromHourlyPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
	err := aggregateIntoCollection(ctx, hourlyTestStatsCollection, pipeline, dailyTestStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate hourly stats into daily stats")
	}
	return nil
}

// FastforwardDailyTestStats is used to skip past TestResults that are too old
// to safely process.
func FastforwardDailyTestStats(projectID string, requester string, day time.Time) error {

	testStats, err := GetLatestDailyTestDoc(projectID, []string{requester})
	if err != nil {
		return errors.Wrap(err, "Failed to Get lastest hourly test doc")
	}

	// If we have already processed the object ids in this range. Say the
	// task died before it updated the daily stats status.
	if testStats != nil {
		from := util.GetUTCDay(day).Add(-time.Hour).Truncate(time.Hour)
		endID := mgobson.NewObjectIdWithTime(from)

		if testStats.LastID < endID {
			if err := UpdateDailyTestDocLastID(testStats, endID); err != nil {
				return errors.Wrap(err, "Failed to Set lastest daily test doc")
			}
		}

	}
	return nil
}

// GenerateDailyTestStatsUsingMerge aggregates the testresults present in the database and
// saves the resulting daily test stats documents for the project, requester, day, and tasks specified.
// It is not dependant on the hourly_test_stats.
// The day covered is the UTC day corresponding to the given `day` parameter.
func GenerateDailyTestStatsUsingMerge(ctx context.Context, opts GenerateOptions) error {
	grip.Info(message.Fields{
		"message":         "Generating daily test stats using merge",
		"project":         opts.ProjectID,
		"requester":       opts.Requester,
		"hour":            opts.Window,
		"tasks":           opts.Tasks,
		"ignore":          opts.TasksToIgnore,
		"MergeBatchLimit": opts.MergeBatchLimit,
	})
	start := util.GetUTCDay(opts.Window)
	end := start.Add(24 * time.Hour)

	// start and endID define the execute bounds of the aggregation. currentID captures the
	// progress through the range.
	currentID := mgobson.NewObjectIdWithTime(start)
	endID := mgobson.NewObjectIdWithTime(end)

	testStats, err := GetLatestDailyTestDoc(opts.ProjectID, []string{opts.Requester})
	if err != nil {
		return errors.Wrap(err, "Failed to Get lastest hourly test doc")
	}

	// If we have already processed the object ids in this range. Say the
	// task died before it updated the daily stats status.
	if testStats != nil {
		if testStats.LastID >= endID {
			return nil
		}

		currentID = testStats.LastID
	}

	for {
		grip.Info(message.Fields{
			"message":         "Generating daily test stats using merge progress",
			"project":         opts.ProjectID,
			"requester":       opts.Requester,
			"hour":            opts.Window,
			"tasks":           opts.Tasks,
			"ignore":          opts.TasksToIgnore,
			"MergeBatchLimit": opts.MergeBatchLimit,
			"start":           start,
			"current":         currentID.Time(),
			"end":             endID.Time(),
		})

		pipeline := dailyTestStatsMergePipeline([]string{opts.ProjectID}, []string{opts.Requester}, opts.TasksToIgnore, opts.Runtime, opts.MergeBatchLimit, currentID, endID)
		output := []bson.M{}
		err = db.Aggregate(testresult.Collection, pipeline, &output)

		if err != nil {
			return errors.Wrap(err, "Failed to generate hourly stats")
		}

		// Get the last processed Test Doc
		testStats, err = GetLatestDailyTestDoc(opts.ProjectID, []string{opts.Requester})
		if err != nil {
			return errors.Wrap(err, "Failed to Get lastest daily test doc")
		}

		if testStats == nil {
			break
		}

		if testStats.LastID == currentID {
			break
		}
		currentID = testStats.LastID
		time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
	}
	return nil
}

///////////////////////////////////////////
// Daily task stats generation functions //
///////////////////////////////////////////

// GenerateDailyTaskStats aggregates the hourly task stats present in the database and saves
// the resulting daily task stats documents for the project, requester, day, and tasks specified.
// The day covered is the UTC day corresponding to the given `day` parameter.
func GenerateDailyTaskStats(ctx context.Context, opts GenerateOptions) error {
	grip.Info(message.Fields{
		"message":   "Generating daily task stats",
		"project":   opts.ProjectID,
		"requester": opts.Requester,
		"hour":      opts.Window,
		"tasks":     opts.Tasks,
	})
	start := util.GetUTCDay(opts.Window)
	end := start.Add(24 * time.Hour)
	pipeline := dailyTaskStatsPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
	err := aggregateIntoCollection(ctx, task.Collection, pipeline, DailyTaskStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate daily task stats")
	}

	if !opts.DisableOldTasks {
		grip.Info(message.Fields{
			"message":   "Generating daily task stats from old tasks",
			"project":   opts.ProjectID,
			"requester": opts.Requester,
			"hour":      opts.Window,
			"tasks":     opts.Tasks,
		})
		pipeline = dailyTaskStatsForOldTasksPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
		err = aggregateIntoCollection(ctx, task.OldCollection, pipeline, DailyTaskStatsCollection)
		if err != nil {
			return errors.Wrap(err, "Failed to aggregate daily task stats")
		}
	}

	return nil
}

//////////////////////////////////////////////////////
// Functions to find which stats need to be updated //
//////////////////////////////////////////////////////

type StatsToUpdate struct {
	ProjectId string    `bson:"project"`
	Requester string    `bson:"requester"`
	Hour      time.Time `bson:"date"`
	Day       time.Time `bson:"day"`
	Tasks     []string  `bson:"task_names"`
}

func (s *StatsToUpdate) canMerge(other *StatsToUpdate) bool {
	return s.ProjectId == other.ProjectId && s.Requester == other.Requester && s.Hour.UTC() == other.Hour.UTC()
}

// lt returns true if this StatsToUpdate should be sorted before the other.
func (s *StatsToUpdate) lt(other *StatsToUpdate) bool {
	if s.ProjectId < other.ProjectId {
		return true
	} else if s.ProjectId == other.ProjectId {
		if s.Hour.UnixNano() < other.Hour.UnixNano() {
			return true
		} else if s.Hour.UnixNano() == other.Hour.UnixNano() {
			if s.Requester < other.Requester {
				return true
			}
		}
	}
	return false
}

// merge merges two StatsToUpdate.
// This method does not check that the objects can be merged.
func (s *StatsToUpdate) merge(other *StatsToUpdate) StatsToUpdate {
	tasks := s.Tasks
	for _, t := range other.Tasks {
		if !containsTask(tasks, t) {
			tasks = append(tasks, t)
		}
	}
	return StatsToUpdate{s.ProjectId, s.Requester, s.Hour, s.Day, tasks}
}

// containsTask indicates if a list of strings contains a specific string.
func containsTask(tasks []string, task string) bool {
	for _, t := range tasks {
		if t == task {
			return true
		}
	}
	return false
}

type FindStatsOptions struct {
	ProjectID       string
	Requesters      []string
	Start           time.Time
	End             time.Time
	DisableOldTasks bool
}

// FindTestResultsToUpdate finds the stats that need to be updated as a result of tasks finishing between 'start' and 'end'.
// The results are ordered by project id, then hour, then requester.
func FindTestResultsToUpdate(opts FindStatsOptions) ([]StatsToUpdate, error) {
	grip.Info(message.Fields{
		"message": "Finding tasks that need their stats updated",
		"project": opts.ProjectID,
		"start":   opts.Start,
		"end":     opts.End,
	})

	pipeline := statsToUpdatePipeline(opts.ProjectID, opts.Requesters, opts.Start, opts.End)
	statsList := []StatsToUpdate{}
	err := db.Aggregate(task.Collection, pipeline, &statsList)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate finished tasks")
	}

	statsListForOldTasks := []StatsToUpdate{}
	if !opts.DisableOldTasks {
		err = db.Aggregate(task.OldCollection, pipeline, &statsListForOldTasks)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to aggregate finished old tasks")
		}
	}
	return mergeStatsToUpdateLists(statsList, statsListForOldTasks), nil
}

// FindStatsToUpdate finds the stats that need to be updated as a result of tasks finishing between 'start' and 'end'.
// The results are ordered by project id, then hour, then requester.
func FindStatsToUpdate(opts FindStatsOptions) ([]StatsToUpdate, error) {
	grip.Info(message.Fields{
		"message": "Finding tasks that need their stats updated",
		"project": opts.ProjectID,
		"start":   opts.Start,
		"end":     opts.End,
	})
	pipeline := statsToUpdatePipeline(opts.ProjectID, opts.Requesters, opts.Start, opts.End)
	taskStatsToUpdate := []StatsToUpdate{}

	err := db.Aggregate(task.Collection, pipeline, &taskStatsToUpdate)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate finished tasks")
	}

	statsListForOldTasks := []StatsToUpdate{}
	if !opts.DisableOldTasks {
		err = db.Aggregate(task.OldCollection, pipeline, &statsListForOldTasks)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to aggregate finished old tasks")
		}
	}
	taskStatsToUpdate = mergeStatsToUpdateLists(taskStatsToUpdate, statsListForOldTasks)
	return taskStatsToUpdate, nil
}

// mergeStatsToUpdateLists takes 2 sorted lists of StatsToUpdate and merge their results.
// The original list elements may be modified.
func mergeStatsToUpdateLists(statsList []StatsToUpdate, statsListOld []StatsToUpdate) []StatsToUpdate {
	length := len(statsList)
	lengthOld := len(statsListOld)
	if length == 0 {
		return statsListOld
	} else if lengthOld == 0 {
		return statsList
	}
	mergedList := []StatsToUpdate{}
	var element StatsToUpdate
	var elementOld StatsToUpdate
	index := 0
	indexOld := 0
	for index < length && indexOld < lengthOld {
		element = statsList[index]
		elementOld = statsListOld[indexOld]
		if element.canMerge(&elementOld) {
			mergedList = append(mergedList, element.merge(&elementOld))
			index += 1
			indexOld += 1
		} else if element.lt(&elementOld) {
			mergedList = append(mergedList, element)
			index += 1
		} else {
			mergedList = append(mergedList, elementOld)
			indexOld += 1
		}
		if index == length {
			mergedList = append(mergedList, statsListOld[indexOld:]...)
			break
		} else if indexOld == lengthOld {
			mergedList = append(mergedList, statsList[index:]...)
			break
		}
	}
	return mergedList
}
