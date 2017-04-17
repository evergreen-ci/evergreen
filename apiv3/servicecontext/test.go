package servicecontext

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/model/task"
)

// DBTestConnector is a struct that implements the Test related methods
// from the ServiceContext through interactions with the backing database.
type DBTestConnector struct{}

func (tc *DBTestConnector) FindTestsByTaskId(taskId, testFilename, status string, limit,
	sortDir int) ([]task.TestResult, error) {
	pipeline := task.TestResultsByTaskIdPipeline(taskId, testFilename, status, limit, sortDir)
	res := []task.TestResult{}

	err := task.Aggregate(pipeline, &res)
	if err != nil {
		return []task.TestResult{}, err
	}
	if len(res) == 0 {
		var message string
		if status != "" {
			message = fmt.Sprintf("tests for task with taskId '%s' and status '%s' not found", taskId, status)
		} else {
			message = fmt.Sprintf("tests for task with taskId '%s' not found", taskId)
		}
		return []task.TestResult{}, &apiv3.APIError{
			StatusCode: http.StatusNotFound,
			Message:    message,
		}
	}

	if testFilename != "" {
		found := false
		for _, t := range res {
			if t.TestFile == testFilename {
				found = true
				break
			}
		}
		if !found {
			return []task.TestResult{}, &apiv3.APIError{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("test with filename %s not found", testFilename),
			}
		}
	}
	return res, nil
}

// MockTaskConnector stores a cached set of tests that are queried against by the
// implementations of the ServiceContext interface's Test related functions.
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

type testSorter []task.TestResult

func (ts testSorter) Len() int {
	return len(ts)
}

func (ts testSorter) Less(i, j int) bool {
	return ts[i].TestFile < ts[j].TestFile
}
func (ts testSorter) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}
