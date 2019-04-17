package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

type GenerateConnector struct{}

// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
func (gc *GenerateConnector) GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage, group amboy.QueueGroup) error {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return errors.Wrapf(err, "problem finding task %s", taskID)
	}
	q, err := group.Get(ctx, t.Version)
	if err != nil {
		return errors.Wrapf(err, "problem getting queue for version %s", t.Version)
	}
	return q.Put(units.NewGenerateTasksJob(taskID, jsonBytes))
}

func (gc *GenerateConnector) GeneratePoll(ctx context.Context, taskID string, group amboy.QueueGroup) (bool, []string, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return false, nil, errors.Wrapf(err, "problem finding task %s", taskID)
	}
	q, err := group.Get(ctx, t.Version)
	if err != nil {
		return false, nil, errors.Wrapf(err, "problem getting queue for version %s", t.Version)
	}
	jobID := fmt.Sprintf("generate-tasks-%s", taskID)
	j, exists := q.Get(jobID)
	if !exists {
		return false, nil, errors.Errorf("task %s not in queue", taskID)
	}
	return j.Status().Completed, j.Status().Errors, nil
}

type MockGenerateConnector struct{}

func (gc *MockGenerateConnector) GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage, group amboy.QueueGroup) error {
	return nil
}

func (gc *MockGenerateConnector) GeneratePoll(ctx context.Context, taskID string, queue amboy.QueueGroup) (bool, []string, error) {
	// no task
	if taskID == "0" {
		return false, nil, errors.New("No task called '0'")
	}
	// finished
	if taskID == "1" {
		return true, nil, nil
	}
	// not yet finished
	return false, nil, nil
}
