package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/gimlet"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

func FindTestById(id string) ([]testresult.TestResult, error) {
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
