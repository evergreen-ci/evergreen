package taskstats

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
	default:
		return errors.Errorf("invalid group by '%s'", gb)
	}
	return nil
}

type Sort string

func (s Sort) validate() error {
	switch s {
	case SortLatestFirst:
	case SortEarliestFirst:
	default:
		return errors.Errorf("invalid sort '%s'", s)
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
	Distro       string
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

// validateCommon validates that the StartAt struct is valid for use with stats.
func (s *StartAt) validateCommon(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	if s == nil {
		catcher.New("StartAt should not be nil")
	}
	if !s.Date.Equal(utility.GetUTCDay(s.Date)) {
		catcher.New("invalid 'start at' date")
	}
	switch groupBy {
	case GroupByDistro:
		if len(s.Distro) == 0 {
			catcher.New("missing distro pagination value")
		}
		fallthrough
	case GroupByVariant:
		if len(s.BuildVariant) == 0 {
			catcher.New("missing build variant pagination value")
		}
		fallthrough
	case GroupByTask:
		if len(s.Task) == 0 {
			catcher.New("missing task pagination value")
		}
	}
	return catcher.Resolve()
}

// validateForTasks validates that the StartAt struct is valid for use with task stats.
func (s *StartAt) validateForTasks(groupBy GroupBy) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(s.validateCommon(groupBy))
	return catcher.Resolve()
}

// StatsFilter represents search and aggregation parameters when querying the task statistics.
type StatsFilter struct {
	Project    string
	Requesters []string
	AfterDate  time.Time
	BeforeDate time.Time

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
	if f == nil {
		return errors.New("stats filter cannot be nil")
	}

	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(f.GroupNumDays <= 0, "invalid group num days")
	catcher.NewWhen(len(f.Requesters) == 0, "missing requesters")
	catcher.Add(f.Sort.validate())
	catcher.Add(f.GroupBy.validate())

	return catcher.Resolve()
}

// validateDates performs common date validation for task stats.
func (f *StatsFilter) validateDates() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(!f.AfterDate.Equal(utility.GetUTCDay(f.AfterDate)), "'after' date is not in UTC")
	catcher.NewWhen(!f.BeforeDate.Equal(utility.GetUTCDay(f.BeforeDate)), "'before' date is not in UTC")
	catcher.NewWhen(!f.BeforeDate.After(f.AfterDate), "'after' date restriction must be earlier than 'before' date restriction")

	return catcher.Resolve()
}

// ValidateForTasks validates that the StatsFilter struct is valid for use with
// task stats.
func (f *StatsFilter) ValidateForTasks() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(f.ValidateCommon())
	catcher.Add(f.validateDates())

	catcher.NewWhen(f.Limit > MaxQueryLimit || f.Limit <= 0, "invalid limit")
	if f.StartAt != nil {
		catcher.Add(f.StartAt.validateForTasks(f.GroupBy))
	}
	catcher.NewWhen(len(f.Tasks) == 0, "missing tasks")

	return catcher.Resolve()

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
		return nil, errors.Wrap(err, "invalid stats filter")
	}
	var stats []TaskStats
	pipeline := filter.TaskStatsQueryPipeline()
	err = db.Aggregate(DailyTaskStatsCollection, pipeline, &stats)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task statistics")
	}
	return stats, nil
}
