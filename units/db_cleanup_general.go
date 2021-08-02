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
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	dbCleanupJobName = "db-cleanup"
	cleanupBatchSize = 100 * 1000
)

func init() {
	registry.AddJobType(dbCleanupJobName, func() amboy.Job {
		return makeDBCleanupJob()
	})
}

type FilterFunc func(time.Time) map[string]interface{}

type dataCleanup struct {
	job.Base       `bson:"metadata" json:"metadata" yaml:"metadata"`
	CollectionName string        `bson:"collection_name" json:"collection_name" yaml:"collection_name"`
	TTL            time.Duration `bson:"ttl" json:"ttl" yaml:"ttl"`
	FilterFunc     FilterFunc    `bson:"filter_func" json:"filter_func" yaml:"filter_func"`

	env evergreen.Environment
}

func makeDBCleanupJob() *dataCleanup {
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

// NewDBCleanupJob batch deletes documents in the given collection older than the TTL.
func NewDBCleanupJob(ts time.Time, collectionName string, filter FilterFunc, ttl time.Duration) amboy.Job {
	j := makeDBCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", dbCleanupJobName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	j.CollectionName = collectionName
	j.TTL = ttl
	j.FilterFunc = filter
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

	totalDocs, _ := j.env.DB().Collection(j.CollectionName).EstimatedDocumentCount(ctx)
	timestamp := time.Now().Add(j.TTL)

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
			num, err := j.deleteCollectionWithLimit(ctx, timestamp, cleanupBatchSize)
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
		"collection":         j.CollectionName,
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

func (j *dataCleanup) deleteCollectionWithLimit(ctx context.Context, ts time.Time, limit int) (int, error) {
	if limit > 100*1000 {
		panic("cannot delete more than 100k documents in a single operation")
	}

	ops := make([]mongo.WriteModel, limit)
	for idx := 0; idx < limit; idx++ {
		ops[idx] = mongo.NewDeleteOneModel().SetFilter(j.FilterFunc(ts))
	}

	res, err := j.env.DB().Collection(j.CollectionName).BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(res.DeletedCount), nil
}
