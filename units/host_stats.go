package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	longRunningHostThreshold = 24 * time.Hour
	hostStatsName            = "host-status-alerting"
)

func init() {
	registry.AddJobType(hostStatsName, func() amboy.Job {
		return makeHostStats()
	})
}

type hostStatsJob struct {
	job.Base `bson:"base" json:"base" yaml:"base"`
	logger   grip.Journaler
}

type taskSpawnedHost struct {
	ID        string `json:"id"`
	SpawnedBy string `json:"spawned_by"`
	Task      string `json:"task_scope"`
	Build     string `json:"build_scope"`
}

func makeHostStats() *hostStatsJob {
	j := &hostStatsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostStatsName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostStatsJob(ts string) amboy.Job {
	job := makeHostStats()
	job.SetID(fmt.Sprintf("%s.%s", hostAlertingName, ts))
	return job
}

func (j *hostStatsJob) Run(_ context.Context) {
	defer j.MarkComplete()

	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	inactiveHosts, err := host.CountInactiveHostsByProvider()
	if err != nil {
		j.AddError(errors.Wrap(err, "error aggregating inactive hosts"))
		return
	}
	j.logger.Info(message.Fields{
		"message": "count of decommissioned/quarantined hosts",
		"counts":  inactiveHosts,
	})

	taskSpawned, err := host.FindAllHostsSpawnedByTasks()
	if err != nil {
		j.AddError(errors.Wrap(err, "error finding hosts spawned by tasks"))
		return
	}
	hosts := []taskSpawnedHost{}
	for _, h := range taskSpawned {
		hosts = append(hosts, taskSpawnedHost{
			ID:        h.Id,
			SpawnedBy: h.User,
			Task:      h.SpawnOptions.TaskID,
			Build:     h.SpawnOptions.BuildID,
		})
	}
	j.logger.Info(message.Fields{
		"message": "hosts spawned by tasks",
		"hosts":   hosts,
	})

	for _, h := range taskSpawned {
		if !util.IsZeroTime(h.StartTime) && h.StartTime.Add(longRunningHostThreshold).Before(time.Now()) {
			j.logger.Warning(message.Fields{
				"message":         "long running host spawned by task",
				"id":              h.Id,
				"duration":        time.Now().Sub(h.StartTime).Seconds(),
				"duration_string": time.Now().Sub(h.StartTime).String(),
				"spawned_by":      h.User,
				"task_scope":      h.SpawnOptions.TaskID,
				"build_scope":     h.SpawnOptions.BuildID,
			})
		}
	}
}
