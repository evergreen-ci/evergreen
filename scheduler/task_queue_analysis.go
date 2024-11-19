package scheduler

import (
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type FactorStats struct {
	Mean              float64
	Max               float64
	MinNonZero        float64
	PercentageNonZero float64
}

type FactorSummary struct {
	Name           string
	Stats          FactorStats
	RelativeImpact float64
}

func generateFactorBreakdownReport(distro string, tasks []task.Task) {
	if len(tasks) == 0 {
		return
	}
	analysis := analyzeFactors(tasks)
	grip.Info(message.Fields{
		"message":      "queue ranking analysis report",
		"distro":       distro,
		"queue_length": len(tasks),
		"analysis":     analysis,
	})
}

func analyzeFactors(tasks []task.Task) []FactorSummary {
	factorNames := []string{
		evergreen.BaseImpact,
		evergreen.InitialPriorityImpact,
		evergreen.LengthImpact,
		evergreen.TaskGroupImpact,
		evergreen.GenerateTaskImpact,
		evergreen.BasePatchImpact,
		evergreen.WaitTimePatchImpact,
		evergreen.CommitQueueImpact,
		evergreen.WaitTimeMainlineTaskImpact,
		evergreen.StepbackImpact,
		evergreen.NumDependentsImpact,
		evergreen.EstimatedRuntimeImpactImpact,
	}
	summaries := make([]FactorSummary, 0, len(factorNames))
	for _, factorName := range factorNames {
		values := getFactorValues(factorName, tasks)
		stats := getStats(values)
		impact := getAvgPercentageImpact(factorName, tasks)
		summaries = append(summaries, FactorSummary{
			Name:           factorName,
			Stats:          stats,
			RelativeImpact: impact,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].RelativeImpact > summaries[j].RelativeImpact
	})
	return summaries
}

// getFactorValues takes a lists of tasks from a task queue and computes a new list of the
// queue rank value breakdown values of those tasks for a specific factor.
func getFactorValues(factorName string, tasks []task.Task) []float64 {
	values := []float64{}
	for _, t := range tasks {
		var value float64
		switch factorName {
		case evergreen.TaskGroupImpact:
			value = float64(t.RankValueBreakdown.TaskGroupImpact)
		case evergreen.GenerateTaskImpact:
			value = float64(t.RankValueBreakdown.GenerateTaskImpact)
		case evergreen.CommitQueueImpact:
			value = float64(t.RankValueBreakdown.CommitQueueImpact)
		case evergreen.BasePatchImpact:
			value = float64(t.RankValueBreakdown.PatchImpact.BaseImpact)
		case evergreen.WaitTimePatchImpact:
			value = float64(t.RankValueBreakdown.PatchImpact.WaitTimeImpact)
		case evergreen.WaitTimeMainlineTaskImpact:
			value = float64(t.RankValueBreakdown.MainlineTaskImpact.WaitTimeImpact)
		case evergreen.StepbackImpact:
			value = float64(t.RankValueBreakdown.MainlineTaskImpact.StepbackTaskImpact)
		case evergreen.NumDependentsImpact:
			value = float64(t.RankValueBreakdown.NumDependentsImpact)
		case evergreen.EstimatedRuntimeImpactImpact:
			value = float64(t.RankValueBreakdown.EstimatedRuntimeImpact)
		}
		values = append(values, value)
	}
	return values
}

// getAvgPercentageImpact computes the average percentage impact that a ranking
// factor had on all tasks in a queue.
func getAvgPercentageImpact(factorName string, tasks []task.Task) float64 {
	var totalImpact float64
	var count int
	for _, t := range tasks {
		impacts := t.RankValueBreakdown.ImpactAnalysis()
		if impact, exists := impacts[factorName]; exists {
			totalImpact += impact
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return totalImpact / float64(count)
}

// getStats takes in a list of values for a specific factor in a task queue
// and computes the max, min, and mean values for that factor on tasks
// in that queue, as well as the percentage of tasks in the queue that were influenced
// by that factor.
func getStats(values []float64) FactorStats {
	if len(values) == 0 {
		return FactorStats{}
	}
	stats := FactorStats{}

	var sum float64
	for _, v := range values {
		sum += v
	}
	stats.Mean = sum / float64(len(values))

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	stats.Max = sorted[len(sorted)-1]
	stats.MinNonZero = stats.Max
	nonZeroCount := 0
	for _, value := range sorted {
		if value > 0 {
			if value < stats.MinNonZero {
				stats.MinNonZero = value
			}
			nonZeroCount++
		}
	}

	stats.PercentageNonZero = float64(nonZeroCount) / float64(len(values)) * 100
	return stats
}
