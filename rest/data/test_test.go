package data

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountTestsByTaskID(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("CedarTestResults", func(t *testing.T) {
		regularTask := &task.Task{
			Id:              "cedar",
			HasCedarResults: true,
		}
		require.NoError(t, regularTask.Insert())
		displayTask := &task.Task{
			Id:              "cedar_display",
			Execution:       1,
			HasCedarResults: true,
			DisplayOnly:     true,
		}
		require.NoError(t, displayTask.Insert())
		srv, handler := mock.NewCedarServer(nil)
		defer srv.Close()

		for _, test := range []struct {
			name       string
			task       *task.Task
			execution  int
			response   apimodels.CedarTestResultsStats
			statusCode int
			hasErr     bool
		}{
			{
				name: "TaskDNE",
				task: &task.Task{
					Id:              "DNE",
					HasCedarResults: true,
				},
				statusCode: http.StatusOK,
				hasErr:     true,
			},
			{
				name:       "TaskExecutionDNE",
				task:       regularTask,
				execution:  100,
				statusCode: http.StatusOK,
				hasErr:     true,
			},
			{
				name:       "CedarError",
				task:       regularTask,
				execution:  regularTask.Execution,
				statusCode: http.StatusInternalServerError,
				hasErr:     true,
			},
			{
				name:       "TaskWithoutResults",
				task:       regularTask,
				execution:  regularTask.Execution,
				statusCode: http.StatusNotFound,
			},
			{
				name:       "TaskWithResults",
				task:       regularTask,
				execution:  regularTask.Execution,
				response:   apimodels.CedarTestResultsStats{TotalCount: 100},
				statusCode: http.StatusOK,
			},
			{
				name:       "DisplayTask",
				task:       displayTask,
				execution:  displayTask.Execution,
				response:   apimodels.CedarTestResultsStats{TotalCount: 100},
				statusCode: http.StatusOK,
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				responseData, err := json.Marshal(&test.response)
				require.NoError(t, err)
				handler.Responses = [][]byte{responseData}
				handler.StatusCode = test.statusCode

				count, err := CountTestsByTaskID(ctx, test.task.Id, test.execution)
				if test.hasErr {
					assert.Error(t, err)
				} else {
					require.NoError(t, err)

					vals := handler.LastRequest.URL.Query()
					var displayStr string
					if test.task.DisplayOnly {
						displayStr = "true"
					}
					assert.Equal(t, strconv.Itoa(test.task.Execution), vals.Get("execution"))
					assert.Equal(t, displayStr, vals.Get("display_task"))
					assert.Equal(t, test.response.TotalCount, count)
				}
			})
		}
	})
}
