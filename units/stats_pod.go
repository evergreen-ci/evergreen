package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const podStatsCollectorJobName = "pod-stats-collector"

func init() {
	registry.AddJobType(podStatsCollectorJobName, func() amboy.Job {
		return makePodStatsCollector()
	})
}

type podStatsCollector struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
}

// NewPodStatsCollector logs statistics about current pod usage.
func NewPodStatsCollector(id string) amboy.Job {
	j := makePodStatsCollector()
	j.SetID(fmt.Sprintf("%s.%s", podStatsCollectorJobName, id))
	j.SetEnqueueScopes(podStatsCollectorJobName)

	return j
}

func makePodStatsCollector() *podStatsCollector {
	j := &podStatsCollector{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podStatsCollectorJobName,
				Version: 0,
			},
		},
	}
	return j
}

func (j *podStatsCollector) Run(ctx context.Context) {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
		return
	}

	if flags.BackgroundStatsDisabled {
		grip.Debug(message.Fields{
			"mode":     "degraded",
			"impact":   "pod stats collection disabled",
			"job":      j.ID(),
			"job_type": j.Type().Name,
		})
		return
	}

	j.AddError(j.logStats())
}

func (j *podStatsCollector) logStats() error {
	statuses := []pod.Status{
		pod.StatusInitializing,
		pod.StatusStarting,
		pod.StatusRunning,
		pod.StatusDecommissioned,
	}
	stats, err := pod.GetStatsByStatus(statuses...)
	if err != nil {
		return errors.Wrap(err, "getting statistics by pod status")
	}

	msg := message.Fields{
		"message":     "pod stats",
		"included_on": evergreen.ContainerHealthDashboard,
	}
	// Ensure that the statuses of interest are always set, even if the query
	// returns no statistics for that status.
	for _, s := range statuses {
		msg[fmt.Sprintf("num_%s", s)] = 0
	}

	var totalRunningTasks int
	for _, s := range stats {
		msg[fmt.Sprintf("num_%s", s.Status)] = s.Count
		totalRunningTasks += s.NumRunningTasks
	}
	msg["num_running_tasks"] = totalRunningTasks

	grip.Info(msg)

	return nil
}
