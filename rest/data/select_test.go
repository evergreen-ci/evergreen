package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTestsQuarantineStatus(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
		taskName  = "my_task"
	)

	// Swap the TSS URL for each test. The outer setter lets us reuse the env
	// installed by testutil's init().
	setTSSURL := func(t *testing.T, url string) {
		original := evergreen.GetEnvironment().Settings().TestSelection.URL
		evergreen.GetEnvironment().Settings().TestSelection.URL = url
		t.Cleanup(func() {
			evergreen.GetEnvironment().Settings().TestSelection.URL = original
		})
	}

	// newServer returns a server that replies to any GetTestsState request with
	// `body` serialized as JSON. It also records how many times it was hit.
	newServer := func(t *testing.T, body map[string]map[string]any) (*httptest.Server, *int) {
		hits := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}))
		t.Cleanup(srv.Close)
		return srv, &hits
	}

	t.Run("EmptyTestNamesSkipsHTTPCall", func(t *testing.T) {
		srv, hits := newServer(t, nil)
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, nil)
		require.NoError(t, err)
		assert.Empty(t, statuses)
		assert.Zero(t, *hits, "no HTTP call should be made for empty input")
	})

	t.Run("StateManuallyQuarantinedReturnsTrue", func(t *testing.T) {
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true}, statuses)
	})

	t.Run("NonQuarantinedStateReturnsFalse", func(t *testing.T) {
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "stable"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("OverrideStateTakesPrecedenceOverStateTrueCase", func(t *testing.T) {
		// State alone would return false; OverrideState flips it to true.
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "stable", "override_state": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true}, statuses)
	})

	t.Run("OverrideStateTakesPrecedenceOverStateFalseCase", func(t *testing.T) {
		// State alone would return true; OverrideState flips it to false.
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined", "override_state": "stable"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("ExplicitNullOverrideStateFallsBackToState", func(t *testing.T) {
		// override_state present but null should not override; State wins.
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined", "override_state": nil},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true}, statuses)
	})

	t.Run("MissingTestInResponseDefaultsToFalse", func(t *testing.T) {
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a", "test_missing"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true, "test_missing": false}, statuses)
	})

	t.Run("ServiceErrorReturnsWrappedError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		_, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "forwarding request to test selection service")
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("NotFoundStillReturnsWrappedError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		_, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "forwarding request to test selection service")
	})
}

// setTSSURLForTest swaps the TSS URL in env settings and restores it on test cleanup.
func setTSSURLForTest(t *testing.T, url string) {
	original := evergreen.GetEnvironment().Settings().TestSelection.URL
	evergreen.GetEnvironment().Settings().TestSelection.URL = url
	t.Cleanup(func() {
		evergreen.GetEnvironment().Settings().TestSelection.URL = original
	})
}

func TestSetTaskQuarantined(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
		taskName  = "my_task"
	)

	t.Run("SuccessfulCallReturnsNoError", func(t *testing.T) {
		var capturedPath, capturedQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		require.NoError(t, SetTaskQuarantined(t.Context(), projectID, bvName, taskName, true))
		assert.Equal(t, fmt.Sprintf("/api/test_selection/%s/my_project/ubuntu/my_task/", TransitionTaskEndpoint), capturedPath)
		assert.Contains(t, capturedQuery, "is_manually_quarantined=true")
	})

	t.Run("ServiceErrorIncludesBody", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		err := SetTaskQuarantined(t.Context(), projectID, bvName, taskName, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "forwarding request to test selection service")
		assert.Contains(t, err.Error(), "boom")
	})
}

func TestSetVariantQuarantined(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
	)

	t.Run("SuccessfulCallReturnsNoError", func(t *testing.T) {
		var capturedPath, capturedQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		require.NoError(t, SetVariantQuarantined(t.Context(), projectID, bvName, false))
		assert.Equal(t, fmt.Sprintf("/api/test_selection/%s/my_project/ubuntu/", TransitionVariantEndpoint), capturedPath)
		assert.Contains(t, capturedQuery, "is_manually_quarantined=false")
	})
}

func TestGetVariantQuarantineStatus(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
	)

	t.Run("EmptyVariantReturnsEmptyMap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		tasks, err := GetVariantQuarantineStatus(t.Context(), projectID, bvName)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("PopulatesNestedMapFromResponse", func(t *testing.T) {
		body := map[string]map[string]any{
			"task_a": {
				"task_name": "task_a",
				"test_stats": map[string]any{
					"test_1": map[string]any{"state": "manually_quarantined"},
					"test_2": map[string]any{"state": "stable"},
				},
			},
			"task_b": {
				"task_name": "task_b",
				"test_stats": map[string]any{
					"test_3": map[string]any{"state": "stable", "override_state": "manually_quarantined"},
				},
			},
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		tasks, err := GetVariantQuarantineStatus(t.Context(), projectID, bvName)
		require.NoError(t, err)
		assert.Equal(t, map[string]map[string]bool{
			"task_a": {"test_1": true, "test_2": false},
			"task_b": {"test_3": true},
		}, tasks)
	})
}

func TestDecorateQuarantineStatus(t *testing.T) {
	setTSSURL := func(t *testing.T, url string) {
		original := evergreen.GetEnvironment().Settings().TestSelection.URL
		evergreen.GetEnvironment().Settings().TestSelection.URL = url
		t.Cleanup(func() {
			evergreen.GetEnvironment().Settings().TestSelection.URL = original
		})
	}

	// statusServer returns a TSS server that resolves quarantine state from a
	// per-task-name map. Path segment matching keeps each execution task's
	// response isolated.
	statusServer := func(t *testing.T, statesByTaskName map[string]map[string]string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			var taskName string
			for name := range statesByTaskName {
				if strings.Contains(r.URL.Path, "/"+name+"/") {
					taskName = name
					break
				}
			}
			body := map[string]map[string]any{}
			for testName, state := range statesByTaskName[taskName] {
				body[testName] = map[string]any{"state": state}
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("NoOpWhenTestSelectionDisabled", func(t *testing.T) {
		parent := &task.Task{
			Id:                   "exec_task",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "exec_task",
			TestSelectionEnabled: false,
		}
		results := []testresult.TestResult{{TaskID: "exec_task", TestName: "TestFoo"}}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), parent, results))
		assert.False(t, results[0].IsManuallyQuarantined)
	})

	t.Run("NoOpWhenResultsEmpty", func(t *testing.T) {
		parent := &task.Task{
			Id:                   "exec_task",
			TestSelectionEnabled: true,
		}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), parent, nil))
	})

	t.Run("ExecutionTaskUsesParentTaskFields", func(t *testing.T) {
		srv := statusServer(t, map[string]map[string]string{
			"exec_task": {"TestFoo": "manually_quarantined", "TestBar": "stable"},
		})
		setTSSURL(t, srv.URL)

		parent := &task.Task{
			Id:                   "exec_task",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "exec_task",
			TestSelectionEnabled: true,
		}
		results := []testresult.TestResult{
			{TaskID: "exec_task", TestName: "TestFoo"},
			{TaskID: "exec_task", TestName: "TestBar"},
		}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), parent, results))
		assert.True(t, results[0].IsManuallyQuarantined)
		assert.False(t, results[1].IsManuallyQuarantined)
	})

	t.Run("DisplayTaskFansOutAcrossExecutionTasks", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(task.Collection))
		require.NoError(t, (&task.Task{
			Id:                   "exec_a",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "task_a",
			TestSelectionEnabled: true,
		}).Insert(t.Context()))
		require.NoError(t, (&task.Task{
			Id:                   "exec_b",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "task_b",
			TestSelectionEnabled: true,
		}).Insert(t.Context()))

		srv := statusServer(t, map[string]map[string]string{
			"task_a": {"TestA1": "manually_quarantined"},
			"task_b": {"TestB1": "stable", "TestB2": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		display := &task.Task{
			Id:                   "display_task",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "display_task",
			DisplayOnly:          true,
			ExecutionTasks:       []string{"exec_a", "exec_b"},
			TestSelectionEnabled: true,
		}
		results := []testresult.TestResult{
			{TaskID: "exec_a", TestName: "TestA1"},
			{TaskID: "exec_b", TestName: "TestB1"},
			{TaskID: "exec_b", TestName: "TestB2"},
		}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), display, results))
		assert.True(t, results[0].IsManuallyQuarantined, "TestA1 should be quarantined")
		assert.False(t, results[1].IsManuallyQuarantined, "TestB1 should not be quarantined")
		assert.True(t, results[2].IsManuallyQuarantined, "TestB2 should be quarantined")
	})

	t.Run("DisplayTaskSkipsExecutionTaskWithoutTestSelection", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(task.Collection))
		require.NoError(t, (&task.Task{
			Id:                   "exec_enabled",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "enabled",
			TestSelectionEnabled: true,
		}).Insert(t.Context()))
		require.NoError(t, (&task.Task{
			Id:                   "exec_disabled",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "disabled",
			TestSelectionEnabled: false,
		}).Insert(t.Context()))

		srv := statusServer(t, map[string]map[string]string{
			"enabled":  {"TestEnabled": "manually_quarantined"},
			"disabled": {"TestDisabled": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		display := &task.Task{
			Id:                   "display_task",
			DisplayOnly:          true,
			ExecutionTasks:       []string{"exec_enabled", "exec_disabled"},
			TestSelectionEnabled: true,
		}
		results := []testresult.TestResult{
			{TaskID: "exec_enabled", TestName: "TestEnabled"},
			{TaskID: "exec_disabled", TestName: "TestDisabled"},
		}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), display, results))
		assert.True(t, results[0].IsManuallyQuarantined)
		assert.False(t, results[1].IsManuallyQuarantined, "TSS should not be queried for execution tasks without test selection enabled")
	})

	t.Run("DisplayTaskWithMissingExecutionTaskDoesNotError", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(task.Collection))
		// No execution tasks inserted.

		srv := statusServer(t, map[string]map[string]string{})
		setTSSURL(t, srv.URL)

		display := &task.Task{
			Id:                   "display_task",
			DisplayOnly:          true,
			ExecutionTasks:       []string{"missing_exec"},
			TestSelectionEnabled: true,
		}
		results := []testresult.TestResult{
			{TaskID: "missing_exec", TestName: "TestOrphan"},
		}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), display, results))
		assert.False(t, results[0].IsManuallyQuarantined)
	})

	t.Run("DisplayTaskFansOutEvenWhenDisplayTaskTestSelectionDisabled", func(t *testing.T) {
		// Covers the case where the display task's TestSelectionEnabled flag is
		// stale (e.g. a new display task was created over a mix of new and
		// pre-existing execution tasks, and only the pre-existing one had TSS
		// enabled). The fan-out should still decorate based on each execution
		// task's actual state.
		require.NoError(t, db.ClearCollections(task.Collection))
		require.NoError(t, (&task.Task{
			Id:                   "exec_enabled",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "enabled",
			TestSelectionEnabled: true,
		}).Insert(t.Context()))

		srv := statusServer(t, map[string]map[string]string{
			"enabled": {"TestEnabled": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		display := &task.Task{
			Id:                   "display_task",
			DisplayOnly:          true,
			ExecutionTasks:       []string{"exec_enabled"},
			TestSelectionEnabled: false,
		}
		results := []testresult.TestResult{
			{TaskID: "exec_enabled", TestName: "TestEnabled"},
		}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), display, results))
		assert.True(t, results[0].IsManuallyQuarantined)
	})
}
