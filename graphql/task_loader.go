package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"go.mongodb.org/mongo-driver/bson"
)

type taskReader struct{}

func (t *taskReader) getTasks(ctx context.Context, taskIDs []string) ([]*restModel.APITask, []error) {
	query := db.Query(bson.M{task.IdKey: bson.M{"$in": taskIDs}}).Project(bson.M{task.GeneratedJSONAsStringKey: 0})
	tasks, err := task.FindAll(ctx, query)
	if err != nil {
		// Return the same error for all requested IDs
		errs := make([]error, len(taskIDs))
		for i := range errs {
			errs[i] = err
		}
		return nil, errs
	}

	taskMap := make(map[string]*task.Task, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].Id] = &tasks[i]
	}

	// Build results in the same order as input IDs
	// Return nil for tasks not found
	results := make([]*restModel.APITask, len(taskIDs))
	errs := make([]error, len(taskIDs))
	for i, id := range taskIDs {
		if dbTask, ok := taskMap[id]; ok {
			apiTask := &restModel.APITask{}
			if err := apiTask.BuildFromService(ctx, dbTask, nil); err != nil {
				errs[i] = err
			} else {
				results[i] = apiTask
			}
		}
		// results[i] remains nil if task not found, errs[i] remains nil
	}

	return results, errs
}

// GetTask returns a single task by ID efficiently using the dataloader.
// Returns nil if the task is not found.
func GetTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	loaders := DataloaderFor(ctx)
	return loaders.TaskLoader.Load(ctx, taskID)
}
