package reliability

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/stats"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

var day1 = time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC)
var day2 = day1.Add(24 * time.Hour)
var day8 = day1.Add(7 * 24 * time.Hour)

const (
	project = "mongodb-mongo-master"
)

func createValidFilter() TaskReliabilityFilter {
	requesters := []string{"r1", "r2"}
	tasks := []string{"task1", "task2"}

	return TaskReliabilityFilter{
		Project:      project,
		Requesters:   requesters,
		AfterDate:    day1,
		BeforeDate:   day2,
		Tasks:        tasks,
		GroupBy:      stats.GroupByDistro,
		GroupNumDays: 1,
		Sort:         stats.SortLatestFirst,
		Limit:        MaxQueryLimit,
		Significance: DefaultSignificance,
	}

}

func clearCollection() error {
	return db.Clear(stats.DailyTaskStatsCollection)
}

func insertDailyTaskStats(project string, requester string, taskName string, variant string, distro string, date time.Time, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed int, avgDuration float64) error {

	err := db.Insert(stats.DailyTaskStatsCollection, bson.M{
		"_id": stats.DbTaskStatsId{
			Project:      project,
			Requester:    requester,
			TaskName:     taskName,
			BuildVariant: variant,
			Distro:       distro,
			Date:         date,
		},
		"num_success":          numSuccess,
		"num_failed":           numFailed,
		"num_timeout":          numTimeout,
		"num_test_failed":      numTestFailed,
		"num_system_failed":    numSystemFailed,
		"num_setup_failed":     numSetupFailed,
		"avg_duration_success": avgDuration,
	})
	return err
}

func TestValidFilter(t *testing.T) {
	require := require.New(t)
	filter := createValidFilter()

	// Check that validate does not find any errors
	// For tests
	err := filter.ValidateForTaskReliability()
	require.NoError(err)
}

func TestFilterInvalidAfterDateAfterBeforeDate(t *testing.T) {
	var err error
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	// With AfterDate after BeforeDate.
	filter.AfterDate = day2
	filter.BeforeDate = day1
	err = filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Invalid AfterDate/BeforeDate values", err.Error())

}

func TestFilterInvalidAfterDateEqualBeforeDate(t *testing.T) {
	var err error
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	// With AfterDate equal to BeforeDate.
	filter.AfterDate = day1
	filter.BeforeDate = day1
	err = filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Invalid AfterDate/BeforeDate values", err.Error())

}

func TestFilterInvalidAfterDateNotUTC(t *testing.T) {
	var err error
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	// With AfterDate not a UTC day.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(time.Hour)
	err = filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Invalid BeforeDate value", err.Error())

}

func TestFilterInvalidBeforeDateNotUTC(t *testing.T) {
	var err error
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	// With BeforeDate not a UTC day.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(time.Hour)
	err = filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Invalid BeforeDate value", err.Error())
}

func TestFilterInvalidForTasks(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	filter.Tasks = []string{}
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Missing Tasks values", err.Error())
}

func TestFilterInvalidGroupNumDays(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	filter.GroupNumDays = -1
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Invalid GroupNumDays value", err.Error())
}

func TestFilterMissingRequesters(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	filter.Requesters = []string{}
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Missing Requesters values", err.Error())
}

func TestFilterInvalidLimit(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	filter.Limit = -1
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Equal("Invalid Limit value", err.Error())
}

func TestFilterInvalidSort(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	filter.Sort = stats.Sort("invalid")
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Contains(err.Error(), "Invalid Sort value:")
}

func TestFilterInvalidGroupBy(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	filter.GroupBy = stats.GroupBy("invalid")
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	assert.Contains(err.Error(), "Invalid GroupBy value:")
}

func TestGetTaskStatsEmptyCollection(t *testing.T) {
	require := require.New(t)

	err := clearCollection()
	require.NoError(err)

	filter := createValidFilter()

	docs, err := GetTaskReliabilityScores(filter)
	require.NoError(err)
	require.Empty(docs)
}

func TestGetTaskStatsOneDocument(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	err := clearCollection()
	require.NoError(err)

	err = insertDailyTaskStats(project, "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	require.NoError(err)

	docs, err := GetTaskReliabilityScores(filter)
	require.NoError(err)
	require.Len(docs, 1)
	assert.Equal(docs[0].SuccessRate, float64(.42))
}

func TestGetTaskStatsTwoDocuments(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	filter := createValidFilter()

	err := clearCollection()
	require.NoError(err)

	err = insertDailyTaskStats(project, "r1", "task1", "v1", "d1", day1, 10, 5, 1, 1, 1, 2, 10.5)
	require.NoError(err)
	err = insertDailyTaskStats(project, "r1", "task1", "v1", "d1", day2.Add(-1*time.Hour), 20, 7, 7, 0, 0, 0, 20.0)
	require.NoError(err)

	docs, err := GetTaskReliabilityScores(filter)
	require.NoError(err)
	require.Len(docs, 2)
	assert.Equal(docs[0].SuccessRate, float64(.56))
	assert.Equal(docs[1].SuccessRate, float64(.42))
}

func TestValidateForTaskReliability(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	filter := TaskReliabilityFilter{}
	filter.Limit = MaxQueryLimit + 1
	filter.Significance = MaxSignificanceLimit + 1
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	message := err.Error()
	assert.Contains(message, "Invalid Limit value")
	assert.Contains(message, "Invalid Significance value")
}
