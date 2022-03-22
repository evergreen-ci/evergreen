package data

import (
	"context"
	"encoding/json"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
)

// GenerateTasks parses JSON files for `generate.tasks` and creates the new builds and tasks.
func GenerateTasks(ctx context.Context, taskID string, jsonBytes []json.RawMessage, group amboy.QueueGroup) error {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return errors.Wrapf(err, "problem finding task %s", taskID)
	}
	if t == nil {
		return errors.Errorf("could not find task %s", taskID)
	}

	// Don't continue if the generator has already run
	if t.GeneratedTasks {
		return errors.New(evergreen.TasksAlreadyGeneratedError)
	}

	if err = t.SetGeneratedJSON(jsonBytes); err != nil {
		return errors.Wrapf(err, "problem setting generated json in task document for %s", t.Id)
	}

	return nil
}

// GeneratePoll checks to see if a `generate.tasks` job has finished.
func GeneratePoll(ctx context.Context, taskID string, group amboy.QueueGroup) (bool, []string, error) {
	t, err := task.FindOneId(taskID)
	if err != nil {
		return false, nil, errors.Wrapf(err, "problem finding task %s", taskID)
	}
	if t == nil {
		return false, nil, errors.Errorf("could not find task %s", taskID)
	}

	var errs []string
	if t.GenerateTasksError != "" {
		errs = []string{t.GenerateTasksError}
	}
	return t.GeneratedTasks, errs, nil
}
