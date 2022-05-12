package stats

import (
	"bytes"
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrestoTestStatsFilterGenerateQuery(t *testing.T) {
	now := time.Now()

	for _, test := range []struct {
		name           string
		filter         PrestoTestStatsFilter
		expectedFilter PrestoTestStatsFilter
		hasErr         bool
	}{
		{
			name: "EmptyProject",
			filter: PrestoTestStatsFilter{
				Variant:    "variant",
				TaskName:   "task",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now,
			},
			hasErr: true,
		},
		{
			name: "EmptyVariant",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				TaskName:   "task",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now,
			},
			hasErr: true,
		},
		{
			name: "EmptyTaskNameAndTestName",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now,
			},
			hasErr: true,
		},
		{
			name: "BeforeDateLessThanAfterDate",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now.Add(-72 * time.Hour),
			},
			hasErr: true,
		},
		{
			name: "AfterDateMoreThan180DaysAgo",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				AfterDate:  now.Add(-181 * 24 * time.Hour),
				BeforeDate: now,
			},
			hasErr: true,
		},
		{
			name: "NegativeOffset",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now,
				Offset:     -1,
			},
			hasErr: true,
		},
		{
			name: "NegativeLimit",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now,
				Limit:      -1,
			},
			hasErr: true,
		},
		{
			name: "LimitGreaterThanMaxQueryLimit",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now,
				Limit:      MaxQueryLimit + 1,
			},
			hasErr: true,
		},
		{
			name: "DefaultValues",
			filter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				TestName:   "test",
				AfterDate:  now.Add(-48 * time.Hour),
				BeforeDate: now.Add(24 * time.Hour),
			},
			expectedFilter: PrestoTestStatsFilter{
				Project:    "project",
				Variant:    "variant",
				TaskName:   "task",
				TestName:   "test",
				Requesters: []string{evergreen.RepotrackerVersionRequester},
				AfterDate:  utility.GetUTCDay(now.Add(-48 * time.Hour)),
				BeforeDate: utility.GetUTCDay(now),
				Limit:      MaxQueryLimit,
			},
		},
		{
			name: "ConfiguredValueNoTestName",
			filter: PrestoTestStatsFilter{
				Project:     "project",
				Variant:     "variant",
				TaskName:    "task",
				Requesters:  []string{evergreen.TriggerRequester},
				AfterDate:   now.Add(-48 * time.Hour),
				BeforeDate:  now.Add(-24 * time.Hour),
				Offset:      200,
				Limit:       100,
				SortDesc:    true,
				GroupByTest: true,
				GroupDays:   true,
			},
			expectedFilter: PrestoTestStatsFilter{
				Project:     "project",
				Variant:     "variant",
				TaskName:    "task",
				Requesters:  []string{evergreen.TriggerRequester},
				AfterDate:   utility.GetUTCDay(now.Add(-48 * time.Hour)),
				BeforeDate:  utility.GetUTCDay(now.Add(-24 * time.Hour)),
				Offset:      200,
				Limit:       100,
				SortDesc:    true,
				GroupByTest: true,
				GroupDays:   true,
			},
		},
		{
			name: "ConfiguredValuesNoTaskName",
			filter: PrestoTestStatsFilter{
				Project:     "project",
				Variant:     "variant",
				TestName:    "test",
				Requesters:  evergreen.PatchRequesters,
				AfterDate:   now.Add(-48 * time.Hour),
				BeforeDate:  now.Add(-24 * time.Hour),
				Offset:      200,
				Limit:       100,
				SortDesc:    true,
				GroupByTest: true,
				GroupDays:   true,
			},
			expectedFilter: PrestoTestStatsFilter{
				Project:     "project",
				Variant:     "variant",
				TestName:    "test",
				Requesters:  evergreen.PatchRequesters,
				AfterDate:   utility.GetUTCDay(now.Add(-48 * time.Hour)),
				BeforeDate:  utility.GetUTCDay(now.Add(-24 * time.Hour)),
				Offset:      200,
				Limit:       100,
				SortDesc:    true,
				GroupByTest: true,
				GroupDays:   true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, args, err := test.filter.GenerateQuery()
			if test.hasErr {
				require.Error(t, err)
				assert.Empty(t, query)
			} else {
				var expectedQuery bytes.Buffer
				require.NoError(t, testStatsQueryTemplate.Execute(&expectedQuery, test.expectedFilter))
				expectedArgs := []interface{}{test.expectedFilter.Project, test.expectedFilter.Variant}
				if test.expectedFilter.TaskName != "" {
					expectedArgs = append(expectedArgs, test.expectedFilter.TaskName)
				}
				if test.expectedFilter.TestName != "" {
					expectedArgs = append(expectedArgs, test.expectedFilter.TestName)
				}
				for _, requester := range test.expectedFilter.Requesters {
					expectedArgs = append(expectedArgs, requester)
				}
				expectedArgs = append(
					expectedArgs,
					test.expectedFilter.AfterDate.Format(dateFormat),
					test.expectedFilter.BeforeDate.Format(dateFormat),
					test.expectedFilter.Offset,
					test.expectedFilter.Limit,
				)

				require.NoError(t, err)
				assert.Equal(t, expectedQuery.String(), query)
				assert.Equal(t, expectedArgs, args)
			}
		})
	}
}

func TestGetPrestoTestStats(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)

	// We need to convert the query args (returned as []interface{}) to
	// []driver.Value so we can pass in the args variadically to the mock
	// DB.
	mockArgs := func(args []interface{}) []driver.Value {
		var mockArgs []driver.Value
		for _, arg := range args {
			mockArgs = append(mockArgs, arg)
		}
		return mockArgs
	}

	t.Run("NoGrouping", func(t *testing.T) {
		now := time.Now()
		filter := PrestoTestStatsFilter{
			Project:    "project",
			Variant:    "variant",
			TaskName:   "task",
			AfterDate:  utility.GetUTCDay(now).Add(-8 * 24 * time.Hour),
			BeforeDate: utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
			DB:         db,
		}
		query, args, err := filter.GenerateQuery()
		require.NoError(t, err)

		expectedResult := []TestStats{
			{
				TestFile:        "test1",
				TaskName:        "task",
				BuildVariant:    "variant",
				Date:            utility.GetUTCDay(now).Add(-5 * 24 * time.Hour),
				NumPass:         5,
				NumFail:         1,
				AvgDurationPass: (10 * time.Minute).Seconds(),
			},
			{
				TestFile:        "test2",
				TaskName:        "task",
				BuildVariant:    "variant",
				Date:            utility.GetUTCDay(now).Add(-4 * 24 * time.Hour),
				NumPass:         10,
				NumFail:         0,
				AvgDurationPass: (5 * time.Minute).Seconds(),
			},
			{
				TestFile:     "test1",
				TaskName:     "task",
				BuildVariant: "variant",
				Date:         utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
				NumPass:      0,
				NumFail:      6,
			},
			{
				TestFile:     "test2",
				TaskName:     "task",
				BuildVariant: "variant",
				Date:         utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
				NumPass:      0,
				NumFail:      10,
			},
		}
		rows := sqlmock.NewRows([]string{"test_name", "task_name", "date", "num_pass", "num_fail", "average_duration"})
		for _, row := range expectedResult {
			rows.AddRow(row.TestFile, row.TaskName, row.Date, row.NumPass, row.NumFail, row.AvgDurationPass*1e9)
		}
		mock.ExpectQuery(query).WithArgs(mockArgs(args)...).WillReturnRows(rows)

		result, err := GetPrestoTestStats(context.Background(), filter)
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})
	t.Run("GroupByTest", func(t *testing.T) {
		now := time.Now()
		filter := PrestoTestStatsFilter{
			Project:     "project",
			Variant:     "variant",
			TaskName:    "task",
			AfterDate:   utility.GetUTCDay(now).Add(-8 * 24 * time.Hour),
			BeforeDate:  utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
			GroupByTest: true,
			DB:          db,
		}
		query, args, err := filter.GenerateQuery()
		require.NoError(t, err)

		expectedResult := []TestStats{
			{
				TestFile:        "test1",
				BuildVariant:    "variant",
				Date:            utility.GetUTCDay(now).Add(-5 * 24 * time.Hour),
				NumPass:         5,
				NumFail:         1,
				AvgDurationPass: (10 * time.Second).Seconds(),
			},
			{
				TestFile:        "test2",
				BuildVariant:    "variant",
				Date:            utility.GetUTCDay(now).Add(-4 * 24 * time.Hour),
				NumPass:         10,
				NumFail:         0,
				AvgDurationPass: (5 * time.Second).Seconds(),
			},
			{
				TestFile:     "test1",
				BuildVariant: "variant",
				Date:         utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
				NumPass:      0,
				NumFail:      6,
			},
			{
				TestFile:     "test2",
				BuildVariant: "variant",
				Date:         utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
				NumPass:      0,
				NumFail:      10,
			},
		}
		rows := sqlmock.NewRows([]string{"test_name", "date", "num_pass", "num_fail", "average_duration"})
		for _, row := range expectedResult {
			rows.AddRow(row.TestFile, row.Date, row.NumPass, row.NumFail, row.AvgDurationPass*1e9)
		}
		mock.ExpectQuery(query).WithArgs(mockArgs(args)...).WillReturnRows(rows)

		result, err := GetPrestoTestStats(context.Background(), filter)
		require.NoError(t, err)
		assert.Equal(t, expectedResult, result)
	})
	t.Run("QueryError", func(t *testing.T) {
		now := time.Now()
		filter := PrestoTestStatsFilter{
			Project:    "project",
			Variant:    "variant",
			TaskName:   "task",
			AfterDate:  utility.GetUTCDay(now).Add(-8 * 24 * time.Hour),
			BeforeDate: utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
			DB:         db,
		}
		query, args, err := filter.GenerateQuery()
		require.NoError(t, err)

		mock.ExpectQuery(query).WithArgs(mockArgs(args)...).WillReturnError(errors.New("query error"))

		result, err := GetPrestoTestStats(context.Background(), filter)
		require.Error(t, err)
		assert.Nil(t, result)
	})
	t.Run("RowError", func(t *testing.T) {
		now := time.Now()
		filter := PrestoTestStatsFilter{
			Project:    "project",
			Variant:    "variant",
			TaskName:   "task",
			AfterDate:  utility.GetUTCDay(now).Add(-8 * 24 * time.Hour),
			BeforeDate: utility.GetUTCDay(now).Add(-1 * 24 * time.Hour),
			DB:         db,
		}
		query, args, err := filter.GenerateQuery()
		require.NoError(t, err)

		rows := sqlmock.NewRows([]string{"test_name", "task_name", "date", "num_pass", "num_fail", "average_duration"}).
			AddRow("test", "task", utility.GetUTCDay(now).Add(-2*24*time.Hour), 0, 5, 0).
			AddRow("test2", "task", utility.GetUTCDay(now).Add(-2*24*time.Hour), 5, 5, 1000000).
			RowError(1, errors.New("row error"))
		mock.ExpectQuery(query).WithArgs(mockArgs(args)...).WillReturnRows(rows)

		result, err := GetPrestoTestStats(context.Background(), filter)
		require.Error(t, err)
		assert.Nil(t, result)
	})
}
