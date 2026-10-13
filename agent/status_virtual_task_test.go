package agent

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/command"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadVirtualTaskTestResultsHandler(t *testing.T) {
	validBody := `{"tasks":[{"task_id":"virtual_task","execution":0,"test_results":{"files":[{"type":"native","files":["results.json"]}]}}]}`

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, a *Agent){
		"NoTaskRunningErrors": func(ctx context.Context, t *testing.T, a *Agent) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/virtual_tasks/test_results", bytes.NewReader([]byte(validBody)))
			a.uploadVirtualTaskTestResultsHandler(rec, req)
			assert.Equal(t, http.StatusInternalServerError, rec.Code)
		},
		"InvalidJSONErrors": func(ctx context.Context, t *testing.T, a *Agent) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/virtual_tasks/test_results", bytes.NewReader([]byte("not json")))
			a.uploadVirtualTaskTestResultsHandler(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		},
		"SuccessReturnsStats": func(ctx context.Context, t *testing.T, a *Agent) {
			a.uploadVirtualTaskTestResults = func(ctx context.Context, req virtualTaskTestResultsRequest) (*virtualTaskTestResultsResponse, error) {
				require.Len(t, req.Tasks, 1)
				assert.Equal(t, "virtual_task", req.Tasks[0].TaskID)
				return &virtualTaskTestResultsResponse{Results: []virtualTaskTestResultsResult{
					{
						TaskID: "virtual_task",
						TestResults: &apimodels.VirtualTaskTestResults{
							Stats:        testresult.TaskTestResultsStats{TotalCount: 2, FailedCount: 1},
							FailedSample: []string{"failing_test"},
						},
					},
				}}, nil
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/virtual_tasks/test_results", bytes.NewReader([]byte(validBody)))
			a.uploadVirtualTaskTestResultsHandler(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), `"total_count":2`)
			assert.Contains(t, rec.Body.String(), `"failed_count":1`)
			assert.Contains(t, rec.Body.String(), "failing_test")
		},
		"UploadErrorReturnsError": func(ctx context.Context, t *testing.T, a *Agent) {
			a.uploadVirtualTaskTestResults = func(ctx context.Context, req virtualTaskTestResultsRequest) (*virtualTaskTestResultsResponse, error) {
				return nil, errors.New("upload failed")
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/virtual_tasks/test_results", bytes.NewReader([]byte(validBody)))
			a.uploadVirtualTaskTestResultsHandler(rec, req)

			assert.Equal(t, http.StatusInternalServerError, rec.Code)
			assert.Contains(t, rec.Body.String(), "upload failed")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			a := &Agent{virtualTaskTestResultsRequestSem: make(chan struct{}, 1)}
			tCase(t.Context(), t, a)
		})
	}
}

func TestTaskContextUploadVirtualTaskTestResults(t *testing.T) {
	request := virtualTaskTestResultsRequest{Tasks: []virtualTaskTestResults{
		{
			TaskID:    "virtual_task",
			Execution: 0,
			TestResults: command.VirtualTaskTestResultsInput{
				Files: []command.VirtualTaskTestResultFileGroup{{Type: "native", Files: []string{"results.json"}}},
			},
		},
	}}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, tc *taskContext, comm *client.Mock){
		"EmptyTasksErrors": func(ctx context.Context, t *testing.T, tc *taskContext, comm *client.Mock) {
			_, err := tc.uploadVirtualTaskTestResults(ctx, comm, virtualTaskTestResultsRequest{})
			assert.ErrorContains(t, err, "at least one virtual task")
		},
		"PrepareFailureReportsPerTaskError": func(ctx context.Context, t *testing.T, tc *taskContext, comm *client.Mock) {
			comm.PrepareVirtualTasksResponse = &apimodels.PrepareVirtualTasksResponse{Results: []apimodels.VirtualTaskPreparationResult{
				{TaskID: "virtual_task", Outcome: apimodels.VirtualTaskCompletionOutcomeFailed, Reason: "task is not a virtual task"},
			}}
			resp, err := tc.uploadVirtualTaskTestResults(ctx, comm, request)
			require.NoError(t, err)
			require.Len(t, resp.Results, 1)
			assert.Contains(t, resp.Results[0].Error, "task is not a virtual task")
			assert.Nil(t, resp.Results[0].TestResults)
		},
		"PrepareNoopReportsPerTaskError": func(ctx context.Context, t *testing.T, tc *taskContext, comm *client.Mock) {
			comm.PrepareVirtualTasksResponse = &apimodels.PrepareVirtualTasksResponse{Results: []apimodels.VirtualTaskPreparationResult{
				{TaskID: "virtual_task", Outcome: apimodels.VirtualTaskCompletionOutcomeSuccess, Reason: "task is already running"},
			}}
			resp, err := tc.uploadVirtualTaskTestResults(ctx, comm, request)
			require.NoError(t, err)
			require.Len(t, resp.Results, 1)
			assert.Contains(t, resp.Results[0].Error, "task is already running")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx := t.Context()
			tc := &taskContext{task: client.TaskData{ID: "runner", Secret: "secret"}}
			comm := client.NewMock("url")
			tCase(ctx, t, tc, comm)
		})
	}
}
