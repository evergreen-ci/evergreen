package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
)

const testLogsCleanupJobName = "data-cleanup-testlogs"

func init() {
	registry.AddJobType(testLogsCleanupJobName, func() amboy.Job {
		return makeTestLogsCleanupJob()
	})
}

type dataCleanupTestLogs struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env evergreen.Environment
}

func makeTestLogsCleanupJob() *dataCleanupTestLogs {
	j := &dataCleanupTestLogs{
		Base: job.Base{

			JobType: amboy.JobType{
				Name:    testLogsCleanupJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

func NewTestLogsCleanupJob(ts time.Time) amboy.Job {
	j := makeTestLogsCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", testLogsCleanupJobName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	return j
}

func (j *dataCleanupTestLogs) Run(ctx context.Context) {
	defer j.MarkComplete()
	startAt := time.Now()

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
			"job_type": testLogsCleanupJobName,
			"job_id":   j.ID(),
			"message":  "disaster recovery backups disabled, also disabling cleanup",
		})
		return
	}

	var (
		batches   int
		numDocs   int
		timeSpent time.Duration
	)

	totalDocs, _ := j.env.DB().Collection(model.TestLogCollection).EstimatedDocumentCount(ctx)

LOOP:
	for {
		select {
		case <-ctx.Done():
			break LOOP
		default:
			if time.Since(startAt) >= 50*time.Second {
				break LOOP
			}
			opStart := time.Now()
			num, err := model.DeleteTestLogsWithLimit(ctx, j.env, time.Now().Add(time.Duration(-365*24)*time.Hour), 100*1000)
			j.AddError(err)

			batches++
			numDocs += num
			timeSpent += time.Since(opStart)
		}
	}

	grip.Info(message.Fields{
		"job_id":             j.ID(),
		"collection":         model.TestLogCollection,
		"job_type":           j.Type().Name,
		"total_docs":         totalDocs,
		"message":            "timing-info",
		"run_start_at":       startAt,
		"has_errors":         j.HasErrors(),
		"aborted":            ctx.Err() != nil,
		"total":              time.Since(startAt).Seconds(),
		"run_end_at":         time.Now(),
		"num_batches":        batches,
		"num_docs":           numDocs,
		"time_spent_seconds": timeSpent.Seconds(),
	})

}
