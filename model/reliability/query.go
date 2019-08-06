package reliability

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/gonum/stat/distuv"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	MaxQueryLimit        = 1001
	MaxSignificanceLimit = 1.0
	MinSignificanceLimit = 0.0
	DefaultSignificance  = 0.05
)

// TaskReliabilityFilter represents search and aggregation parameters when querying the test or task statistics.
type TaskReliabilityFilter struct {
	Project    string
	Requesters []string
	AfterDate  time.Time
	BeforeDate time.Time

	Tasks         []string
	BuildVariants []string
	Distros       []string

	GroupNumDays int
	GroupBy      stats.GroupBy
	StartAt      *stats.StartAt
	Limit        int
	Sort         stats.Sort
	Significance float64
}

// ValidateForTaskReliability validates that the StartAt struct is valid for use with test stats.
func (f *TaskReliabilityFilter) ValidateForTaskReliability() error {
	catcher := grip.NewBasicCatcher()
	statsFilter := stats.StatsFilter{
		Project:       f.Project,
		Requesters:    f.Requesters,
		AfterDate:     f.AfterDate,
		BeforeDate:    f.BeforeDate,
		Tasks:         f.Tasks,
		BuildVariants: f.BuildVariants,
		Distros:       f.Distros,
		GroupNumDays:  f.GroupNumDays,
		GroupBy:       f.GroupBy,
		StartAt:       f.StartAt,
		Limit:         f.Limit,
		Sort:          f.Sort,
	}

	catcher.Add(statsFilter.ValidateCommon())

	if f.Limit > MaxQueryLimit || f.Limit <= 0 {
		catcher.Add(errors.New("Invalid Limit value"))
	}

	if f.Significance > MaxSignificanceLimit || f.Significance < MinSignificanceLimit {
		catcher.Add(errors.New("Invalid Significance value"))
	}

	if len(f.Tasks) == 0 {
		catcher.Add(errors.New("Missing Task values"))
	}
	return catcher.Resolve()
}

//////////////////////////////
// Task Reliability Querying //
//////////////////////////////

// TaskReliability represents task execution statistics.
type TaskReliability struct {
	TaskName     string    `bson:"task_name"`
	BuildVariant string    `bson:"variant"`
	Distro       string    `bson:"distro"`
	Date         time.Time `bson:"date"`

	NumTotal           int     `bson:"num_total"`
	NumSuccess         int     `bson:"num_success"`
	NumFailed          int     `bson:"num_failed"`
	NumTimeout         int     `bson:"num_timeout"`
	NumTestFailed      int     `bson:"num_test_failed"`
	NumSystemFailed    int     `bson:"num_system_failed"`
	NumSetupFailed     int     `bson:"num_setup_failed"`
	AvgDurationSuccess float64 `bson:"avg_duration_success"`
	SuccessRate        float64
	Z                  float64
	LastUpdate         time.Time `bson:"last_update"`
}

// calculateSuccessRate using
// // https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval#Wilson_score_interval
// and return the lower value (for success rates).
func (s *TaskReliability) calculateSuccessRate() {
	total := float64(s.NumTotal)
	success := float64(s.NumSuccess)
	failed := float64(s.NumFailed)
	low := 0.0
	high := 0.0
	p := 0.0

	if total != 0 {
		product := success * failed
		p = success / total

		zSquared := s.Z * s.Z
		zSquared2 := zSquared / 2.0
		zSquared4 := zSquared / 4.0

		value := (success + zSquared2) / total / (1 + zSquared/total)
		margin := (s.Z * math.Sqrt(product/total+zSquared4) / total) / (1 + zSquared/total)
		high = value + margin
		low = value - margin

	}
	grip.Debugf("SuccessRate: %.4f(p=%.4f,hi=%.4f). Success=%d, Failed=%d, Total=%d, z=%4f\n", low, p, high, int(success), int(failed), int(total), s.Z)
	s.SuccessRate = (math.Round(low*100) / 100)
}

// Create a TaskReliability struct from the task stats and calculate the success rate
// using the z score.
func newTaskReliability(taskStat stats.TaskStats, z float64) TaskReliability {
	reliability := TaskReliability{
		TaskName:           taskStat.TaskName,
		BuildVariant:       taskStat.BuildVariant,
		Distro:             taskStat.Distro,
		Date:               taskStat.Date,
		NumTotal:           taskStat.NumTotal,
		NumSuccess:         taskStat.NumSuccess,
		NumFailed:          taskStat.NumFailed,
		NumTimeout:         taskStat.NumTimeout,
		NumTestFailed:      taskStat.NumTestFailed,
		NumSystemFailed:    taskStat.NumSystemFailed,
		NumSetupFailed:     taskStat.NumSetupFailed,
		AvgDurationSuccess: taskStat.AvgDurationSuccess,
		LastUpdate:         taskStat.LastUpdate,
		Z:                  z,
	}
	reliability.calculateSuccessRate()
	return reliability
}

// Convert the significance level to a z score for a two tailed test.
// The z score is the number of standard deviations from the mean.
// https://www.investopedia.com/terms/z/zscore.asp.
// https://www.investopedia.com/terms/t/two-tailed-test.asp
func significanceToZ(significance float64) float64 {
	z := distuv.Normal{Mu: 0., Sigma: 1.}.Quantile(1. - significance/2.)
	grip.Debugf("significanceToZ: significance=%.4f, z=%.4f\n", significance, z)
	return z
}

// GetTaskReliabilityScores queries the precomputed task statistics using a filter and then calculates
// the success reliability score from the lower bound wilson confidence interval.
// https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval#Wilson_score_interval.
func GetTaskReliabilityScores(filter TaskReliabilityFilter) ([]TaskReliability, error) {
	err := filter.ValidateForTaskReliability()
	if err != nil {
		return nil, errors.Wrap(err, "The provided StatsFilter is invalid")
	}
	var taskStats []stats.TaskStats
	pipeline := filter.TaskReliabilityQueryPipeline()
	jsonString, _ := json.MarshalIndent(pipeline, "", "\t")
	err = ioutil.WriteFile("/home/jim/tmp/pipeline.json", []byte(jsonString), 0644)
	err = db.Aggregate(stats.DailyTaskStatsCollection, pipeline, &taskStats)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to aggregate task statistics")
	}

	apiReliabilityResult := make([]TaskReliability, len(taskStats))
	z := significanceToZ(filter.Significance)
	for i, taskStat := range taskStats {
		apiReliabilityResult[i] = newTaskReliability(taskStat, z)
	}
	return apiReliabilityResult, nil
}
