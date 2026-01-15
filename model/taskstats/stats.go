// Package taskstats provides functions to generate and query pre-computed and
// task statistics. The statistics are aggregated per day and a combination of
// (project, variant, distro, task, requester).
package taskstats

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
type StatsStatus struct {
	ProjectID string `bson:"_id"`
	// LastJobRun is the start date of the last successful pre-computation
	// job that ran for the project.
	LastJobRun time.Time `bson:"last_job_run"`
	// ProcessedTasksUntil is the date before which all finished tasks have
	// been processed. It is usually the same as LastJobRun unless a
	// previous job has failed and the stats computation has not caught up
	// yet.
	ProcessedTasksUntil time.Time `bson:"processed_tasks_until"`
	// Runtime is the amount of time the last successful pre-computation
	// job that for the project took to complete.
	Runtime time.Duration `bson:"runtime"`
}

// createDefaultStatsStatus creates a StatsStatus for projects that don't have
// a status in the DB yet.
func createDefaultStatsStatus(projectID string) StatsStatus {
	defaultBackFillStart := utility.GetUTCDay(time.Now().Add(-defaultBackFillPeriod))
	return StatsStatus{
		ProjectID:           projectID,
		LastJobRun:          defaultBackFillStart,
		ProcessedTasksUntil: defaultBackFillStart,
	}
}

////////////////////////////
// Stats status functions //
////////////////////////////

// GetStatsStatus retrieves the status of the stats pre-computations for a
// project.
func GetStatsStatus(ctx context.Context, projectID string) (StatsStatus, error) {
	status := StatsStatus{}
	q := db.Query(statsStatusQuery(projectID))
	err := db.FindOneQContext(ctx, DailyStatsStatusCollection, q, &status)
	if adb.ResultsNotFound(err) {
		return createDefaultStatsStatus(projectID), nil
	}
	if err != nil {
		return status, errors.Wrap(err, "retrieving test stats status")
	}
	return status, nil
}

// UpdateStatsStatus updates the status of the stats pre-computations for a project.
func UpdateStatsStatus(ctx context.Context, projectID string, lastJobRun, processedTasksUntil time.Time, runtime time.Duration) error {
	status := StatsStatus{
		ProjectID:           projectID,
		LastJobRun:          lastJobRun,
		ProcessedTasksUntil: processedTasksUntil,
		Runtime:             runtime,
	}
	_, err := db.Replace(ctx, DailyStatsStatusCollection, bson.M{"_id": projectID}, status)
	if err != nil {
		return errors.Wrap(err, "updating test stats status")
	}
	return nil
}

// GetUpdateWindow returns the start and end of the time window for the stats.
// This size of this window is capped at 12 hours to prevent
// long-running jobs, overwhelming the database, and avoid excessive load.
func (status *StatsStatus) GetUpdateWindow() (time.Time, time.Time) {
	start := status.ProcessedTasksUntil
	end := time.Now()

	windowSize := end.Sub(start)
	cutoffDuration := 12 * time.Hour
	if windowSize >= cutoffDuration {
		end = start.Add(cutoffDuration)
	}
	return start, end
}

///////////////////////////////////////////
// Daily task stats generation functions //
///////////////////////////////////////////

type GenerateStatsOptions struct {
	ProjectID string
	Requester string
	Tasks     []string
	Date      time.Time
}

// GenerateStats aggregates the tasks in the database into task stats documents
// for the given project, requester, day, and tasks specified. The day covered
// is the UTC day corresponding to the given day parameter.
func GenerateStats(ctx context.Context, opts GenerateStatsOptions) error {
	grip.Info(message.Fields{
		"message":   "generating daily task stats",
		"project":   opts.ProjectID,
		"requester": opts.Requester,
		"day":       opts.Date,
		"tasks":     opts.Tasks,
	})
	start := utility.GetUTCDay(opts.Date)
	end := start.Add(24 * time.Hour)
	if err := aggregateIntoCollection(ctx, task.Collection, statsPipeline(opts.ProjectID, opts.Requester, start, end, opts.Tasks), DailyTaskStatsCollection); err != nil {
		return errors.Wrap(err, "aggregating daily task stats")
	}

	return nil
}

/////////////////////////////////////////////////////////////////
// Functions to find which daily task stats need to be updated //
/////////////////////////////////////////////////////////////////

type StatsToUpdate struct {
	Requester string    `bson:"requester"`
	Day       time.Time `bson:"day"`
	Tasks     []string  `bson:"task_names"`
}

type FindStatsToUpdateOptions struct {
	ProjectID  string
	Requesters []string
	Start      time.Time
	End        time.Time
}

// FindStatsToUpdate finds the stats that need to be updated as a result of
// tasks finishing between the given start and end times. The results are
// ordered are ordered by first by date, then requester.
func FindStatsToUpdate(ctx context.Context, opts FindStatsToUpdateOptions) ([]StatsToUpdate, error) {
	grip.Info(message.Fields{
		"message": "finding tasks that need their stats updated",
		"project": opts.ProjectID,
		"start":   opts.Start,
		"end":     opts.End,
	})

	var toUpdate []StatsToUpdate
	if err := db.Aggregate(ctx, task.Collection, statsToUpdatePipeline(opts.ProjectID, opts.Requesters, opts.Start, opts.End), &toUpdate); err != nil {
		return nil, errors.Wrap(err, "finding tasks that need their stats updated")
	}

	return toUpdate, nil
}
