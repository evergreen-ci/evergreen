package reliability

import (
	"math"
	"time"

	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/stat/distuv"
)

const (
	MaxQueryLimit        = taskstats.MaxQueryLimit - 1 // 1000 // route.ReliabilityAPIMaxNumTasks
	MaxSignificanceLimit = 1.0
	MinSignificanceLimit = 0.0
	DefaultSignificance  = 0.05

	GroupByTask       = taskstats.GroupByTask
	GroupByVariant    = taskstats.GroupByVariant
	GroupByDistro     = taskstats.GroupByDistro
	SortEarliestFirst = taskstats.SortEarliestFirst
	SortLatestFirst   = taskstats.SortLatestFirst
)

// TaskReliabilityFilter represents search and aggregation parameters when querying the test or task statistics.
type TaskReliabilityFilter struct {
	taskstats.StatsFilter
	Significance float64
}

// ValidateForTaskReliability validates that the StartAt struct is valid for use with test taskstats.
func (f *TaskReliabilityFilter) ValidateForTaskReliability() error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(f.ValidateCommon())

	if !f.AfterDate.Equal(utility.GetUTCDay(f.AfterDate)) {
		catcher.New("invalid 'after' date")
	}
	if !f.BeforeDate.Equal(utility.GetUTCDay(f.BeforeDate)) {
		catcher.New("invalid 'before' date")
	}
	if f.BeforeDate.Before(f.AfterDate) {
		catcher.New("'after' date restriction must be earlier than 'before' date restriction")
	}

	if f.Limit > MaxQueryLimit || f.Limit <= 0 {
		catcher.New("invalid limit")
	}

	if f.Significance > MaxSignificanceLimit || f.Significance < MinSignificanceLimit {
		catcher.New("invalid significance")
	}

	if len(f.Tasks) == 0 {
		catcher.New("missing tasks")
	}
	return catcher.Resolve()
}

//////////////////////////////
// Task Reliability Querying //
//////////////////////////////

// TaskReliability represents task execution statistics.
type TaskReliability struct {
	TaskName           string
	BuildVariant       string
	Distro             string
	Date               time.Time
	NumTotal           int
	NumSuccess         int
	NumFailed          int
	NumTimeout         int
	NumTestFailed      int
	NumSystemFailed    int
	NumSetupFailed     int
	AvgDurationSuccess float64
	SuccessRate        float64
	Z                  float64
	LastUpdate         time.Time
}

// calculateSuccessRate using
// https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval#Wilson_score_interval
// and return the lower value (for success rates).
func (s *TaskReliability) calculateSuccessRate() {
	total := float64(s.NumTotal)
	success := float64(s.NumSuccess)
	low := 0.0
	high := 0.0
	p := 0.0

	if total != 0 {
		p = success / total

		dist := s.Z * math.Sqrt((p*(1.-p)+s.Z*s.Z/(4.*total))/total)
		denominator := 1. + s.Z*s.Z/total
		c1 := p + s.Z*s.Z/(2.*total)
		high = math.Min(1, (c1+dist)/denominator)
		low = math.Max(0, (c1-dist)/denominator)
	}
	s.SuccessRate = (math.Ceil(low*100) / 100)
	grip.Info(message.Fields{
		"message":      "calculated task success rate",
		"num_success":  s.NumSuccess,
		"num_failed":   s.NumFailed,
		"num_total":    s.NumTotal,
		"z":            s.Z,
		"success_rate": s.SuccessRate,
		"low":          low,
		"p":            p,
		"high":         high,
	})
}

// Create a TaskReliability struct from the task stats and calculate the success rate
// using the z score.
func newTaskReliability(taskStat taskstats.TaskStats, z float64) TaskReliability {
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
		return nil, errors.Wrap(err, "invalid stats filter")
	}
	taskStats, err := filter.GetTaskStats()
	if err != nil {
		return nil, errors.Wrap(err, "aggregating task statistics")
	}

	apiReliabilityResult := make([]TaskReliability, len(taskStats))
	z := significanceToZ(filter.Significance)
	for i, taskStat := range taskStats {
		apiReliabilityResult[i] = newTaskReliability(taskStat, z)
	}
	return apiReliabilityResult, nil
}
