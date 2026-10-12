package task

import (
	"context"
	"errors"
	"slices"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/cost"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

type Tasks []*Task

const taskInsertBatchSize = 500

type taskInserter interface {
	InsertMany(context.Context, []any, ...*options.InsertManyOptions) (*mongo.InsertManyResult, error)
}

func (t Tasks) Len() int           { return len(t) }
func (t Tasks) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t Tasks) Less(i, j int) bool { return t[i].Id < t[j].Id }

func (t Tasks) getPayload() []any {
	payload := make([]any, len(t))
	for idx := range t {
		payload[idx] = any(t[idx])
	}

	return payload
}

func (t Tasks) Export() []Task {
	out := make([]Task, len(t))
	for idx := range t {
		out[idx] = *t[idx]
	}
	return out
}

func (t Tasks) Insert(ctx context.Context) error {
	return db.InsertMany(ctx, Collection, t.getPayload()...)
}

func (t Tasks) InsertUnordered(ctx context.Context) error {
	if t.Len() == 0 {
		return nil
	}
	return t.insertUnordered(ctx, evergreen.GetEnvironment().DB().Collection(Collection))
}

func (t Tasks) insertUnordered(ctx context.Context, collection taskInserter) error {
	var combinedErrors mongo.BulkWriteException
	for start := 0; start < len(t); start += taskInsertBatchSize {
		end := min(start+taskInsertBatchSize, len(t))
		// Bound BSON and tracing buffers while retaining tasks for dependency counts.
		_, err := collection.InsertMany(ctx, t[start:end].getPayload(), options.InsertMany().SetOrdered(false))
		if err == nil {
			continue
		}
		var batchErrors mongo.BulkWriteException
		if !errors.As(err, &batchErrors) {
			return err
		}
		// Unordered writes still attempt later batches after document-level errors.
		for _, writeError := range batchErrors.WriteErrors {
			writeError.Index += start
			combinedErrors.WriteErrors = append(combinedErrors.WriteErrors, writeError)
		}
		if batchErrors.WriteConcernError != nil {
			combinedErrors.WriteConcernError = batchErrors.WriteConcernError
		}
		for _, label := range batchErrors.Labels {
			if !slices.Contains(combinedErrors.Labels, label) {
				combinedErrors.Labels = append(combinedErrors.Labels, label)
			}
		}
	}
	if len(combinedErrors.WriteErrors) > 0 || combinedErrors.WriteConcernError != nil {
		return combinedErrors
	}
	return nil
}

// InsertUnorderedWithPredictions inserts tasks with predicted costs applied without modifying the input tasks.
func (t Tasks) InsertUnorderedWithPredictions(ctx context.Context, predictions map[string]cost.Cost) error {
	if t.Len() == 0 {
		return nil
	}

	ctx, span := tracer.Start(ctx, "insert-tasks-with-predictions")
	defer span.End()
	span.SetAttributes(attribute.Int(evergreen.PatchNumTasksOtelAttribute, len(t)))

	// Create payload with predictions applied to copies
	_, payloadSpan := tracer.Start(ctx, "build-task-payload")
	payload := make([]any, len(t))
	for idx := range t {
		taskCopy := *t[idx] // Make a copy to avoid modifying the original
		if predictedCost, ok := predictions[taskCopy.Id]; ok && !predictedCost.IsZero() {
			taskCopy.PredictedTaskCost = predictedCost
		}
		payload[idx] = any(&taskCopy)
	}
	payloadSpan.End()

	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertMany(ctx, payload, options.InsertMany().SetOrdered(false))
	return err
}

// ByPriority sorts execution tasks within a parent display task according to
// their display statuses (and has nothing to do with its scheduling priority).
type ByPriority []Task

func (p ByPriority) Len() int      { return len(p) }
func (p ByPriority) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p ByPriority) Less(i, j int) bool {
	return p[i].displayTaskPriority() < p[j].displayTaskPriority()
}
