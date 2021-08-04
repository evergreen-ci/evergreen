package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
)

const (
	dbCleanupTestLogName = "db-cleanup-test-log"
)

func init() {
	registry.AddJobType(dbCleanupTestLogName, func() amboy.Job {
		return makeDBCleanupTestLogJob()
	})
}

type dbCleanupTestLogJob struct {
	job.Base    `bson:"metadata" json:"metadata" yaml:"metadata"`
	DataCleanup DataCleanupJobBase `bson:"data_cleanup" json:"data_cleanup" yaml:"data_cleanup"`

	env evergreen.Environment
}

func makeDBCleanupTestLogJob() *dbCleanupTestLogJob {
	j := &dbCleanupTestLogJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    dbCleanupTestLogName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

// NewDBCleanupJob batch deletes documents in the given collection older than the TTL.
func NewDBCleanupTestLogJob(ts time.Time, ttl time.Duration) amboy.Job {
	j := makeDBCleanupTestLogJob()
	j.SetID(fmt.Sprintf("%s.%s", dbCleanupTestLogName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	j.DataCleanup.CollectionName = model.TestLogCollection
	j.DataCleanup.TTL = ttl
	return j
}

func (j *dbCleanupTestLogJob) Run(ctx context.Context) {
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
			"job_type": dbCleanupTestLogName,
			"job_id":   j.ID(),
			"message":  "disaster recovery backups disabled, also disabling cleanup",
		})
		return
	}

	message, errors := j.DataCleanup.runWithDeleteFn(ctx, testresult.TestResultFilter)
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
