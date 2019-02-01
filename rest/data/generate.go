package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

type GenerateConnector struct{}

// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
func (gc *GenerateConnector) GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage, q amboy.Queue) error {
	return q.Put(units.NewGenerateTasksJob(taskID, jsonBytes))
}

func (gc *GenerateConnector) GeneratePoll(ctx context.Context, taskID string, queue amboy.Queue) (bool, error) {
	j, exists := queue.Get(fmt.Sprintf("generate-tasks-%s", taskID))
	if !exists {
		return false, errors.Errorf("task %s not in queue", taskID)
	}
	return j.Status().Completed, nil
}

type MockGenerateConnector struct{}

func (gc *MockGenerateConnector) GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage, q amboy.Queue) error {
	return nil
}

func (gc *MockGenerateConnector) GeneratePoll(ctx context.Context, taskID string, queue amboy.Queue) (bool, error) {
	// no task
	if taskID == "0" {
		return false, errors.New("No task called '0'")
	}
	// finished
	if taskID == "1" {
		return true, nil
	}
	// not yet finished
	return false, nil
}
