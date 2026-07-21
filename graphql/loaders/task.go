package loaders

import (
	"context"
	"errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vikstrous/dataloadgen"
	"go.mongodb.org/mongo-driver/bson"
)

type taskReader struct{}

func (r *taskReader) getTasks(ctx context.Context, taskIDs []string) (map[string]*task.Task, error) {
	// Mirror task.FindOneId, which projects out the generated JSON to avoid
	// loading a potentially large field that most callers don't need.
	query := db.Query(task.ByIds(taskIDs)).Project(bson.M{task.GeneratedJSONAsStringKey: 0})
	tasks, err := task.FindAll(ctx, query)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "error fetching tasks in dataloader",
		}))
		return nil, &batchError{err: err}
	}

	taskMap := make(map[string]*task.Task, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].Id] = &tasks[i]
	}

	return taskMap, nil
}

// GetTask returns a single task by ID efficiently using the dataloader.
// Returns nil if the task is not found.
func GetTask(ctx context.Context, taskID string) (*task.Task, error) {
	l := For(ctx)
	result, err := l.TaskLoader.Load(ctx, taskID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return result, err
}

// PreloadTasks enqueues every task ID into the dataloader's current batch in a
// single synchronous loop, guaranteeing that subsequent GetTask calls for these
// IDs are served from the loader's thunk cache without any additional MongoDB
// queries. Use this when a resolver knows up front that it will need many tasks
// whose loads would otherwise be split across multiple batches due to the
// wait-time window.
//
// Errors are intentionally discarded here; per-key errors are still surfaced to
// the individual GetTask callers via the cached thunks.
func PreloadTasks(ctx context.Context, taskIDs []string) {
	if len(taskIDs) == 0 {
		return
	}
	l := For(ctx)
	_, _ = l.TaskLoader.LoadAll(ctx, taskIDs)
}
