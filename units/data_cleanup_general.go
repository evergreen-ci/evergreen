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
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
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

type dataCleanup struct {
	job.Base       `bson:"metadata" json:"metadata" yaml:"metadata"`
	CollectionName string `bson:"collection_name" json:"collection_name" yaml:"collection_name"`
	TTLDays        int    `bson:"ttl_days" json:"ttl_days" yaml:"ttl_days"`

	env evergreen.Environment
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

func NewDbCleanupJob(ts time.Time, collectionName string, ttlDays int) amboy.Job {
	j := makeDbCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", dbCleanupJobName, ts.Format(TSFormat)))
	j.UpdateTimeInfo(amboy.JobTimeInfo{MaxTime: time.Minute})
	j.CollectionName = collectionName
	j.TTLDays = ttlDays
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
	timestamp := time.Now().Add(time.Duration(-j.TTLDays*24) * time.Hour)

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
			num, err := j.deleteCollectionWithLimit(ctx, j.env, timestamp, cleanupBatchSize)
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

func (j *dataCleanup) deleteCollectionWithLimit(ctx context.Context, env evergreen.Environment, ts time.Time, limit int) (int, error) {
	if limit > 100*1000 {
		panic("cannot delete more than 100k documents in a single operation")
	}

	ops := make([]mongo.WriteModel, limit)
	for idx := 0; idx < limit; idx++ {
		var filter map[string]interface{}
		if j.CollectionName == testresult.Collection {
			filter = bson.M{"_id": bson.M{"$lt": primitive.NewObjectIDFromTimestamp(ts).Hex()}}
		} else if j.CollectionName == "event_log" {
			rTypes := []string{
				"TASK",
				"SCHEDULER",
				"PATCH",
			}
			filter = bson.M{"_id": bson.M{"$lt": primitive.NewObjectIDFromTimestamp(ts)}, "r_type": bson.M{"$in": rTypes}}

		} else if j.CollectionName == model.TestLogCollection {
			filter = bson.M{"_id": bson.M{"$lt": primitive.NewObjectIDFromTimestamp(ts).Hex()}}
		}
		ops[idx] = mongo.NewDeleteOneModel().SetFilter(filter)
	}

	res, err := env.DB().Collection(j.CollectionName).BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(res.DeletedCount), nil
}
