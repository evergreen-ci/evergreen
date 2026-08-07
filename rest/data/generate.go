package data

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// GenerateTasks parses JSON files for `generate.tasks` and stores them for the generate tasks job to
// process. It returns the task the JSON was stored for so the caller can avoid re-fetching it.
func GenerateTasks(ctx context.Context, settings *evergreen.Settings, taskID string, jsonFiles []json.RawMessage) (*task.Task, error) {
	t, err := task.FindOneIdWithGeneratedJSON(ctx, taskID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding task '%s'", taskID)
	}
	if t == nil {
		return nil, errors.Errorf("task '%s' not found", taskID)
	}

	// Don't continue if the generator has already run
	// Return status code 400 to prevent retries
	if t.GeneratedTasks {
		return nil, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    evergreen.TasksAlreadyGeneratedError,
		}
	}

	var files task.GeneratedJSONFiles
	for _, f := range jsonFiles {
		files = append(files, string(f))
	}
	if _, err := task.GeneratedJSONInsertWithS3Fallback(ctx, settings, t, files, evergreen.ProjectStorageMethodDB); err != nil {
		return nil, errors.Wrapf(err, "inserting generated JSON files for task '%s'", t.Id)
	}

	return t, nil
}

// GeneratePoll checks to see if a `generate.tasks` job has finished.
func GeneratePoll(ctx context.Context, taskID string) (bool, string, error) {
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return false, "", errors.Wrapf(err, "finding task '%s'", taskID)
	}
	if t == nil {
		return false, "", errors.Errorf("task '%s' not found", taskID)
	}

	return t.GeneratedTasks, t.GenerateTasksError, nil
}
