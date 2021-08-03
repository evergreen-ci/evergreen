package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
)

const (
	dbCleanupEventLogName = "db-cleanup-event-log"
)

func init() {
	registry.AddJobType(dbCleanupEventLogName, func() amboy.Job {
		return makeDBCleanupEventLogJob()
	})
}

type dbCleanupEventLogJob struct {
	job.Base    `bson:"metadata" json:"metadata" yaml:"metadata"`
	DataCleanup DataCleanupJobBase `bson:"data_cleanup" json:"data_cleanup" yaml:"data_cleanup"`

	env evergreen.Environment
}

func makeDBCleanupEventLogJob() *dbCleanupEventLogJob {
	j := &dbCleanupEventLogJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    dbCleanupEventLogName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

// NewDBCleanupJob batch deletes documents in the given collection older than the TTL.
func NewDBCleanupEventLogJob(ts time.Time, ttl time.Duration) amboy.Job {
	j := makeDBCleanupEventLogJob()
	j.SetID(fmt.Sprintf("%s.%s", dbCleanupEventLogName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	j.DataCleanup.CollectionName = event.AllLogCollection
	j.DataCleanup.TTL = ttl
	return j
}

func (j *dbCleanupEventLogJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}

	if flags.BackgroundCleanupDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job_type": dbCleanupEventLogName,
			"job_id":   j.ID(),
			"message":  "disaster recovery backups disabled, also disabling cleanup",
		})
		return
	}

	message, errors := j.DataCleanup.RunWithDeleteFn(ctx, model.TestLogFilter)
	if len(errors) != 0 {
		//add errors
		for _, err := range errors {
			j.AddError(err)
		}
		return
	}
	message["job_id"] = j.ID()
	message["job_type"] = j.Type().Name
	message["has_errors"] = j.HasErrors()

	grip.Info(message)
}
