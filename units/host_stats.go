package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
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
	ID                  string `json:"id"`
	SpawnedBy           string `json:"spawned_by"`
	Task                string `json:"task_scope"`
	TaskExecutionNumber int    `json:"task_execution_number"`
	Build               string `json:"build_scope"`
	Project             string `json:"project"`
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
	return j
}

func NewHostStatsJob(ts string) amboy.Job {
	job := makeHostStats()
	job.SetID(fmt.Sprintf("%s.%s", hostStatsName, ts))
	return job
}

func (j *hostStatsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	inactiveHosts, err := host.CountInactiveHostsByProvider(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "counting inactive hosts by cloud provider"))
		return
	}
	j.logger.Info(ctx, message.Fields{
		"message": "count of decommissioned/quarantined hosts",
		"counts":  inactiveHosts,
	})

	taskSpawned, err := host.FindAllHostsSpawnedByTasks(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding hosts spawned by tasks"))
		return
	}
	hosts := []taskSpawnedHost{}
	for _, h := range taskSpawned {
		hosts = append(hosts, taskSpawnedHost{
			ID:                  h.Id,
			SpawnedBy:           h.User,
			Task:                h.SpawnOptions.TaskID,
			TaskExecutionNumber: h.SpawnOptions.TaskExecutionNumber,
			Build:               h.SpawnOptions.BuildID,
			Project:             h.SpawnOptions.ProjectID,
		})
	}
	j.logger.Info(ctx, message.Fields{
		"message": "hosts spawned by tasks",
		"hosts":   hosts,
	})

	for _, h := range taskSpawned {
		if !utility.IsZeroTime(h.StartTime) && h.StartTime.Add(longRunningHostThreshold).Before(time.Now()) {
			j.logger.Warning(ctx, message.Fields{
				"message":               "long running host spawned by task",
				"id":                    h.Id,
				"duration":              time.Since(h.StartTime).Seconds(),
				"duration_string":       time.Since(h.StartTime).String(),
				"spawned_by":            h.User,
				"task_scope":            h.SpawnOptions.TaskID,
				"task_execution_number": h.SpawnOptions.TaskExecutionNumber,
				"build_scope":           h.SpawnOptions.BuildID,
				"project":               h.SpawnOptions.ProjectID,
			})
		}
	}

	count, err := host.CountVirtualWorkstationsByInstanceType(ctx)
	j.AddError(err)
	grip.Info(ctx, message.Fields{
		"message": "virtual workstations",
		"stats":   count,
	})

	spawnStats, err := host.AggregateSpawnhostData(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "aggregating spawn host data"))
	}
	debugHostCount, countErr := host.CountDebugSpawnhosts(ctx)
	if countErr != nil {
		j.AddError(errors.Wrap(countErr, "counting debug spawn hosts"))
		return
	}
	j.logger.Info(ctx, message.Fields{
		"message":                 "spawn host usage stats",
		"stats":                   "spawn-hosts",
		"total_hosts":             spawnStats.TotalHosts,
		"total_stopped_hosts":     spawnStats.TotalStoppedHosts,
		"total_unexpirable_hosts": spawnStats.TotalUnexpirableHosts,
		"num_users_with_hosts":    spawnStats.NumUsersWithHosts,
		"total_volumes":           spawnStats.TotalVolumes,
		"total_volume_size":       spawnStats.TotalVolumeSize,
		"num_users_with_volumes":  spawnStats.NumUsersWithVolumes,
		"instance_types":          spawnStats.InstanceTypes,
		"debug_hosts":             debugHostCount,
		"non_debug_hosts":         spawnStats.TotalHosts - debugHostCount,
	})

}
