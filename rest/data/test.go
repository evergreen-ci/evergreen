package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

func (tc *DBTestConnector) FindTestById(id string) ([]testresult.TestResult, error) {
	if !bson.IsObjectIdHex(id) {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("invalid test id %s", id),
		}
	}
	results, err := testresult.Find(db.Query(bson.M{"_id": bson.ObjectIdHex(id)}))
	if err != nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("test result not found %s", err.Error()),
		}
	}

	return results, nil
}

func (tc *DBTestConnector) GetTestCountByTaskIdAndFilters(taskId, testName string, statuses []string, execution int) (int, error) {
	t, err := task.FindOneIdNewOrOld(taskId)
	if err != nil {
		return 0, errors.Wrapf(err, fmt.Sprintf("error finding task %s", taskId))
	}
	if t == nil {
		return 0, errors.New(fmt.Sprintf("task not found %s", taskId))
	}
	var taskIds []string
	if t.DisplayOnly {
		taskIds = t.ExecutionTasks
	} else {
		taskIds = []string{taskId}
	}
	count, err := testresult.TestResultCount(taskIds, testName, statuses, execution)
	if err != nil {
		return 0, errors.Wrapf(err, fmt.Sprintf("Error counting test results for task %s", taskId))
	}
	return count, nil
}

func (tc *DBTestConnector) FindTestsByTaskId(opts FindTestsByTaskIdOpts) ([]testresult.TestResult, error) {
	t, err := task.FindOneIdNewOrOld(opts.TaskID)
	if err != nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", err.Error()),
		}
	}
	if t == nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", opts.TaskID),
		}
	}
	var taskIDs []string
	if t.DisplayOnly {
		taskIDs = t.ExecutionTasks
	} else {
		taskIDs = []string{opts.TaskID}
	}

	res, err := testresult.TestResultsFilterSortPaginate(testresult.TestResultsFilterSortPaginateOpts{
		TestID:    opts.TestID,
		TaskIDs:   taskIDs,
		TestName:  opts.TestName,
		Statuses:  opts.Statuses,
		SortBy:    opts.SortBy,
		GroupID:   opts.GroupID,
		SortDir:   opts.SortDir,
		Page:      opts.Page,
		Limit:     opts.Limit,
		Execution: opts.Execution,
	})

	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return []testresult.TestResult{}, nil
	}

	return res, nil
}
