// Package stats provides functions to generate and query pre-computed test and task statistics.
// The statistics are aggregated per day and a combination of (test, task, variant, distro, project, requester) for
// tests and a combination of (task, variant, distro,  project, requester) for tasks.
// For tests intermediate hourly statistics are also stored to avoid some re-computation.
package stats

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	defaultBackFillStart := utility.GetUTCDay(time.Now().Add(-defaultBackFillPeriod))
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
	q := db.Query(statsStatusQuery(projectId))
	err := db.FindOneQ(DailyStatsStatusCollection, q, &status)
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
	_, err := db.Upsert(DailyStatsStatusCollection, bson.M{"_id": projectId}, status)
	if err != nil {
		return errors.Wrap(err, "Failed to update test stats status")
	}
	return nil
}

//////////////////////////////////////////////////////
// Hourly and daily test stats generation functions //
//////////////////////////////////////////////////////

type GenerateOptions struct {
	ProjectID string
	Requester string
	Tasks     []string
	Window    time.Time
	Runtime   time.Time
}

// GenerateHourlyTestStats aggregates task and testresults prsent in the database and saves the
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
	start := utility.GetUTCHour(opts.Window)
	end := start.Add(time.Hour)
	// Generate the stats based on tasks.
	pipeline := hourlyTestStatsPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
	err := aggregateIntoCollection(ctx, task.Collection, pipeline, HourlyTestStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to generate hourly stats")
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
	start := utility.GetUTCDay(opts.Window)
	end := start.Add(24 * time.Hour)
	pipeline := dailyTestStatsFromHourlyPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
	err := aggregateIntoCollection(ctx, HourlyTestStatsCollection, pipeline, DailyTestStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate hourly stats into daily stats")
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
	start := utility.GetUTCDay(opts.Window)
	end := start.Add(24 * time.Hour)
	pipeline := dailyTaskStatsPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks, opts.Runtime)
	err := aggregateIntoCollection(ctx, task.Collection, pipeline, DailyTaskStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate daily task stats")
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
	ProjectID  string
	Requesters []string
	Start      time.Time
	End        time.Time
}

// FidnStatsToUpdate finds the stats that need to be updated as a result of tasks finishing between 'start' and 'end'.
// The results are ordered by project id, then hour, then requester.
func FindStatsToUpdate(opts FindStatsOptions) ([]StatsToUpdate, error) {
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

	return statsList, nil
}
