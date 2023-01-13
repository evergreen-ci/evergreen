package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

type CedarHandler struct {
	Response    []byte
	StatusCode  int
	LastRequest *http.Request
}

func (h *CedarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.LastRequest = r
	if h.StatusCode > 0 {
		w.WriteHeader(h.StatusCode)
	}
	_, _ = w.Write(h.Response)
}

func (h *CedarHandler) SetTestResults(results []task.TestResult, filteredCount *int) error {
	cedarTestResults := apimodels.CedarTestResults{
		Stats: apimodels.CedarTestResultsStats{
			TotalCount:    len(results),
			FilteredCount: filteredCount,
		},
		Results: make([]apimodels.CedarTestResult, len(results)),
	}
	for i, result := range results {
		cedarTestResults.Results[i] = apimodels.CedarTestResult{
			TaskID:          result.TaskID,
			Execution:       result.Execution,
			TestName:        result.TestFile,
			DisplayTestName: result.DisplayTestName,
			GroupID:         result.GroupID,
			LogTestName:     result.LogTestName,
			LogURL:          result.URL,
			RawLogURL:       result.URLRaw,
			LineNum:         result.LineNum,
			Start:           time.Unix(int64(result.StartTime), 0),
			End:             time.Unix(int64(result.EndTime), 0),
			Status:          result.Status,
		}
		if result.Status == evergreen.TestFailedStatus {
			cedarTestResults.Stats.FailedCount++
		}
	}

	var err error
	h.Response, err = json.Marshal(&cedarTestResults)
	return errors.Wrap(err, "marshalling Cedar test results")
}

func NewCedarServer(env evergreen.Environment) (*httptest.Server, *CedarHandler) {
	handler := &CedarHandler{}
	srv := httptest.NewServer(handler)

	if env == nil {
		env = evergreen.GetEnvironment()
	}
	env.Settings().Cedar.BaseURL = srv.URL

	return srv, handler
}
