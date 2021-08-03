package units

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	cleanupBatchSize = 100 * 1000
)

type FilterFunc func(time.Time) map[string]interface{}

type DataCleanupJobBase struct {
	CollectionName string        `bson:"collection_name" json:"collection_name" yaml:"collection_name"`
	TTL            time.Duration `bson:"ttl" json:"ttl" yaml:"ttl"`

	errors []error
	env    evergreen.Environment
}

func (b *DataCleanupJobBase) runWithDeleteFn(ctx context.Context, filterFunc FilterFunc) (message.Fields, []error) {
	startAt := time.Now()

	if b.env == nil {
		b.env = evergreen.GetEnvironment()
	}

	var (
		batches   int
		numDocs   int
		timeSpent time.Duration
	)

	totalDocs, _ := b.env.DB().Collection(b.CollectionName).EstimatedDocumentCount(ctx)
	timestamp := time.Now().Add(b.TTL)

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
			num, err := b.deleteCollectionWithLimit(ctx, timestamp, cleanupBatchSize, filterFunc)
			b.errors = append(b.errors, err)

			batches++
			numDocs += num
			timeSpent += time.Since(opStart)
			if num < cleanupBatchSize {
				break LOOP
			}
		}
	}

	return message.Fields{
		"batch_size":         cleanupBatchSize,
		"total_docs":         totalDocs,
		"collection":         b.CollectionName,
		"message":            "timing-info",
		"run_start_at":       startAt,
		"oid":                primitive.NewObjectIDFromTimestamp(timestamp).Hex(),
		"oid_ts":             timestamp.Format(TSFormat),
		"aborted":            ctx.Err() != nil,
		"total":              time.Since(startAt).Seconds(),
		"run_end_at":         time.Now(),
		"num_batches":        batches,
		"num_docs":           numDocs,
		"time_spent_seconds": timeSpent.Seconds(),
	}, b.errors
}

func (b *DataCleanupJobBase) deleteCollectionWithLimit(ctx context.Context, ts time.Time, limit int, filterFunc FilterFunc) (int, error) {
	if limit > 100*1000 {
		return 0, errors.New("cannot delete more than 100k documents in a single operation")
	}

	ops := make([]mongo.WriteModel, limit)
	for idx := 0; idx < limit; idx++ {
		ops[idx] = mongo.NewDeleteOneModel().SetFilter(filterFunc(ts))
	}

	res, err := b.env.DB().Collection(b.CollectionName).BulkWrite(ctx, ops, options.BulkWrite().SetOrdered(false))
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(res.DeletedCount), nil
}
