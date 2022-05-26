package stats

import (
	"bytes"
	"context"
	"database/sql"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// testStatsQuery returns aggregated statistics based on the daily test
	// statistics stored in Presto.
	testStatsQuery = `
WITH matched_and_aggregated AS (
    SELECT  test_name,{{ if not .GroupByTest }}
            task_name,{{ end }}
	    {{ if .GroupDays }}MIN(task_create_iso) AS task_create_iso{{ else }}task_create_iso{{ end }},
	    SUM(num_pass) AS num_pass,
	    SUM(num_fail) AS num_fail,
	    SUM(total_pass_duration_ns) AS total_pass_duration_ns
    FROM v__results__daily_test_stats__v1
    WHERE project = ?
    AND   variant = ?{{ if .TaskName }}
    AND   task_name = ?{{ end }}{{ if .TestName }}
    AND   test_name = ?{{ end }}
    AND   request_type IN (?{{ range slice .Requesters 1}}, ?{{end}})
    AND   task_create_iso BETWEEN ? AND ?
    GROUP BY test_name{{ if not .GroupByTest }}, task_name{{ end }}{{ if not .GroupDays }}, task_create_iso{{ end }}
    ORDER BY task_create_iso{{ if .SortDesc }} DESC{{ end }},{{ if not .GroupByTest }} task_name ASC,{{ end }} test_name ASC
    OFFSET ?
    LIMIT ?
)
SELECT  test_name,{{ if not .GroupByTest }}
        task_name,{{ end }}
	date(task_create_iso) AS date,
        num_pass,
        num_fail,
        IF(num_pass <> 0, total_pass_duration_ns/num_pass, 0) AS average_duration
FROM matched_and_aggregated
`
	dateFormat = "2006-01-02"
)

var testStatsQueryTemplate = template.Must(template.New("test_stats_api").Parse(testStatsQuery))

// PrestoTestStatsFilter represents search and aggregation parameters when
// querying test statistics in Presto.
type PrestoTestStatsFilter struct {
	Project    string
	Variant    string
	TaskName   string
	TestName   string
	Requesters []string

	AfterDate  time.Time
	BeforeDate time.Time

	Offset   int
	Limit    int
	SortDesc bool

	GroupByTest bool
	GroupDays   bool

	DB *sql.DB
}

// Validate checks that the filter is valid and, if necessary, populates
// default values.
func (f *PrestoTestStatsFilter) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(f.Project == "", "must specify a project")
	catcher.NewWhen(f.Variant == "", "must specify a variant")
	catcher.NewWhen(f.TaskName == "" && f.TestName == "", "must specify a task name and/or test name")
	if len(f.Requesters) == 0 {
		f.Requesters = []string{evergreen.RepotrackerVersionRequester}
	}

	today := utility.GetUTCDay(time.Now())
	f.AfterDate = utility.GetUTCDay(f.AfterDate)
	f.BeforeDate = utility.GetUTCDay(f.BeforeDate)
	catcher.NewWhen(f.AfterDate.After(f.BeforeDate), "before date must be earlier or equal to after date")
	catcher.NewWhen(today.Sub(f.AfterDate) > 180*24*time.Hour, "must specify an after date within 180 days from today")
	if f.BeforeDate.After(today) {
		f.BeforeDate = today
	}

	if f.Limit == 0 {
		f.Limit = MaxQueryLimit
	}
	catcher.NewWhen(f.Offset < 0, "offset cannot be negative")
	catcher.NewWhen(f.Limit < 0, "limit cannot be negative")
	catcher.ErrorfWhen(f.Limit > MaxQueryLimit, "limit cannot exceed %d", MaxQueryLimit)

	if f.DB == nil {
		f.DB = evergreen.GetEnvironment().Settings().Presto.DB()
	}

	return errors.Wrap(catcher.Resolve(), "invalid Presto test stats filter")
}

// GenerateQuery creates a prepared SQL query and arguments slice based on the
// filter. The returned query and arguments can be passed directly into many
// `database/sql.DB` functions such as `Query` and `Exec`.
func (f PrestoTestStatsFilter) GenerateQuery() (queryString string, args []interface{}, err error) {
	if err = f.Validate(); err != nil {
		return
	}

	var query bytes.Buffer
	if err = testStatsQueryTemplate.Execute(&query, f); err != nil {
		err = errors.Wrap(err, "executing test stats query template")
		return
	}
	queryString = query.String()

	args = []interface{}{f.Project, f.Variant}
	if f.TaskName != "" {
		args = append(args, f.TaskName)
	}
	if f.TestName != "" {
		args = append(args, f.TestName)
	}
	for _, requester := range f.Requesters {
		args = append(args, requester)
	}
	args = append(
		args,
		f.AfterDate.Format(dateFormat),
		f.BeforeDate.Format(dateFormat),
		f.Offset,
		f.Limit,
	)

	return
}

// GetPrestoTestStats queries the precomputed test statistics in Presto using
// the given filter.
func GetPrestoTestStats(ctx context.Context, filter PrestoTestStatsFilter) ([]TestStats, error) {
	query, args, err := filter.GenerateQuery()
	if err != nil {
		return nil, err
	}

	rows, err := filter.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "executing test stats query")
	}
	defer rows.Close()

	var stats []TestStats
	for rows.Next() {
		s := TestStats{BuildVariant: filter.Variant}
		dests := []interface{}{&s.TestFile}
		if !filter.GroupByTest {
			dests = append(dests, &s.TaskName)
		}
		dests = append(dests, &s.Date, &s.NumPass, &s.NumFail, &s.AvgDurationPass)

		if err := rows.Scan(dests...); err != nil {
			return nil, errors.Wrap(err, "scanning test stats row")
		}

		// Durations from Presto are in nanoseconds, we need to convert
		// them to seconds.
		s.AvgDurationPass = time.Duration(s.AvgDurationPass).Seconds()
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "reading test stats")
	}

	return stats, nil
}
