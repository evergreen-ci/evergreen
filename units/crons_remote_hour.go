package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const cronsRemoteHourJobName = "crons-remote-hour"

func init() {
	registry.AddJobType(cronsRemoteHourJobName, NewCronRemoteHourJob)
}

type cronsRemoteHourJob struct {
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
	ErrorCount int `bson:"error_count" json:"error_count" yaml:"error_count"`
	env        evergreen.Environment
}

func NewCronRemoteHourJob() amboy.Job {
	j := &cronsRemoteHourJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cronsRemoteHourJobName,
				Version: 0,
			},
		},
	}
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteHourJobName, utility.RoundPartOfHour(0).Format(TSFormat)))
	return j
}

func (j *cronsRemoteHourJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := []amboy.QueueOperation{
		PopulateCacheHistoricalTaskDataJob(2),
		PopulateSpawnhostExpirationCheckJob(),
		PopulateCloudCleanupJob(j.env),
		PopulateVolumeExpirationCheckJob(),
		PopulateVolumeExpirationJob(),
		PopulateUnstickVolumesJob(),
		PopulateDuplicateTaskCheckJobs(),
		PopulateUnexpirableSpawnHostStatsJob(),
		PopulateDistroAutoTuneJobs(),
	}

	queue := j.env.RemoteQueue()
	catcher := grip.NewBasicCatcher()
	for _, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}

		catcher.Add(op(ctx, queue))
	}

	j.ErrorCount = catcher.Len()

	grip.Debug(message.Fields{
		"queue": "service",
		"id":    j.ID(),
		"type":  j.Type().Name,
		"num":   len(ops),
		"errs":  j.ErrorCount,
	})
}
