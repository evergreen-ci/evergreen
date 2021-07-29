package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	dbCleanupJobName = "db-cleanup"
	cleanupBatchSize = 100 * 1000
)

func init() {
	registry.AddJobType(dbCleanupJobName, func() amboy.Job {
		return makeDbCleanupJob()
	})
}

type cleanupJob func(context.Context, evergreen.Environment, time.Time, int) (int, error)

type dataCleanup struct {
	job.Base       `bson:"metadata" json:"metadata" yaml:"metadata"`
	collectionName string `bson:"collection_name" json:"collection_name" yaml:"collection_name"`

	env               evergreen.Environment
	deleteWithLimitFn cleanupJob
}

func makeDbCleanupJob() *dataCleanup {
	j := &dataCleanup{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    dbCleanupJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())

	return j
}

func NewDbCleanupJob(ts time.Time, deleteWithLimitFn cleanupJob, collectionName string) amboy.Job {
	j := makeDbCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", dbCleanupJobName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	j.deleteWithLimitFn = deleteWithLimitFn
	j.collectionName = collectionName
	return j
}

func (j *dataCleanup) Run(ctx context.Context) {
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
			"job_type": dbCleanupJobName,
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

	totalDocs, _ := j.env.DB().Collection(j.collectionName).EstimatedDocumentCount(ctx)
	timestamp := time.Now().Add(time.Duration(-365*24) * time.Hour)

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
			num, err := j.deleteWithLimitFn(ctx, j.env, timestamp, cleanupBatchSize)
			j.AddError(err)

			batches++
			numDocs += num
			timeSpent += time.Since(opStart)
			if num < cleanupBatchSize {
				break
			}
		}
	}

	grip.Info(message.Fields{
		"job_id":             j.ID(),
		"job_type":           j.Type().Name,
		"batch_size":         cleanupBatchSize,
		"total_docs":         totalDocs,
		"collection":         j.collectionName,
		"message":            "timing-info",
		"run_start_at":       startAt,
		"oid":                primitive.NewObjectIDFromTimestamp(timestamp).Hex(),
		"oid_ts":             timestamp.Format(TSFormat),
		"has_errors":         j.HasErrors(),
		"aborted":            ctx.Err() != nil,
		"total":              time.Since(startAt).Seconds(),
		"run_end_at":         time.Now(),
		"num_batches":        batches,
		"num_docs":           numDocs,
		"time_spent_seconds": timeSpent.Seconds(),
	})
}
