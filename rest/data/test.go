package data

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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

func (tc *DBTestConnector) FindTestsByTaskId(taskId, testId, testName, status string, limit, execution int) ([]testresult.TestResult, error) {
	t, err := task.FindOneIdNewOrOld(taskId)
	if err != nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", err.Error()),
		}
	}
	if t == nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", taskId),
		}
	}
	var taskIds []string
	if t.DisplayOnly {
		taskIds = t.ExecutionTasks
	} else {
		taskIds = []string{taskId}
	}
	q := testresult.TestResultsQuery(taskIds, testId, testName, status, limit, execution)
	return testresult.Find(q)
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

func (tc *DBTestConnector) FindTestsByTaskIdFilterSortPaginate(taskId, testName string, statuses []string, sortBy string, sortDir, page, limit, execution int) ([]testresult.TestResult, error) {
	t, err := task.FindOneIdNewOrOld(taskId)
	if err != nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", err.Error()),
		}
	}
	if t == nil {
		return []testresult.TestResult{}, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task not found %s", taskId),
		}
	}
	var taskIds []string
	if t.DisplayOnly {
		taskIds = t.ExecutionTasks
	} else {
		taskIds = []string{taskId}
	}

	res, err := testresult.TestResultsFilterSortPaginate(taskIds, testName, statuses, sortBy, sortDir, page, limit, execution)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return []testresult.TestResult{}, nil
	}

	return res, nil
}

// MockTaskConnector stores a cached set of tests that are queried against by the
// implementations of the Connector interface's Test related functions.
type MockTestConnector struct {
	CachedTests []testresult.TestResult
	StoredError error
}

func (mtc *MockTestConnector) FindTestById(id string) ([]testresult.TestResult, error) {
	return nil, nil
}

func (mtc *MockTestConnector) FindTestsByTaskId(taskId, testId, testName, status string, limit, execution int) ([]testresult.TestResult, error) {
	if mtc.StoredError != nil {
		return []testresult.TestResult{}, mtc.StoredError
	}

	// loop until the testId is found
	for ix, t := range mtc.CachedTests {
		if string(t.ID) == testId { // We've found the test to start from
			if testName == "" {
				return mtc.findAllTestsFromIx(ix, limit), nil
			}
			return mtc.findTestsByNameFromIx(testName, ix, limit), nil
		}
	}
	return nil, nil
}
func (tc *MockTestConnector) GetTestCountByTaskIdAndFilters(taskId, testName string, statuses []string, execution int) (int, error) {
	return 0, nil
}
func (tc *MockTestConnector) FindTestsByTaskIdFilterSortPaginate(taskId, filter string, statuses []string, sortBy string, sortDir, page, limit, execution int) ([]testresult.TestResult, error) {
	//todo
	return nil, nil
}

func (mtc *MockTestConnector) findAllTestsFromIx(ix, limit int) []testresult.TestResult {
	if ix+limit > len(mtc.CachedTests) {
		return mtc.CachedTests[ix:]
	}
	return mtc.CachedTests[ix : ix+limit]
}

func (mtc *MockTestConnector) findTestsByNameFromIx(name string, ix, limit int) []testresult.TestResult {
	possibleTests := mtc.CachedTests[ix:]
	testResults := []testresult.TestResult{}
	for jx, t := range possibleTests {
		if t.TestFile == name {
			testResults = append(testResults, possibleTests[jx])
		}
		if len(testResults) == limit {
			return testResults
		}
	}
	return testResults
}
