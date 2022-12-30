package data

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

// CountTestsByTaskID returns the number of tests for a given task ID and
// execution.
func CountTestsByTaskID(ctx context.Context, taskID string, execution int) (int, error) {
	t, err := task.FindOneIdOldOrNew(taskID, execution)
	if err != nil {
		return 0, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding task '%s'", taskID).Error(),
		}
	}
	if t == nil || t.Execution != execution {
		return 0, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' with execution %d not found", taskID, execution),
		}
	}
	if !t.HasCedarResults {
		return 0, nil
	}

	stats, status, err := apimodels.GetCedarTestResultsStats(ctx, apimodels.GetCedarTestResultsOptions{
		BaseURL:     evergreen.GetEnvironment().Settings().Cedar.BaseURL,
		TaskID:      taskID,
		Execution:   utility.ToIntPtr(execution),
		DisplayTask: t.DisplayOnly,
	})
	if err != nil {
		return 0, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "getting test results stats from Cedar for task '%s'", taskID).Error(),
		}
	}
	if status != http.StatusOK && status != http.StatusNotFound {
		return 0, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("getting test results stats from Cedar for task '%s' returned status %d", taskID, status),
		}
	}

	return stats.TotalCount, nil
}
