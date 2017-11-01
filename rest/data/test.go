package data

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
)

// DBTestConnector is a struct that implements the Test related methods
// from the Connector through interactions with the backing database.
type DBTestConnector struct{}

// TODO Fix this after PM-810
func (tc *DBTestConnector) FindTestsByTaskId(taskId, testFilename, status string, limit,
	sortDir int) ([]task.TestResult, error) {

	return []task.TestResult{}, &rest.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "find tests by task id is not implemented pending PM-810",
	}
}

// MockTaskConnector stores a cached set of tests that are queried against by the
// implementations of the Connector interface's Test related functions.
type MockTestConnector struct {
	CachedTests []task.TestResult
	StoredError error
}

// FindTestsBytaskId
func (mtc *MockTestConnector) FindTestsByTaskId(taskId, testFilename, status string, limit,
	sortDir int) ([]task.TestResult, error) {
	if mtc.StoredError != nil {
		return []task.TestResult{}, mtc.StoredError
	}

	// loop until the filename is found
	for ix, t := range mtc.CachedTests {
		if t.TestFile == testFilename {
			// We've found the test
			var testsToReturn []task.TestResult
			if sortDir < 0 {
				if ix-limit > 0 {
					testsToReturn = mtc.CachedTests[ix-(limit) : ix]
				} else {
					testsToReturn = mtc.CachedTests[:ix]
				}
			} else {
				if ix+limit > len(mtc.CachedTests) {
					testsToReturn = mtc.CachedTests[ix:]
				} else {
					testsToReturn = mtc.CachedTests[ix : ix+limit]
				}
			}
			return testsToReturn, nil
		}
	}
	return nil, nil
}
