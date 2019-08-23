package reliability

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
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
		StatsFilter: stats.StatsFilter{
			Project:      project,
			Requesters:   requesters,
			AfterDate:    day1,
			BeforeDate:   day2,
			Tasks:        tasks,
			GroupBy:      stats.GroupByDistro,
			GroupNumDays: 1,
			Sort:         stats.SortLatestFirst,
			Limit:        MaxQueryLimit,
		},
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

func handleNoFormat(format string, i int) string {
	n := strings.Count(format, "%")
	if n > 0 {
		return fmt.Sprintf(format, i)
	}
	return format
}

func insertManyDailyTaskStats(many int, projectFmt string, requesterFmt string, taskNameFmt string, variantFmt string, distroFmt string, date time.Time, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed int, avgDuration float64) error {

	items := []interface{}{}
	for i := 0; i < many; i++ {
		items = append(items, bson.M{
			"_id": stats.DbTaskStatsId{
				Project:      handleNoFormat(projectFmt, i),
				Requester:    handleNoFormat(requesterFmt, i),
				TaskName:     handleNoFormat(taskNameFmt, i),
				BuildVariant: handleNoFormat(variantFmt, i),
				Distro:       handleNoFormat(distroFmt, i),
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
	}

	err := db.InsertManyUnordered(stats.DailyTaskStatsCollection, items...)
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

func GetTaskReliability(t *testing.T) {
	requesters := []string{
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
		evergreen.MergeTestRequester,
	}
	const (
		task1    = "task1"
		task2    = "task2"
		variant1 = "v1"
		variant2 = "v2"
		distro1  = "d1"
		distro2  = "d2"
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withCancelledContext := func(ctx context.Context, fn func(context.Context)) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		fn(ctx)
	}

	// Common DB setup. Clear the database and insert some tasks.
	// taskN only has variantN and distroN (e.g. "task1", "variant1", "distro1").
	setupDB := func(t *testing.T) {
		err := clearCollection()
		require.NoError(t, err)

		require.NoError(t, insertDailyTaskStats(project, requesters[0], task1, variant1, distro1, day1, 10, 5, 1, 1, 1, 2, 10.5))
		require.NoError(t, insertDailyTaskStats(project, requesters[0], task1, variant1, distro1, day2.Add(-1*time.Hour), 20, 7, 7, 0, 0, 0, 20.0))

		require.NoError(t, insertDailyTaskStats(project, requesters[0], task2, variant2, distro2, day1, 10, 5, 1, 1, 1, 2, 10.5))
		require.NoError(t, insertDailyTaskStats(project, requesters[0], task2, variant2, distro2, day2.Add(-1*time.Hour), 20, 7, 7, 0, 0, 0, 20.0))
	}

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		"GetTaskReliability": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter){
				"No Matches": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.Tasks = []string{"this won't match anything"}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Equal(len(docs), 0)
					})
				},
				"Non matching task1 combination": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.Tasks = []string{task1}
						filter.StatsFilter.BuildVariants = []string{variant2}
						filter.StatsFilter.Distros = []string{distro2}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 0)
					})
				},
				"Non matching task2 combination": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.Tasks = []string{task2}
						filter.StatsFilter.BuildVariants = []string{variant1}
						filter.StatsFilter.Distros = []string{distro1}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 0)
					})
				},
				"task1": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.Tasks = []string{task1}
						filter.StatsFilter.BuildVariants = []string{variant1, variant2}
						filter.StatsFilter.Distros = []string{distro1, distro2}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 2)
						for _, doc := range docs {
							require.Equal(doc.TaskName, task1)
							require.NotEqual(doc.BuildVariant, variant2)
							require.NotEqual(doc.Distro, distro2)
						}
					})
				},
				"task2": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.Tasks = []string{task2}
						filter.StatsFilter.BuildVariants = []string{variant1, variant2}
						filter.StatsFilter.Distros = []string{distro1, distro2}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 2)
						for _, doc := range docs {
							require.Equal(doc.TaskName, task2)
							require.NotEqual(doc.BuildVariant, variant1)
							require.NotEqual(doc.Distro, distro1)
						}
					})
				},
				"All Tasks Match": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.Tasks = []string{task1, task2}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.NotEqual(len(docs), 0)
					})
				},
				"MaxQueryLimit": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						task3 := "task3"
						variantFmt := "variant %04d"
						distroFmt := "distro %04d"

						// Note the withSetupAndTeardown inserts 4 documents so there are 1004 in the database after the next line runs.
						require.NoError(insertManyDailyTaskStats(MaxQueryLimit, project, "r1", task3, variantFmt, distroFmt, day1, 10, 5, 1, 1, 1, 2, 10.5))

						filter.StatsFilter.Tasks = []string{task1, task2, task3}
						filter.StatsFilter.BuildVariants = []string{}
						filter.StatsFilter.Distros = []string{}
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, MaxQueryLimit)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						setupDB(t)
						filter := createValidFilter()
						testCase(ctx, t, filter)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(ctx)
			require.NoError(t, err)

			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(tctx, t, env)
		})
	}
}

func TestValidateForTaskReliability(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	filter := TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: MaxQueryLimit + 1,
		},
		Significance: MaxSignificanceLimit + 1,
	}
	err := filter.ValidateForTaskReliability()
	require.Error(err)
	message := err.Error()
	assert.Contains(message, "Invalid Limit value")
	assert.Contains(message, "Invalid Significance value")
}
