package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// virtualTaskTestResultsRequest is the request body for uploading virtual
// tasks' test results using the agent's local status server.
type virtualTaskTestResultsRequest struct {
	Tasks []virtualTaskTestResults `json:"tasks"`
}

type virtualTaskTestResults struct {
	TaskID      string                              `json:"task_id"`
	Execution   int                                 `json:"execution"`
	TestResults command.VirtualTaskTestResultsInput `json:"test_results"`
}

// virtualTaskTestResultsResponse contains the per-virtual-task outcomes of
// test results upload.
type virtualTaskTestResultsResponse struct {
	Results []virtualTaskTestResultsResult `json:"results"`
}

type virtualTaskTestResultsResult struct {
	TaskID      string                            `json:"task_id"`
	TestResults *apimodels.VirtualTaskTestResults `json:"test_results,omitempty"`
	Error       string                            `json:"error,omitempty"`
}

// uploadVirtualTaskTestResultsHandler uploads test results for virtual tasks.
// It returns each task's test result stats so the caller can push-complete the
// tasks itself.
// Callers are expected to upload all test results for a virtual task in a
// single request.
func (a *Agent) uploadVirtualTaskTestResultsHandler(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, errors.Wrap(err, "reading virtual task test results request body"))
		return
	}
	var req virtualTaskTestResultsRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		writeAgentHandlerError(w, http.StatusBadRequest, errors.Wrap(err, "unmarshalling virtual task test results request body as JSON"))
		return
	}

	a.uploadVirtualTaskTestResultsMutex.RLock()
	upload := a.uploadVirtualTaskTestResults
	a.uploadVirtualTaskTestResultsMutex.RUnlock()
	if upload == nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, errors.New("no task is currently running"))
		return
	}

	// Bound the number of test result uploads running at once so a task can't
	// overwhelm the host by uploading many test results simultaneously.
	select {
	case a.virtualTaskTestResultsRequestSem <- struct{}{}:
		defer func() { <-a.virtualTaskTestResultsRequestSem }()
	case <-r.Context().Done():
		writeAgentHandlerError(w, http.StatusServiceUnavailable, errors.Wrap(r.Context().Err(), "waiting to upload virtual task test results"))
		return
	}

	resp, err := upload(r.Context(), req)
	if err != nil {
		writeAgentHandlerError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		grip.Error(r.Context(), errors.Wrap(err, "writing virtual task test results response"))
	}
}

// uploadVirtualTaskTestResults prepares and uploads test results for each of the
// given virtual tasks using the runner's task output, and returns the per-task
// outcomes. A failure to upload an individual task's results is reported in that
// task's result rather than failing the whole request.
func (tc *taskContext) uploadVirtualTaskTestResults(ctx context.Context, comm client.Communicator, req virtualTaskTestResultsRequest) (*virtualTaskTestResultsResponse, error) {
	if len(req.Tasks) == 0 {
		return nil, errors.New("must specify at least one virtual task")
	}

	preparations := make([]apimodels.VirtualTaskPreparation, len(req.Tasks))
	for i, task := range req.Tasks {
		preparations[i] = apimodels.VirtualTaskPreparation{TaskID: task.TaskID, Execution: task.Execution}
	}
	prep, err := comm.PrepareVirtualTasks(ctx, tc.task, preparations)
	if err != nil {
		return nil, errors.Wrap(err, "preparing virtual tasks for test result upload")
	}
	prepared := make(map[string]apimodels.VirtualTaskPreparationResult, len(prep.Results))
	for _, result := range prep.Results {
		prepared[result.TaskID] = result
	}

	resp := &virtualTaskTestResultsResponse{Results: make([]virtualTaskTestResultsResult, 0, len(req.Tasks))}
	for _, task := range req.Tasks {
		resp.Results = append(resp.Results, tc.uploadEntryTestResults(ctx, prepared[task.TaskID], task))
	}
	return resp, nil
}

func (tc *taskContext) uploadEntryTestResults(ctx context.Context, prep apimodels.VirtualTaskPreparationResult, task virtualTaskTestResults) virtualTaskTestResultsResult {
	result := virtualTaskTestResultsResult{TaskID: task.TaskID}

	if prep.TaskID == "" {
		result.Error = "virtual task was not prepared for test result upload"
		return result
	}
	if prep.Outcome != apimodels.VirtualTaskCompletionOutcomeSuccess || prep.Reason != "" {
		result.Error = errors.Errorf("cannot upload test results for virtual task '%s': %s", task.TaskID, prep.Reason).Error()
		return result
	}

	testResultsCreatedAt := task.TestResults.GetCreatedAt()
	stats, err := command.ProcessVirtualTaskTestResultFiles(ctx, tc.taskConfig, tc.logger, *prep.TestResultsInfo, prep.TaskCreateTime, testResultsCreatedAt, task.TestResults.Files)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.TestResults = stats
	return result
}
