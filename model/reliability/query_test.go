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
)

var day1 = time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC)
var day2 = day1.Add(24 * time.Hour)
var day8 = day1.Add(7 * 24 * time.Hour)

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
)

var requesters = []string{
	evergreen.PatchVersionRequester,
	evergreen.GithubPRRequester,
	evergreen.MergeTestRequester,
}

var task1Item1 = stats.DbTaskStats{
	Id: stats.DbTaskStatsId{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task1,
		BuildVariant: variant1,
		Distro:       distro1,
		Date:         day1,
	},
	NumSuccess:         10,
	NumFailed:          5,
	NumTimeout:         1,
	NumTestFailed:      1,
	NumSystemFailed:    1,
	NumSetupFailed:     2,
	AvgDurationSuccess: 10.5,
}

var task1Item2 = stats.DbTaskStats{
	Id: stats.DbTaskStatsId{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task1,
		BuildVariant: variant1,
		Distro:       distro1,
		Date:         day2.Add(-1 * time.Hour),
	},
	NumSuccess:         20,
	NumFailed:          7,
	NumTimeout:         7,
	NumTestFailed:      0,
	NumSystemFailed:    0,
	NumSetupFailed:     0,
	AvgDurationSuccess: 20.0,
}

var task2Item1 = stats.DbTaskStats{
	Id: stats.DbTaskStatsId{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task2,
		BuildVariant: variant2,
		Distro:       distro2,
		Date:         day2,
	},
	NumSuccess:         10,
	NumFailed:          5,
	NumTimeout:         1,
	NumTestFailed:      1,
	NumSystemFailed:    1,
	NumSetupFailed:     2,
	AvgDurationSuccess: 10.5,
}
var task2Item2 = stats.DbTaskStats{
	Id: stats.DbTaskStatsId{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task2,
		BuildVariant: variant2,
		Distro:       distro2,
		Date:         day2.Add(-1 * time.Hour),
	},
	NumSuccess:         20,
	NumFailed:          7,
	NumTimeout:         7,
	NumTestFailed:      0,
	NumSystemFailed:    0,
	NumSetupFailed:     0,
	AvgDurationSuccess: 20.0,
}

var task3item1 = stats.DbTaskStats{
	Id: stats.DbTaskStatsId{
		Project:      project,
		Requester:    requesters[0],
		TaskName:     task3,
		BuildVariant: variant2,
		Distro:       distro2,
		Date:         day1,
	},
	NumSuccess:         10,
	NumFailed:          5,
	NumTimeout:         1,
	NumTestFailed:      1,
	NumSystemFailed:    1,
	NumSetupFailed:     2,
	AvgDurationSuccess: 10.5,
}

func createValidFilter() TaskReliabilityFilter {
	tasks := []string{task1, task2}

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

func InsertDailyTaskStats(taskStats ...interface{}) error {
	err := db.InsertManyUnordered(stats.DailyTaskStatsCollection, taskStats...)
	return err
}

func handleNoFormat(format string, i int) string {
	n := strings.Count(format, "%")
	if n > 0 {
		return fmt.Sprintf(format, i)
	}
	return format
}

func InsertManyDailyTaskStats(many int, prototype stats.DbTaskStats, projectFmt string, requesterFmt string, taskNameFmt string, variantFmt string, distroFmt string) error {

	items := make([]interface{}, many, many)
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
        filter.BeforeDate = day1.Add(-24 * time.Hour)
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

func TestGetTaskReliabilityGrouped(t *testing.T) {
        require := require.New(t)
        filter := createValidFilter()
        task1 := "task1"
        task2 := "task2"

        variant1 := "v1"
        variant2 := "v2"

        distro1 := "d1"
        distro2 := "d2"

        err := clearCollection()
        require.NoError(err)

        require.NoError(insertDailyTaskStats(project, "r1", task1, variant1, distro1, day1, 10, 5, 1, 1, 1, 2, 10.5))
        require.NoError(insertDailyTaskStats(project, "r1", task1, variant1, distro1, day2.Add(-1*time.Hour), 20, 7, 7, 0, 0, 0, 20.0))

        require.NoError(insertDailyTaskStats(project, "r1", task2, variant1, distro2, day1, 10, 5, 1, 1, 1, 2, 10.5))
        require.NoError(insertDailyTaskStats(project, "r1", task2, variant1, distro2, day2.Add(-1*time.Hour), 20, 7, 7, 0, 0, 0, 20.0))

        docs, err := GetTaskReliabilityScores(filter)
        require.NoError(err)
        require.NotEqual(len(docs), 0)

        filter.StatsFilter.Tasks = []string{task1}
        filter.StatsFilter.BuildVariants = []string{variant1, variant2}
        filter.StatsFilter.Distros = []string{distro1, distro2}
        docs, err = GetTaskReliabilityScores(filter)
        require.NoError(err)
        require.Len(docs, 2)
        require.NotEqual(docs[0].Date, day1)
        require.NotEqual(docs[1].Date, day2)

        // filter.StatsFilter.Tasks = []string{task2}
        // filter.StatsFilter.BuildVariants = []string{variant1, variant2}
        // filter.StatsFilter.Distros = []string{distro1, distro2}
        // docs, err = GetTaskReliabilityScores(filter)
        // require.NoError(err)
        // require.Len(docs, 2)
        // for _, doc := range docs {
        // 	require.Equal(doc.TaskName, task2)
        // 	require.NotEqual(doc.BuildVariant, variant1)
        // 	require.NotEqual(doc.Distro, distro1)
        // }

        // filter.StatsFilter.Tasks = []string{task1}
        // filter.StatsFilter.Tasks = []string{task1}
        // filter.StatsFilter.BuildVariants = []string{variant2}
        // filter.StatsFilter.Distros = []string{distro2}
        // docs, err = GetTaskReliabilityScores(filter)
        // require.NoError(err)
        // require.Len(docs, 0)

        // filter.StatsFilter.Tasks = []string{task2}
        // filter.StatsFilter.BuildVariants = []string{variant1}
        // filter.StatsFilter.Distros = []string{distro1}
        // docs, err = GetTaskReliabilityScores(filter)
        // require.NoError(err)
        // require.Len(docs, 0)

        // task3 := "task3"
        // variantFmt := "variant %04d"
        // distroFmt := "distro %04d"

        // require.NoError(insertManyDailyTaskStats(MaxQueryLimit, project, "r1", task3, variantFmt, distroFmt, day1, 10, 5, 1, 1, 1, 2, 10.5))

        // filter.StatsFilter.Tasks = []string{task1, task2, task3}
        // filter.StatsFilter.BuildVariants = []string{}
        // filter.StatsFilter.Distros = []string{}
        // docs, err = GetTaskReliabilityScores(filter)
        // require.NoError(err)
        // require.Len(docs, MaxQueryLimit)
}

func TestGroupedResults(t *testing.T) {
        // after := time.Date(2018, 8, 12, 0, 0, 0, 0, time.UTC)
        // before := time.Date(2018, 8, 13, 0, 0, 0, 0, time.UTC)
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

        // filter := createValidFilter()
        task1 := "task1"
        task2 := "task2"

        variant1 := "v1"
        variant2 := "v2"

        distro1 := "d1"
        distro2 := "d2"

        numSuccess := 20
        numFailed := 15
        numTimeout := 1
        numTestFailed := 2
        numSystemFailed := 3
        numSetupFailed := 4
        avgDuration := 10.5

        for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
                "Grouped": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
                        for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter){
                                "1 Day / Group 1 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "1 Day / Group 1 descending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
                                        require := require.New(t)
                                        withCancelledContext(ctx, func(ctx context.Context) {
                                                filter.StatsFilter.Sort = stats.SortLatestFirst
                                                docs, err := GetTaskReliabilityScores(filter)
                                                require.NoError(err)
                                                require.Len(docs, 1)

                                                require.Equal(docs[0].Date, day1)
                                                require.Equal(docs[0].NumSuccess, numSuccess)
                                                require.Equal(docs[0].NumFailed, numFailed)
                                        })
                                },
                                "2 Day / Group 1 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "2 Day / Group 1 descending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
                                        require := require.New(t)
                                        withCancelledContext(ctx, func(ctx context.Context) {
                                                filter.StatsFilter.Sort = stats.SortLatestFirst
                                                filter.StatsFilter.GroupNumDays = 1
                                                filter.StatsFilter.BeforeDate = day2
                                                docs, err := GetTaskReliabilityScores(filter)
                                                require.NoError(err)
                                                require.Len(docs, 2)

                                                require.Equal(docs[0].Date, day2)
                                                require.Equal(docs[0].NumSuccess, numSuccess)
                                                require.Equal(docs[0].NumFailed, numFailed)

                                                require.Equal(docs[1].Date, day1)
                                                require.Equal(docs[1].NumSuccess, numSuccess)
                                                require.Equal(docs[1].NumFailed, numFailed)
                                        })
                                },
                                "2 Day / Group 2 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "2 Day / Group 2 descending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
                                        require := require.New(t)
                                        withCancelledContext(ctx, func(ctx context.Context) {
                                                filter.StatsFilter.Sort = stats.SortLatestFirst
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
                                //<--
                                "7 Day / Group 1 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "7 Day / Group 7 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                //<--
                                "28 Day / Group 1 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "28 Day / Group 28 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "56 Day / Group 56 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
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
                                "112 Day / Group 112 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
                                        require := require.New(t)
                                        withCancelledContext(ctx, func(ctx context.Context) {
                                                filter.StatsFilter.GroupNumDays = 112
                                                filter.StatsFilter.BeforeDate = day1.Add(time.Duration(filter.StatsFilter.GroupNumDays-1) * 24 * time.Hour)
                                                docs, err := GetTaskReliabilityScores(filter)
                                                require.NoError(err)
                                                require.Len(docs, 1)

                                                require.Equal(docs[0].Date, day1)
                                                require.Equal(docs[0].NumSuccess, numSuccess*filter.StatsFilter.GroupNumDays)
                                                require.Equal(docs[0].NumFailed, numFailed*filter.StatsFilter.GroupNumDays)
                                        })
                                },
                                "224 Day / Group 224 ascending": func(ctx context.Context, t *testing.T, filter TaskReliabilityFilter) {
                                        require := require.New(t)
                                        withCancelledContext(ctx, func(ctx context.Context) {
                                                filter.StatsFilter.GroupNumDays = 224
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
                                                filter := TaskReliabilityFilter{
                                                        StatsFilter: stats.StatsFilter{
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
                                                                Sort:          stats.SortEarliestFirst,
                                                        },
                                                        Significance: .05,
                                                }
                                                err := clearCollection()
                                                require.NoError(t, err)

                                                require.NoError(t, insertDailyTaskStats(project, requesters[0], task1, variant1, distro1, day1, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed, avgDuration))
                                                require.NoError(t, insertDailyTaskStats(project, requesters[0], task2, variant1, distro2, day1, numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed, avgDuration))

                                                var i int
                                                for i = 1; i < 224; i++ {
                                                        delta := time.Duration(i) * 24 * time.Hour
                                                        require.NoError(t, insertDailyTaskStats(project, requesters[0], task1, variant1, distro1, day1.Add(delta), numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed, avgDuration))
                                                        require.NoError(t, insertDailyTaskStats(project, requesters[0], task2, variant1, distro1, day2.Add(delta), numSuccess, numFailed, numTimeout, numTestFailed, numSystemFailed, numSetupFailed, avgDuration))

                                                }
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
