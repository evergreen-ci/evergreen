package data

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
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
