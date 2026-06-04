package units

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
)

const (
	taskStatsCollectorJobName  = "task-stats-collector"
	taskStatsCollectorInterval = time.Minute
)

func init() {
	registry.AddJobType(taskStatsCollectorJobName,
		func() amboy.Job { return makeTaskStatsCollector() })
}

type taskStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	logger   grip.Journaler
}

// NewTaskStatsCollector captures a single report of the status of
// tasks that have completed in the last minute.
func NewTaskStatsCollector(id string) amboy.Job {
	t := makeTaskStatsCollector()
	t.SetID(fmt.Sprintf("%s-%s", taskStatsCollectorJobName, id))
	return t
}

func makeTaskStatsCollector() *taskStatsCollector {
	j := &taskStatsCollector{
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    taskStatsCollectorJobName,
				Version: 0,
			},
		},
	}
	return j
}

func (j *taskStatsCollector) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.BackgroundStatsDisabled {
		grip.Debug(ctx, message.Fields{
			"mode":     "degraded",
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	tasks, err := task.GetRecentTasks(ctx, taskStatsCollectorInterval)
	if err != nil {
		j.AddError(err)
		return
	}

	j.logger.Info(ctx, task.GetResultCounts(tasks))

	if len(tasks) == 0 {
		return
	}
	j.logStats(ctx, tasks)
}

func (j *taskStatsCollector) logStats(ctx context.Context, tasks []task.Task) {
	for _, msg := range getWaitTimeMessages(tasks) {
		j.logger.Info(ctx, msg)
	}

	requesterCounts := make(map[string]int, len(evergreen.AllRequesterTypes))
	projectCounts := make(map[string]int)
	for _, t := range tasks {
		requesterCounts[t.Requester]++
		projectCounts[t.Project]++
	}

	j.logger.Info(ctx, message.Fields{
		"message":              "task stats by requester",
		"stats":                "tasks-by-requester",
		"gitter_request":       requesterCounts[evergreen.RepotrackerVersionRequester],
		"patch_request":        requesterCounts[evergreen.PatchVersionRequester],
		"github_pull_request":  requesterCounts[evergreen.GithubPRRequester],
		"github_merge_request": requesterCounts[evergreen.GithubMergeRequester],
		"trigger_request":      requesterCounts[evergreen.TriggerRequester],
		"ad_hoc":               requesterCounts[evergreen.AdHocRequester],
		"git_tag_request":      requesterCounts[evergreen.GitTagRequester],
		"total":                len(tasks),
	})

	j.logger.Info(ctx, message.Fields{
		"message":  "task stats by project",
		"stats":    "tasks-by-project",
		"projects": topNProjects(projectCounts, 10),
		"total":    len(tasks),
	})
}

type taskTimings struct {
	waitSecs      float64
	overheadRatio float64
}

const minDistroTasksForStats = 5

// getWaitTimeMessages computes wait time percentiles and scheduler overhead
// ratios for the given tasks, broken down globally and per-distro/per-requester.
func getWaitTimeMessages(tasks []task.Task) []message.Fields {
	var globalTimings []taskTimings
	byDistro := make(map[string][]taskTimings)
	byRequester := make(map[string][]taskTimings)

	for _, t := range tasks {
		if utility.IsZeroTime(t.StartTime) || utility.IsZeroTime(t.FinishTime) {
			continue
		}

		submitTime := t.DependenciesMetTime
		if utility.IsZeroTime(submitTime) {
			submitTime = t.ScheduledTime
		}
		if utility.IsZeroTime(submitTime) {
			continue
		}

		turnaround := t.FinishTime.Sub(submitTime)
		waitTime := t.StartTime.Sub(submitTime)
		if waitTime < 0 || turnaround <= 0 {
			continue
		}

		timing := taskTimings{
			waitSecs: waitTime.Seconds(),
		}
		// Overhead ratio is the fraction of total turnaround spent waiting rather than executing.
		timing.overheadRatio = waitTime.Seconds() / turnaround.Seconds()

		globalTimings = append(globalTimings, timing)
		if t.DistroId != "" {
			byDistro[t.DistroId] = append(byDistro[t.DistroId], timing)
		}
		if t.Requester != "" {
			byRequester[t.Requester] = append(byRequester[t.Requester], timing)
		}
	}

	var msgs []message.Fields

	if len(globalTimings) > 0 {
		msgs = append(msgs, makeWaitStatsMessage("", "", globalTimings))
	}

	distroStats := make(map[string]message.Fields)
	for distro, timings := range byDistro {
		if len(timings) < minDistroTasksForStats {
			continue
		}
		distroStats[distro] = makeWaitStatsFields(timings)
	}
	if len(distroStats) > 0 {
		msgs = append(msgs, message.Fields{
			"message": "task queue wait stats by distro",
			"stats":   "queue-wait-by-distro",
			"distros": distroStats,
		})
	}

	for requester, timings := range byRequester {
		msgs = append(msgs, makeWaitStatsMessage("requester", requester, timings))
	}

	return msgs
}

func makeWaitStatsFields(timings []taskTimings) message.Fields {
	waits := make([]float64, len(timings))
	ratios := make([]float64, len(timings))
	for i, t := range timings {
		waits[i] = t.waitSecs
		ratios[i] = t.overheadRatio
	}
	slices.Sort(waits)
	slices.Sort(ratios)

	return message.Fields{
		"sample_size":        len(timings),
		"p50_wait_seconds":   percentile(waits, 50),
		"p90_wait_seconds":   percentile(waits, 90),
		"p50_overhead_ratio": percentile(ratios, 50),
		"p90_overhead_ratio": percentile(ratios, 90),
	}
}

func makeWaitStatsMessage(groupKey, groupValue string, timings []taskTimings) message.Fields {
	fields := makeWaitStatsFields(timings)

	msg := "task queue wait stats"
	if groupKey != "" {
		msg = fmt.Sprintf("task queue wait stats by %s", groupKey)
	}
	fields["message"] = msg
	fields["stats"] = "queue-wait"
	if groupKey != "" {
		fields[groupKey] = groupValue
	}
	return fields
}

func percentile(sorted []float64, pct float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(pct / 100 * float64(len(sorted)-1))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func topNProjects(counts map[string]int, n int) map[string]int {
	type projectCount struct {
		project string
		count   int
	}

	sorted := make([]projectCount, 0, len(counts))
	for p, c := range counts {
		sorted = append(sorted, projectCount{project: p, count: c})
	}
	slices.SortFunc(sorted, func(a, b projectCount) int {
		return b.count - a.count
	})

	result := make(map[string]int, min(n, len(sorted)))
	for i := range min(n, len(sorted)) {
		result[sorted[i].project] = sorted[i].count
	}
	return result
}
