package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/cost"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

type Tasks []*Task

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

// insertBatchSize caps how many tasks go into a single insertMany. 128 is
// a multiple of the server's internalInsertMaxBatchSize (64), so each batch fills whole
// server-side WiredTiger transactions rather than leaving a partial one.
const insertBatchSize = 128

// insertTasksUnordered inserts the payload in batches of insertBatchSize. On
// error it returns without attempting the remaining batches, so the tasks are
// left partially inserted (as they already are for a partially-failed unordered
// insertMany).
func insertTasksUnordered(ctx context.Context, payload []any) error {
	coll := evergreen.GetEnvironment().DB().Collection(Collection)
	for start := 0; start < len(payload); start += insertBatchSize {
		batch := payload[start:min(start+insertBatchSize, len(payload))]
		if _, err := coll.InsertMany(ctx, batch, options.InsertMany().SetOrdered(false)); err != nil {
			return errors.Wrapf(err, "inserting tasks %d-%d of %d", start, start+len(batch), len(payload))
		}
	}
	return nil
}

func (t Tasks) InsertUnordered(ctx context.Context) error {
	if t.Len() == 0 {
		return nil
	}
	return insertTasksUnordered(ctx, t.getPayload())
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

	return insertTasksUnordered(ctx, payload)
}

// ByPriority sorts execution tasks within a parent display task according to
// their display statuses (and has nothing to do with its scheduling priority).
type ByPriority []Task

func (p ByPriority) Len() int      { return len(p) }
func (p ByPriority) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p ByPriority) Less(i, j int) bool {
	return p[i].displayTaskPriority() < p[j].displayTaskPriority()
}
