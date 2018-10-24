// Provides functions to generate and query pre-computed test and task statistics.
// The statistics are aggregated per day and a combination of (test, task, variant, distro, project, requester) for
// tests and a combination of (task, variant, distro,  project, requester) for tasks.
// For tests intermediate hourly statistics are also stored to avoid some re-computation.
package stats

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	fourWeeks = 4 * 7 * 24 * time.Hour
)

// Represents the status for stats pre-computations for a project.
// LastJobRun is the start date of the last successful pre-computation job that ran for the project.
// ProcessedTasksUntil is the date before which all finished tasks have been processed. It is usually
// the same as LastJobRun unless a previous job has failed and the stats computation has not caught up yet.
type StatsStatus struct {
	ProjectId           string    `bson:"_id"`
	LastJobRun          time.Time `bson:"last_job_run"`
	ProcessedTasksUntil time.Time `bson:"processed_tasks_until"`
}

func createDefaultStatsStatus(projectId string) StatsStatus {
	fourWeeksAgo := getUTCDay(time.Now().Add(-fourWeeks))
	return StatsStatus{
		ProjectId:           projectId,
		LastJobRun:          fourWeeksAgo,
		ProcessedTasksUntil: fourWeeksAgo,
	}
}

////////////////////////////
// Stats status functions //
////////////////////////////

// Retrieves the status of the stats pre-computations for a project.
func GetStatsStatus(projectId string) (StatsStatus, error) {
	status := StatsStatus{}
	query := statsStatusQuery(projectId)
	err := db.FindOne(dailyStatsStatusCollection, query, db.NoProjection, db.NoSort, &status)
	if err == mgo.ErrNotFound {
		return createDefaultStatsStatus(projectId), nil
	}
	if err != nil {
		return status, errors.Wrap(err, "Failed to retrieve test stats status")
	}
	return status, nil
}

// Updates the status of the stats pre-computations for a project.
func UpdateStatsStatus(projectId string, lastJobRun time.Time, processedTasksUntil time.Time) error {
	status := StatsStatus{
		ProjectId:           projectId,
		LastJobRun:          lastJobRun,
		ProcessedTasksUntil: processedTasksUntil,
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

// Aggregates task and testresults prsent in the database and saves the resulting hourly test stats documents
// for the project, requester, hour, and tasks specified.
// The hour covered is the UTC hour corresponding to the given `hour` parameter.
func GenerateHourlyTestStats(projectId string, requester string, hour time.Time, tasks []string, jobRunTime time.Time) error {
	grip.Infof("Generating hourly test stats for (%v, %v, %v, %v)", projectId, requester, hour, tasks)
	start := getUTCHour(hour)
	end := start.Add(time.Hour)
	// Generate the stats based on tasks.
	pipeline := hourlyTestStatsPipeline(projectId, requester, start, end, tasks, jobRunTime)
	err := aggregateIntoCollection(tasksCollection, pipeline, hourlyTestStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to generate hourly stats")
	}

	grip.Infof("Generating hourly test stats from old tasks for (%v, %v, %v, %v)", projectId, requester, hour, tasks)
	// Generate/Update the stats for old tasks.
	pipeline = hourlyTestStatsForOldTasksPipeline(projectId, requester, start, end, tasks, jobRunTime)
	err = aggregateIntoCollection(oldTasksCollection, pipeline, hourlyTestStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to generate hourly stats for old tasks")
	}
	return nil
}

// Aggregates the hourly test stats present in the database and saves the resulting daily test stats documents
// for the project, requester, day, and tasks specified.
// The day covered is the UTC day corresponding to the given `day` parameter.
func GenerateDailyTestStatsFromHourly(projectId string, requester string, day time.Time, tasks []string, jobRunTime time.Time) error {
	grip.Infof("Generating daily test stats for (%v, %v, %v, %v)", projectId, requester, day, tasks)
	start := getUTCDay(day)
	end := start.Add(24 * time.Hour)
	pipeline := dailyTestStatsFromHourlyPipeline(projectId, requester, start, end, tasks, jobRunTime)
	err := aggregateIntoCollection(hourlyTestStatsCollection, pipeline, dailyTestStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate hourly stats into daily stats")
	}
	return nil
}

///////////////////////////////////////////
// Daily task stats generation functions //
///////////////////////////////////////////

// Aggregates the hourly task stats present in the database and saves the resulting daily task stats documents
// for the project, requester, day, and tasks specified.
// The day covered is the UTC day corresponding to the given `day` parameter.
func GenerateDailyTaskStats(projectId string, requester string, day time.Time, tasks []string, jobRunTime time.Time) error {
	grip.Infof("Generating daily task stats for (%v, %v, %v, %v)", projectId, requester, day, tasks)
	start := getUTCDay(day)
	end := start.Add(24 * time.Hour)
	pipeline := dailyTaskStatsPipeline(projectId, requester, start, end, tasks, jobRunTime)
	err := aggregateIntoCollection(tasksCollection, pipeline, dailyTaskStatsCollection)
	if err != nil {
		return errors.Wrap(err, "Failed to aggregate daily task stats")
	}

	grip.Infof("Generating daily task stats from old tasks for (%v, %v, %v, %v)", projectId, requester, day, tasks)
	start = getUTCDay(day)
	end = start.Add(24 * time.Hour)
	pipeline = dailyTaskStatsForOldTasksPipeline(projectId, requester, start, end, tasks, jobRunTime)
	err = aggregateIntoCollection(oldTasksCollection, pipeline, dailyTaskStatsCollection)
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

func (s StatsToUpdate) canMerge(other StatsToUpdate) bool {
	return s.ProjectId == other.ProjectId && s.Requester == other.Requester && s.Hour.UTC() == other.Hour.UTC()
}

// Returns true if this StatsToUpdate should be sorted before the other.
func (s StatsToUpdate) lt(other StatsToUpdate) bool {
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

// Merges two StatsToUpdate.
// This method does not check that the objects can be merged.
func (s StatsToUpdate) merge(other StatsToUpdate) StatsToUpdate {
	tasks := s.Tasks
	for _, t := range other.Tasks {
		if !containsTask(tasks, t) {
			tasks = append(tasks, t)
		}
	}
	return StatsToUpdate{s.ProjectId, s.Requester, s.Hour, s.Day, tasks}
}

// Indicates if a list of strings contains a specific string.
func containsTask(tasks []string, task string) bool {
	for _, t := range tasks {
		if t == task {
			return true
		}
	}
	return false
}

// Find the stats that need to be updated as a result of tasks finishing between 'start' and 'end'.
// The results are ordered by project id, then hour, then requester.
func FindStatsToUpdate(projectId string, start time.Time, end time.Time) ([]StatsToUpdate, error) {
	grip.Infof("Finding tasks that need their stats updated for (%v, %v, %v)", projectId, start, end)
	pipeline := statsToUpdatePipeline(projectId, start, end)
	statsList := []StatsToUpdate{}
	err := db.Aggregate(tasksCollection, pipeline, &statsList)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate finished tasks")
	}
	statsListForOldTasks := []StatsToUpdate{}
	err = db.Aggregate(oldTasksCollection, pipeline, &statsListForOldTasks)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate finished old tasks")
	}
	return mergeStatsToUpdateLists(statsList, statsListForOldTasks), nil
}

// Takes 2 sorted lists of StatsToUpdate and merge their results.
// The original list elements may be modified.
func mergeStatsToUpdateLists(statsList []StatsToUpdate, statsListOld []StatsToUpdate) []StatsToUpdate {
	length := len(statsList)
	lengthOld := len(statsListOld)
	if length == 0 {
		return statsListOld
	} else if lengthOld == 0 {
		return statsList
	}
	var mergedList = []StatsToUpdate{}
	var element StatsToUpdate
	var elementOld StatsToUpdate
	index := 0
	indexOld := 0
	for index < length && indexOld < lengthOld {
		element = statsList[index]
		elementOld = statsListOld[indexOld]
		if element.canMerge(elementOld) {
			mergedList = append(mergedList, element.merge(elementOld))
			index += 1
			indexOld += 1
		} else if element.lt(elementOld) {
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

////////////////////////////
// Utility time functions //
////////////////////////////

// Creates and returns a time.Time corresponding to the start of the UTC day containing the given date.
func getUTCDay(date time.Time) time.Time {
	// Convert to UTC.
	date = date.In(time.UTC)
	// Create a new time.Time for the beginning of the day.
	year, month, day := date.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// Creates and returns a time.Time corresponding to the start of the UTC hour containing the given date.
func getUTCHour(date time.Time) time.Time {
	// Convert to UTC.
	date = date.In(time.UTC)
	// Create a new time.Time for the beginning of the hour.
	year, month, day := date.Date()
	hour := date.Hour()
	return time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
}
