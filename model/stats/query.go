package stats

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	MaxQueryLimit             = 1001
	GroupByTest       GroupBy = "test"
	GroupByTask       GroupBy = "task"
	GroupByVariant    GroupBy = "variant"
	GroupByDistro     GroupBy = "distro"
	SortEarliestFirst Sort    = "earliest"
	SortLatestFirst   Sort    = "latest"
)

type GroupBy string

func (gb GroupBy) validate() error {
	switch gb {
	case GroupByDistro:
	case GroupByVariant:
	case GroupByTask:
	case GroupByTest:
	default:
		return errors.Errorf("Invalid GroupBy value: %s", gb)
	}
	return nil
}

type Sort string

func (s Sort) validate() error {
	switch s {
	case SortLatestFirst:
	case SortEarliestFirst:
	default:
		return errors.Errorf("Invalid Sort value: %s", s)
	}
	return nil
}

/////////////
// Filters //
/////////////

// StartAt represents parameters that allow a search query to resume at a specific point.
// Used for pagination.
type StartAt struct {
	Date         time.Time
	BuildVariant string
	Task         string
	Test         string
	Distro       string
}

// StartAtFromTestStats creates a StartAt that can be used to resume a test stats query.
// Using the returned StartAt the given TestStats will be the first result.
func StartAtFromTestStats(testStats *TestStats) StartAt {
	startAt := StartAt{
		Date:         testStats.Date,
		BuildVariant: testStats.BuildVariant,
		Task:         testStats.TaskName,
		Test:         testStats.TestFile,
		Distro:       testStats.Distro,
	}
	return startAt
}

// StartAtFromTaskStats creates a StartAt that can be used to resume a task stats query.
// Using the returned StartAt the given TaskStats will be the first result.
func StartAtFromTaskStats(taskStats *TaskStats) StartAt {
	startAt := StartAt{
		Date:         taskStats.Date,
		BuildVariant: taskStats.BuildVariant,
		Task:         taskStats.TaskName,
		Distro:       taskStats.Distro,
	}
	return startAt
}

// validateCommon validates that the StartAt struct is valid for use with test stats.
func (s *StartAt) validateCommon(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	if s == nil {
		catcher.New("StartAt should not be nil")
	}
	if !s.Date.Equal(utility.GetUTCDay(s.Date)) {
		catcher.New("Invalid StartAt Date value")
	}
	switch groupBy {
	case GroupByDistro:
		if len(s.Distro) == 0 {
			catcher.New("Missing StartAt Distro value")
		}
		fallthrough
	case GroupByVariant:
		if len(s.BuildVariant) == 0 {
			catcher.New("Missing StartAt BuildVariant value")
		}
		fallthrough
	case GroupByTask:
		if len(s.Task) == 0 {
			catcher.New("Missing StartAt Task value")
		}
	}
	return catcher.Resolve()
}

// validateForTests validates that the StartAt struct is valid for use with test stats.
func (s *StartAt) validateForTests(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(s.validateCommon(groupBy))
	if len(s.Test) == 0 {
		catcher.New("Missing Start Test value")
	}
	return catcher.Resolve()
}

// validateForTasks validates that the StartAt struct is valid for use with task stats.
func (s *StartAt) validateForTasks(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(s.validateCommon(groupBy))
	if len(s.Test) != 0 {
		catcher.New("StartAt for task stats should not have a Test value")
	}
	return catcher.Resolve()
}

// StatsFilter represents search and aggregation parameters when querying the test or task statistics.
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

// validateCommon performs common validations regardless of the filter's intended use.
func (f *StatsFilter) ValidateCommon() error {
	catcher := grip.NewBasicCatcher()
	if f == nil {
		catcher.New("StatsFilter should not be nil")
	}

	if f.GroupNumDays <= 0 {
		catcher.New("Invalid GroupNumDays value")
	}
	if len(f.Requesters) == 0 {
		catcher.New("Missing Requesters values")
	}
	catcher.Add(f.Sort.validate())
	catcher.Add(f.GroupBy.validate())

	return catcher.Resolve()
}

// validateDates performs common date validation for test / task stats.
func (f *StatsFilter) validateDates() error {
	catcher := grip.NewBasicCatcher()
	if !f.AfterDate.Equal(utility.GetUTCDay(f.AfterDate)) {
		catcher.New("Invalid AfterDate value")
	}
	if !f.BeforeDate.Equal(utility.GetUTCDay(f.BeforeDate)) {
		catcher.New("Invalid BeforeDate value")
	}
	if !f.BeforeDate.After(f.AfterDate) {
		catcher.New("Invalid AfterDate/BeforeDate values")
	}

	return catcher.Resolve()
}

// ValidateForTests validates that the StatsFilter struct is valid for use with test stats.
func (f *StatsFilter) ValidateForTests() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(f.ValidateCommon())
	catcher.Add(f.validateDates())

	if f.Limit > MaxQueryLimit || f.Limit <= 0 {
		catcher.New("Invalid Limit value")
	}
	if f.StartAt != nil {
		catcher.Add(f.StartAt.validateForTests(f.GroupBy))
	}
	if len(f.Tests) == 0 && len(f.Tasks) == 0 {
		catcher.New("Missing Tests or Tasks values")
	}

	return catcher.Resolve()
}

// ValidateForTasks use with test stats validates that the StatsFilter struct is valid for use with task stats.
func (f *StatsFilter) ValidateForTasks() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(f.ValidateCommon())
	catcher.Add(f.validateDates())

	if f.Limit > MaxQueryLimit || f.Limit <= 0 {
		catcher.New("Invalid Limit value")
	}
	if f.StartAt != nil {
		catcher.Add(f.StartAt.validateForTasks(f.GroupBy))
	}
	if len(f.Tests) > 0 {
		catcher.New("Invalid Tests value, should be nil or empty")
	}
	if len(f.Tasks) == 0 {
		catcher.New("Missing Tasks values")
	}
	if f.GroupBy == GroupByTest {
		catcher.New("Invalid GroupBy value for a task filter")
	}

	return catcher.Resolve()

}

//////////////////////////////
// Test Statistics Querying //
//////////////////////////////

// TestStats represents test execution statistics.
type TestStats struct {
	TestFile     string    `bson:"test_file"`
	TaskName     string    `bson:"task_name"`
	BuildVariant string    `bson:"variant"`
	Distro       string    `bson:"distro"`
	Date         time.Time `bson:"date"`

	NumPass         int       `bson:"num_pass"`
	NumFail         int       `bson:"num_fail"`
	AvgDurationPass float64   `bson:"avg_duration_pass"`
	LastUpdate      time.Time `bson:"last_update"`
}

func (s *TestStats) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(s) }
func (s *TestStats) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, s) }

// GetTestStats queries the precomputed test statistics using a filter.
func GetTestStats(filter StatsFilter) ([]TestStats, error) {
	err := filter.ValidateForTests()
	if err != nil {
		return nil, errors.Wrap(err, "The provided StatsFilter is invalid")
	}
	var stats []TestStats
	pipeline := filter.testStatsQueryPipeline()
	err = db.Aggregate(DailyTestStatsCollection, pipeline, &stats)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate test statistics")
	}
	return stats, nil
}

//////////////////////////////
// Task Statistics Querying //
//////////////////////////////

// TaskStats represents task execution statistics.
type TaskStats struct {
	TaskName     string    `bson:"task_name"`
	BuildVariant string    `bson:"variant"`
	Distro       string    `bson:"distro"`
	Date         time.Time `bson:"date"`

	NumTotal           int       `bson:"num_total"`
	NumSuccess         int       `bson:"num_success"`
	NumFailed          int       `bson:"num_failed"`
	NumTimeout         int       `bson:"num_timeout"`
	NumTestFailed      int       `bson:"num_test_failed"`
	NumSystemFailed    int       `bson:"num_system_failed"`
	NumSetupFailed     int       `bson:"num_setup_failed"`
	AvgDurationSuccess float64   `bson:"avg_duration_success"`
	LastUpdate         time.Time `bson:"last_update"`
}

func (s *TaskStats) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(s) }
func (s *TaskStats) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, s) }

// GetTaskStats queries the precomputed task statistics using a filter.
func GetTaskStats(filter StatsFilter) ([]TaskStats, error) {
	err := filter.ValidateForTasks()
	if err != nil {
		return nil, errors.Wrap(err, "The provided StatsFilter is invalid")
	}
	var stats []TaskStats
	pipeline := filter.TaskStatsQueryPipeline()
	err = db.Aggregate(DailyTaskStatsCollection, pipeline, &stats)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate task statistics")
	}
	return stats, nil
}
