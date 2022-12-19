package reliability

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var day1 = time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC)
var day2 = day1.Add(24 * time.Hour)

const (
	project    = "mongodb-mongo-master"
	task1      = "task1"
	task2      = "task2"
	task3      = "task3"
	variant1   = "v1"
	variant2   = "v2"
	variantFmt = "variant %04d"
	distro1    = "d1"
	distro2    = "d2"
	distroFmt  = "distro %04d"

	numSuccess      = 10
	numFailed       = 5
	numTimeout      = 1
	numTestFailed   = 1
	numSystemFailed = 1
	numSetupFailed  = 2
	avgDuration     = 10.5

	numSuccess1      = 20
	numFailed1       = 7
	numTimeout1      = 7
	numTestFailed1   = 0
	numSystemFailed1 = 0
	numSetupFailed1  = 0
	avgDuration1     = 20.0
)

var requesters = []string{
	evergreen.PatchVersionRequester,
	evergreen.GithubPRRequester,
	evergreen.MergeTestRequester,
}

var task1Item1 = taskstats.DBTaskStats{
	Id: taskstats.DBTaskStatsID{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task1,
		BuildVariant: variant1,
		Distro:       distro1,
		Date:         day1,
	},
	NumSuccess:         numSuccess,
	NumFailed:          numFailed,
	NumTimeout:         numTimeout,
	NumTestFailed:      numTestFailed,
	NumSystemFailed:    numSystemFailed,
	NumSetupFailed:     numSetupFailed,
	AvgDurationSuccess: avgDuration,
}

var task1Item2 = taskstats.DBTaskStats{
	Id: taskstats.DBTaskStatsID{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task1,
		BuildVariant: variant1,
		Distro:       distro1,
		Date:         day2.Add(-1 * time.Hour),
	},
	NumSuccess:         numSuccess1,
	NumFailed:          numFailed1,
	NumTimeout:         numTimeout1,
	NumTestFailed:      numTestFailed1,
	NumSystemFailed:    numSystemFailed1,
	NumSetupFailed:     numSetupFailed1,
	AvgDurationSuccess: avgDuration1,
}

var task2Item1 = taskstats.DBTaskStats{
	Id: taskstats.DBTaskStatsID{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task2,
		BuildVariant: variant2,
		Distro:       distro2,
		Date:         day2,
	},
	NumSuccess:         numSuccess,
	NumFailed:          numFailed,
	NumTimeout:         numTimeout,
	NumTestFailed:      numTestFailed,
	NumSystemFailed:    numSystemFailed,
	NumSetupFailed:     numSetupFailed,
	AvgDurationSuccess: avgDuration,
}
var task2Item2 = taskstats.DBTaskStats{
	Id: taskstats.DBTaskStatsID{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task2,
		BuildVariant: variant2,
		Distro:       distro2,
		Date:         day2.Add(-1 * time.Hour),
	},
	NumSuccess:         numSuccess1,
	NumFailed:          numFailed1,
	NumTimeout:         numTimeout1,
	NumTestFailed:      numTestFailed1,
	NumSystemFailed:    numSystemFailed1,
	NumSetupFailed:     numSetupFailed1,
	AvgDurationSuccess: avgDuration1,
}

var task3item1 = taskstats.DBTaskStats{
	Id: taskstats.DBTaskStatsID{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task3,
		BuildVariant: variant2,
		Distro:       distro2,
		Date:         day1,
	},
	NumSuccess:         numSuccess,
	NumFailed:          numFailed,
	NumTimeout:         numTimeout,
	NumTestFailed:      numTestFailed,
	NumSystemFailed:    numSystemFailed,
	NumSetupFailed:     numSetupFailed,
	AvgDurationSuccess: avgDuration,
}

func createValidFilter() TaskReliabilityFilter {
	tasks := []string{task1, task2}

	return TaskReliabilityFilter{
		StatsFilter: taskstats.StatsFilter{
			Project:      project,
			Requesters:   requesters,
			AfterDate:    day1,
			BeforeDate:   day2,
			Tasks:        tasks,
			GroupBy:      taskstats.GroupByDistro,
			GroupNumDays: 1,
			Sort:         taskstats.SortLatestFirst,
			Limit:        MaxQueryLimit,
		},
		Significance: DefaultSignificance,
	}

}

func clearCollection() error {
	return db.Clear(taskstats.DailyTaskStatsCollection)
}

func InsertDailyTaskStats(taskStats ...interface{}) error {
	err := db.InsertManyUnordered(taskstats.DailyTaskStatsCollection, taskStats...)
	return err
}

func handleNoFormat(format string, i int) string {
	n := strings.Count(format, "%")
	if n > 0 {
		return fmt.Sprintf(format, i)
	}
	return format
}

func InsertManyDailyTaskStats(many int, prototype taskstats.DBTaskStats, projectFmt string, requesterFmt string, taskNameFmt string, variantFmt string, distroFmt string) error {

	items := make([]interface{}, many)
	for i := 0; i < many; i++ {
		item := prototype
		item.Id.Project = handleNoFormat(projectFmt, i)
		item.Id.Requester = handleNoFormat(requesterFmt, i)
		item.Id.TaskName = handleNoFormat(taskNameFmt, i)
		item.Id.BuildVariant = handleNoFormat(variantFmt, i)
		item.Id.Distro = handleNoFormat(distroFmt, i)
		items[i] = item
	}

	return InsertDailyTaskStats(items...)
}

func TestValidFilter(t *testing.T) {
	require := require.New(t)
	filter := createValidFilter()

	// Check that validate does not find any errors
	// For tests
	err := filter.ValidateForTaskReliability()
	require.NoError(err)
}

func TestFilterValidAfterEqualsBefore(t *testing.T) {
	var err error
	require := require.New(t)
	filter := createValidFilter()

	filter.AfterDate = day1
	filter.BeforeDate = day1
	err = filter.ValidateForTaskReliability()
	require.NoError(err)
}

func TestFilterInvalidAfterDateAfterBeforeDate(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	// With AfterDate after BeforeDate.
	filter.AfterDate = day2
	filter.BeforeDate = day1
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidAfterDateEqualBeforeDate(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	// With AfterDate equal to BeforeDate.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(-24 * time.Hour)
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidAfterDateNotUTC(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	// With AfterDate not a UTC day.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(time.Hour)
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidBeforeDateNotUTC(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	// With BeforeDate not a UTC day.
	filter.AfterDate = day1
	filter.BeforeDate = day1.Add(time.Hour)
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidForTasks(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	filter.Tasks = []string{}
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidGroupNumDays(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	filter.GroupNumDays = -1
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterMissingRequesters(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	filter.Requesters = []string{}
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidLimit(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	filter.Limit = -1
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidSort(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	filter.Sort = taskstats.Sort("invalid")
	assert.Error(filter.ValidateForTaskReliability())
}

func TestFilterInvalidGroupBy(t *testing.T) {
	assert := assert.New(t)
	filter := createValidFilter()

	filter.GroupBy = taskstats.GroupBy("invalid")
	assert.Error(filter.ValidateForTaskReliability())
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

	require.NoError(InsertDailyTaskStats(task1Item1))

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

	require.NoError(InsertDailyTaskStats(task1Item1, task1Item2))
	docs, err := GetTaskReliabilityScores(filter)
	require.NoError(err)
	require.Len(docs, 2)
	assert.Equal(docs[0].SuccessRate, float64(.56))
	assert.Equal(docs[1].SuccessRate, float64(.42))
}

func TestGetTaskReliability(t *testing.T) {

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

		require.NoError(t, InsertDailyTaskStats(task1Item1, task1Item2, task2Item1, task2Item2))
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
						require.NoError(InsertManyDailyTaskStats(MaxQueryLimit, task3item1, project, requesters[0], task3, variantFmt, distroFmt))

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

	filter := TaskReliabilityFilter{
		StatsFilter: taskstats.StatsFilter{
			Limit: MaxQueryLimit + 1,
		},
		Significance: MaxSignificanceLimit + 1,
	}
	assert.Error(filter.ValidateForTaskReliability())
}

func TestGetTaskReliabilityScores(t *testing.T) {
	requesters := []string{
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
		evergreen.MergeTestRequester,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withCancelledContext := func(ctx context.Context, fn func(context.Context)) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		fn(ctx)
	}

	commonSetup := func(ctx context.Context, t *testing.T) TaskReliabilityFilter {
		filter := TaskReliabilityFilter{
			StatsFilter: taskstats.StatsFilter{
				Project:       project,
				Requesters:    requesters,
				Tasks:         []string{task1},
				BuildVariants: []string{variant1, variant2},
				Distros:       []string{distro1, distro2},
				BeforeDate:    day1,
				AfterDate:     day1,
				GroupNumDays:  1,
				GroupBy:       "distro",
				Limit:         MaxQueryLimit,
				Sort:          taskstats.SortEarliestFirst,
			},
			Significance: .05,
		}
		err := clearCollection()
		require.NoError(t, err)

		require.NoError(t, InsertDailyTaskStats(
			taskstats.DBTaskStats{
				Id: taskstats.DBTaskStatsID{
					Project:      project,
					Requester:    requesters[0],
					TaskName:     task1,
					BuildVariant: variant1,
					Distro:       distro1,
					Date:         day1,
				},
				NumSuccess:         numSuccess,
				NumFailed:          numFailed,
				NumTimeout:         numTimeout,
				NumTestFailed:      numTestFailed,
				NumSystemFailed:    numSystemFailed,
				NumSetupFailed:     numSetupFailed,
				AvgDurationSuccess: avgDuration,
			},
			taskstats.DBTaskStats{
				Id: taskstats.DBTaskStatsID{
					Project:      project,
					Requester:    requesters[0],
					TaskName:     task2,
					BuildVariant: variant1,
					Distro:       distro2,
					Date:         day1,
				},
				NumSuccess:         numSuccess1,
				NumFailed:          numFailed1,
				NumTimeout:         numTimeout1,
				NumTestFailed:      numTestFailed1,
				NumSystemFailed:    numSystemFailed1,
				NumSetupFailed:     numSetupFailed1,
				AvgDurationSuccess: avgDuration1,
			}))

		// 60 relates to the max date range test, picked a value to ensure that there is more than enough data.
		for i := 1; i < 60; i++ {
			delta := time.Duration(i) * 24 * time.Hour
			require.NoError(t, InsertDailyTaskStats(
				taskstats.DBTaskStats{
					Id: taskstats.DBTaskStatsID{
						Project:      project,
						Requester:    requesters[0],
						TaskName:     task1,
						BuildVariant: variant1,
						Distro:       distro1,
						Date:         day1.Add(delta),
					},
					NumSuccess:         numSuccess,
					NumFailed:          numFailed,
					NumTimeout:         numTimeout,
					NumTestFailed:      numTestFailed,
					NumSystemFailed:    numSystemFailed,
					NumSetupFailed:     numSetupFailed,
					AvgDurationSuccess: avgDuration,
				},
				taskstats.DBTaskStats{
					Id: taskstats.DBTaskStatsID{
						Project:      project,
						Requester:    requesters[0],
						TaskName:     task2,
						BuildVariant: variant1,
						Distro:       distro1,
						Date:         day2.Add(delta),
					},
					NumSuccess:         numSuccess,
					NumFailed:          numFailed,
					NumTimeout:         numTimeout,
					NumTestFailed:      numTestFailed,
					NumSystemFailed:    numSystemFailed,
					NumSetupFailed:     numSetupFailed,
					AvgDurationSuccess: avgDuration,
				}))
		}
		return filter
	}

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		"GroupNumDays": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter){
				"1": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)
					})
				},
				"2": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 2
						filter.StatsFilter.BeforeDate = day2
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess*filter.StatsFilter.GroupNumDays)
						require.Equal(docs[0].NumFailed, numFailed*filter.StatsFilter.GroupNumDays)
					})
				},
				"7": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 7
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(filter.StatsFilter.GroupNumDays-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess*filter.StatsFilter.GroupNumDays)
						require.Equal(docs[0].NumFailed, numFailed*filter.StatsFilter.GroupNumDays)
					})
				},
				"28": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 28
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(filter.StatsFilter.GroupNumDays-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess*filter.StatsFilter.GroupNumDays)
						require.Equal(docs[0].NumFailed, numFailed*filter.StatsFilter.GroupNumDays)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := commonSetup(ctx, t)
						testCase(ctx, t, filter)
					})
				})
			}
		},
		"Sort": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter){
				"SortLatestFirst": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						duration := 7
						filter.StatsFilter.Sort = taskstats.SortLatestFirst
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(duration-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, duration)

						require.Equal(docs[0].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, day1)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
				"SortEarliestFirst": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						duration := 7
						filter.StatsFilter.Sort = taskstats.SortEarliestFirst
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(duration-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, duration)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := commonSetup(ctx, t)
						testCase(ctx, t, filter)
					})
				})
			}
		},
		"DateRange": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter){
				"1 Day": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)
					})
				},
				"2 Days": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day2
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 2)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)
						require.Equal(docs[1].Date, day2)
						require.Equal(docs[1].NumSuccess, numSuccess)
						require.Equal(docs[1].NumFailed, numFailed)
					})
				},
				"7 Days": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(6 * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 7)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
				"28 Days": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(27 * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 28)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
				"56 Days": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 56
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(filter.StatsFilter.GroupNumDays-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess*filter.StatsFilter.GroupNumDays)
						require.Equal(docs[0].NumFailed, numFailed*filter.StatsFilter.GroupNumDays)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := commonSetup(ctx, t)
						testCase(ctx, t, filter)
					})
				})
			}
		},
		// Test some common expected combinations.
		"Combined": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter){
				// Group by day for 1 day
				"1 Day / Group 1": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)
					})
				},
				// group by day for a week newest first.
				"7 Day / Group 1 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						duration := 7
						filter.StatsFilter.Sort = taskstats.SortLatestFirst
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(duration-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, duration)

						require.Equal(docs[0].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, day1)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
				// group by day for a week oldest first.
				"7 Day / Group 1 descending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						duration := 7
						filter.StatsFilter.Sort = taskstats.SortEarliestFirst
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(duration-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, duration)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
				// group by week for a month newest first.
				"28 Day / Group 7": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						duration := 28
						filter.StatsFilter.GroupNumDays = 1
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(duration-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, duration)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess)
						require.Equal(docs[0].NumFailed, numFailed)

						last := len(docs) - 1
						require.Equal(docs[last].Date, filter.StatsFilter.BeforeDate)
						require.Equal(docs[last].NumSuccess, numSuccess)
						require.Equal(docs[last].NumFailed, numFailed)

					})
				},
				// group by month for a month newest first.
				"28 Day / Group 28": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
					require := require.New(t)
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.GroupNumDays = 28
						filter.StatsFilter.BeforeDate = day1.Add(time.Duration(filter.StatsFilter.GroupNumDays-1) * 24 * time.Hour)
						docs, err := GetTaskReliabilityScores(filter)
						require.NoError(err)
						require.Len(docs, 1)

						require.Equal(docs[0].Date, day1)
						require.Equal(docs[0].NumSuccess, numSuccess*filter.StatsFilter.GroupNumDays)
						require.Equal(docs[0].NumFailed, numFailed*filter.StatsFilter.GroupNumDays)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := commonSetup(ctx, t)
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
			localTime := time.Local
			time.Local = nil
			defer func() {
				time.Local = localTime
			}()

			opTests(tctx, t, env)
		})
	}
}
