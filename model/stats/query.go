package stats

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	MaxQueryLimit             = 1000
	GroupByTest       GroupBy = "test"
	GroupByTask       GroupBy = "task"
	GroupByVariant    GroupBy = "variant"
	GroupByDistro     GroupBy = "distro"
	SortEarliestFirst Sort    = "earliest"
	SortLatestFirst   Sort    = "latest"
)

type GroupBy string

func (gb *GroupBy) validate() error {
	switch *gb {
	case GroupByDistro:
	case GroupByVariant:
	case GroupByTask:
	case GroupByTest:
	default:
		return errors.Errorf("Invalid GroupBy value: %v", *gb)
	}
	return nil
}

type Sort string

func (s *Sort) validate() error {
	switch *s {
	case SortLatestFirst:
	case SortEarliestFirst:
	default:
		return errors.Errorf("Invalid Sort value: %v", *s)
	}
	return nil
}

/////////////
// Filters //
/////////////

// Represents parameters that allow a search query to resume at a specific point.
// Used for pagination.
type StartAt struct {
	Date         time.Time
	BuildVariant string
	Task         string
	Test         string
	Distro       string
}

// Validates that the StartAt struct is valid for use with test stats.
func (s *StartAt) validateCommon(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	if !s.Date.Equal(util.GetUTCDay(s.Date)) {
		catcher.Add(errors.New("Invalid StartAt Date value"))
	}
	switch groupBy {
	case GroupByDistro:
		if len(s.Distro) == 0 {
			catcher.Add(errors.New("Missing StartAt Distro value"))
		}
		fallthrough
	case GroupByVariant:
		if len(s.BuildVariant) == 0 {
			catcher.Add(errors.New("Missing StartAt BuildVariant value"))
		}
		fallthrough
	case GroupByTask:
		if len(s.Task) == 0 {
			catcher.Add(errors.New("Missing StartAt Task value"))
		}
	}
	return catcher.Resolve()
}

// Validates that the StartAt struct is valid for use with test stats.
func (s *StartAt) validateForTests(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(s.validateCommon(groupBy))
	if len(s.Test) == 0 {
		catcher.Add(errors.New("Missing Start Test value"))
	}
	return catcher.Resolve()
}

// Validates that the StartAt struct is valid for use with task stats.
func (s *StartAt) validateForTasks(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(s.validateCommon(groupBy))
	if len(s.Test) != 0 {
		catcher.Add(errors.New("StartAt for task stats should not have a Test value"))
	}
	return catcher.Resolve()
}

// Represents search and aggregation parameters when querying the test or task statistics.
type StatsFilter struct {
	Project    string
	Requesters []string
	AfterDate  time.Time
	BeforeDate time.Time

	Tests         []string
	Tasks         []string
	BuildVariants []string
	Distros       []string

	GroupNumDays int
	GroupBy      GroupBy
	StartAt      *StartAt
	Limit        int
	Sort         Sort
}

// Creates a StatsFilter with usual default values.
// BuildVariants and Distros default to nil.
// GroupNumDays defaults to 1.
// GroupBy defaults to GroupByDistro.
// Start defaults to nil.
// Limit defaults to MaxQueryLimit
// Sort defaults to SortLatestFirst.
func NewDefaultStatsFilter(project string, requesters []string, after time.Time, before time.Time, tests []string, tasks []string) StatsFilter {
	return StatsFilter{
		Project:       project,
		Requesters:    requesters,
		AfterDate:     after,
		BeforeDate:    before,
		Tests:         tests,
		Tasks:         tasks,
		BuildVariants: nil,
		Distros:       nil,
		GroupNumDays:  1,
		GroupBy:       GroupByDistro,
		StartAt:       nil,
		Limit:         MaxQueryLimit,
		Sort:          SortEarliestFirst,
	}
}

// Validates that the StatsFilter struct is valid for its intended use.
func (f *StatsFilter) validateCommon() error {
	catcher := grip.NewBasicCatcher()

	if f.GroupNumDays <= 0 {
		catcher.Add(errors.New("Invalid GroupNumDays value"))
	}
	if !f.AfterDate.Equal(util.GetUTCDay(f.AfterDate)) {
		catcher.Add(errors.New("Invalid AfterDate value"))
	}
	if !f.BeforeDate.Equal(util.GetUTCDay(f.BeforeDate)) {
		catcher.Add(errors.New("Invalid BeforeDate value"))
	}
	if !f.BeforeDate.After(f.AfterDate) {
		catcher.Add(errors.New("Invalid AfterDate/BeforeDate values"))
	}
	if len(f.Requesters) == 0 {
		catcher.Add(errors.New("Missing Requesters values"))
	}
	if f.Limit > MaxQueryLimit || f.Limit <= 0 {
		catcher.Add(errors.New("Invalid Limit value"))
	}
	catcher.Add(f.Sort.validate())
	catcher.Add(f.GroupBy.validate())

	return catcher.Resolve()
}

// Validates that the StatsFilter struct is valid for its intended use.
func (f *StatsFilter) validateForTests() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(f.validateCommon())
	if f.StartAt != nil {
		catcher.Add(f.StartAt.validateForTests(f.GroupBy))
	}
	if len(f.Tests) == 0 && len(f.Tasks) == 0 {
		catcher.Add(errors.New("Missing Tests or Tasks values"))
	}

	return catcher.Resolve()
}

// Validates that the StatsFilter struct is valid for its intended use.
func (f *StatsFilter) validateForTasks() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(f.validateCommon())
	if f.StartAt != nil {
		catcher.Add(f.StartAt.validateForTasks(f.GroupBy))
	}
	if len(f.Tests) > 0 {
		catcher.Add(errors.New("Invalid Tests value, should be nil or empty"))
	}
	if len(f.Tasks) == 0 {
		catcher.Add(errors.New("Missing Tasks values"))
	}
	if f.GroupBy == GroupByTest {
		catcher.Add(errors.New("Invalid GroupBy value for a task filter"))
	}

	return catcher.Resolve()

}

//////////////////////////////
// Test Statistics Querying //
//////////////////////////////

// Test execution statistics.
type TestStats struct {
	TestFile     string    `bson:"test_file" json:"test_file"`
	TaskName     string    `bson:"task_name" json:"task_name"`
	BuildVariant string    `bson:"variant" json:"variant"`
	Distro       string    `bson:"distro" json:"distro"`
	Date         time.Time `bson:"date" json:"date"`

	NumPass         int       `bson:"num_pass" json:"num_pass"`
	NumFail         int       `bson:"num_fail" json:"num_fail"`
	AvgDurationPass float32   `bson:"avg_duration_pass" json:"avg_duration_pass"`
	LastUpdate      time.Time `bson:"last_update"`
}

// Queries the precomputed test statistics using a filter.
func GetTestStats(filter *StatsFilter) ([]TestStats, error) {
	err := filter.validateForTests()
	if err != nil {
		return nil, err
	}
	var stats []TestStats
	pipeline := testStatsQueryPipeline(filter)
	err = db.Aggregate(dailyTestStatsCollection, pipeline, &stats)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate test statistics")
	}
	return stats, nil
}

//////////////////////////////
// Task Statistics Querying //
//////////////////////////////

// Task execution statistics.
type TaskStats struct {
	TaskName     string    `bson:"task_name" json:"task_name"`
	BuildVariant string    `bson:"variant" json:"variant"`
	Distro       string    `bson:"distro" json:"distro"`
	Date         time.Time `bson:"date" json:"date"`

	NumTotal           int       `bson:"num_total" json:"num_total"`
	NumSuccess         int       `bson:"num_success" json:"num_success"`
	NumFailed          int       `bson:"num_failed" json:"num_failed"`
	NumTimeout         int       `bson:"num_timeout" json:"num_timeout"`
	NumTestFailed      int       `bson:"num_test_failed" json:"num_test_failed"`
	NumSystemFailed    int       `bson:"num_system_failed" json:"num_system_failed"`
	NumSetupFailed     int       `bson:"num_setup_failed" json:"num_setup_failed"`
	AvgDurationSuccess float32   `bson:"avg_duration_success" json:"avg_duration_success"`
	LastUpdate         time.Time `bson:"last_update"`
}

// Queries the precomputed task statistics using a filter.
func GetTaskStats(filter *StatsFilter) ([]TaskStats, error) {
	err := filter.validateForTasks()
	if err != nil {
		return nil, err
	}
	var stats []TaskStats
	pipeline := taskStatsQueryPipeline(filter)
	err = db.Aggregate(dailyTaskStatsCollection, pipeline, &stats)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate task statistics")
	}
	return stats, nil
}
