package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func captureGripMessages(t *testing.T) *send.InternalSender {
	originalSender := grip.GetSender()
	sender := send.MakeInternalLogger()
	require.NoError(t, grip.SetSender(sender))
	t.Cleanup(func() {
		require.NoError(t, grip.SetSender(originalSender))
	})
	return sender
}

func TestLogTSSError(t *testing.T) {
	t.Run("CanceledRequestIsNotLogged", func(t *testing.T) {
		sender := captureGripMessages(t)

		logTSSError(t.Context(), context.Canceled, nil, time.Second, message.Fields{
			"message":  "test selection request canceled",
			"endpoint": GetTestsStateEndpoint,
		})

		assert.Zero(t, sender.Len())
	})

	t.Run("GetTestsStateTimeoutIsNotLogged", func(t *testing.T) {
		sender := captureGripMessages(t)

		logTSSError(t.Context(), context.DeadlineExceeded, nil, time.Second, message.Fields{
			"message":  "test selection request timed out",
			"endpoint": GetTestsStateEndpoint,
		})

		assert.Zero(t, sender.Len())
	})

	t.Run("MutationTimeoutIsError", func(t *testing.T) {
		sender := captureGripMessages(t)

		logTSSError(t.Context(), context.DeadlineExceeded, nil, time.Second, message.Fields{
			"message":  "test selection request timed out",
			"endpoint": TransitionTestsEndpoint,
		})

		require.Equal(t, 1, sender.Len())
		assert.Equal(t, level.Error, sender.GetMessage().Priority)
	})
}

func TestSelectTestsSetsTimeout(t *testing.T) {
	for _, test := range []struct {
		name             string
		tests            []string
		expectedEndpoint string
	}{
		{
			name:             "ExplicitTestsShouldSetTimeout",
			tests:            []string{"test"},
			expectedEndpoint: SelectTestsEndpoint,
		},
		{
			name:             "AllKnownTestsShouldSetTimeout",
			expectedEndpoint: SelectKnownTestsEndpoint,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sender := captureGripMessages(t)
			setTSSURLForTest(t, "http://tss.example.com")
			startAt := time.Now()
			var capturedPath string
			var capturedBody struct {
				ProjectID        string `json:"project_id"`
				BuildVariantName string `json:"build_variant_name"`
				TaskID           string `json:"task_id"`
				TaskName         string `json:"task_name"`
			}
			originalClient := testSelectionHTTPClient
			testSelectionHTTPClient = &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					deadline, ok := req.Context().Deadline()
					require.True(t, ok)
					assert.WithinDuration(t, startAt.Add(testSelectionSelectTimeout), deadline, time.Second)
					capturedPath = req.URL.Path
					require.NoError(t, json.NewDecoder(req.Body).Decode(&capturedBody))
					return nil, context.DeadlineExceeded
				}),
			}
			t.Cleanup(func() {
				testSelectionHTTPClient = originalClient
			})

			selectedTests, err := SelectTests(t.Context(), model.SelectTestsRequest{
				Project:      "project/name",
				Requester:    evergreen.PatchVersionRequester,
				BuildVariant: "build/variant",
				TaskID:       "task_id",
				TaskName:     "task/name",
				Tests:        test.tests,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Empty(t, selectedTests)
			assert.Equal(t, "/api/test_selection/"+test.expectedEndpoint+"/", capturedPath)
			assert.Equal(t, "project/name", capturedBody.ProjectID)
			assert.Equal(t, "build/variant", capturedBody.BuildVariantName)
			assert.Equal(t, "task_id", capturedBody.TaskID)
			assert.Equal(t, "task/name", capturedBody.TaskName)
			require.Equal(t, 1, sender.Len())
			fields, ok := sender.GetMessage().Message.Raw().(message.Fields)
			require.True(t, ok)
			assert.Equal(t, test.expectedEndpoint, fields["endpoint"])
			assert.Equal(t, true, fields["timeout"])
			assert.Contains(t, fields, "duration_ms")
		})
	}
}

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

	t.Run("UsesBodyEndpoint", func(t *testing.T) {
		var capturedPath string
		var capturedBody struct {
			ProjectID        string   `json:"project_id"`
			BuildVariantName string   `json:"build_variant_name"`
			TaskName         string   `json:"task_name"`
			TestNames        []string `json:"test_names"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), "my/project", "ubuntu/2204", "my/task", []string{"test/name"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test/name": false}, statuses)
		assert.Equal(t, "/api/test_selection/get_tests_state/", capturedPath)
		assert.Equal(t, "my/project", capturedBody.ProjectID)
		assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
		assert.Equal(t, "my/task", capturedBody.TaskName)
		assert.Equal(t, []string{"test/name"}, capturedBody.TestNames)
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

	t.Run("ServiceErrorReturnsDefaultStatuses", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("CanceledContextReturnsDefaultStatuses", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"test_a":{"state":"manually_quarantined"}}`))
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		statuses, err := GetTestsQuarantineStatus(ctx, projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("NotFoundReturnsDefaultStatuses", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
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

func TestSetTestQuarantined(t *testing.T) {
	var capturedPath string
	var capturedBody struct {
		ProjectID             string   `json:"project_id"`
		BuildVariantName      string   `json:"build_variant_name"`
		TaskName              string   `json:"task_name"`
		TestNames             []string `json:"test_names"`
		IsManuallyQuarantined bool     `json:"is_manually_quarantined"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(srv.Close)
	setTSSURLForTest(t, srv.URL)

	require.NoError(t, SetTestQuarantined(t.Context(), "my/project", "ubuntu/2204", "my/task", "test/name", true))
	assert.Equal(t, "/api/test_selection/transition_tests/", capturedPath)
	assert.Equal(t, "my/project", capturedBody.ProjectID)
	assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
	assert.Equal(t, "my/task", capturedBody.TaskName)
	assert.Equal(t, []string{"test/name"}, capturedBody.TestNames)
	assert.True(t, capturedBody.IsManuallyQuarantined)
}

func TestSetTaskQuarantined(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
		taskName  = "my_task"
	)

	t.Run("SuccessfulCallReturnsNoError", func(t *testing.T) {
		var capturedPath string
		var capturedBody struct {
			ProjectID             string `json:"project_id"`
			BuildVariantName      string `json:"build_variant_name"`
			TaskName              string `json:"task_name"`
			IsManuallyQuarantined bool   `json:"is_manually_quarantined"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		require.NoError(t, SetTaskQuarantined(t.Context(), "my/project", "ubuntu/2204", "my/task", true))
		assert.Equal(t, "/api/test_selection/transition_task/", capturedPath)
		assert.Equal(t, "my/project", capturedBody.ProjectID)
		assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
		assert.Equal(t, "my/task", capturedBody.TaskName)
		assert.True(t, capturedBody.IsManuallyQuarantined)
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
	t.Run("SuccessfulCallReturnsNoError", func(t *testing.T) {
		var capturedPath string
		var capturedBody struct {
			ProjectID             string `json:"project_id"`
			BuildVariantName      string `json:"build_variant_name"`
			IsManuallyQuarantined bool   `json:"is_manually_quarantined"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		require.NoError(t, SetVariantQuarantined(t.Context(), "my/project", "ubuntu/2204", false))
		assert.Equal(t, "/api/test_selection/transition_variant/", capturedPath)
		assert.Equal(t, "my/project", capturedBody.ProjectID)
		assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
		assert.False(t, capturedBody.IsManuallyQuarantined)
	})
}

func TestGetVariantQuarantineStatus(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
	)

	t.Run("UsesBodyEndpoint", func(t *testing.T) {
		var capturedPath string
		var capturedBody struct {
			ProjectID        string `json:"project_id"`
			BuildVariantName string `json:"build_variant_name"`
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		tasks, err := GetVariantQuarantineStatus(t.Context(), "my/project", "ubuntu/2204")
		require.NoError(t, err)
		assert.Empty(t, tasks)
		assert.Equal(t, "/api/test_selection/get_variant_state/", capturedPath)
		assert.Equal(t, "my/project", capturedBody.ProjectID)
		assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
	})

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
	// per-task-name map.
	statusServer := func(t *testing.T, statesByTaskName map[string]map[string]string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			var request struct {
				TaskName string `json:"task_name"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			body := map[string]map[string]any{}
			for testName, state := range statesByTaskName[request.TaskName] {
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

	t.Run("ExecutionTaskTSSFailureDoesNotError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		parent := &task.Task{
			Id:                   "exec_task",
			Project:              "p",
			BuildVariant:         "bv",
			DisplayName:          "exec_task",
			TestSelectionEnabled: true,
		}
		results := []testresult.TestResult{{TaskID: "exec_task", TestName: "TestFoo"}}
		require.NoError(t, DecorateQuarantineStatus(t.Context(), parent, results))
		assert.False(t, results[0].IsManuallyQuarantined)
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

func TestRecordQuarantinedTestsSkipped(t *testing.T) {
	const (
		projectID = "my_project"
		bvName    = "ubuntu"
		taskName  = "my_task"
		taskID    = "my_task_id"
	)
	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)

	taskOutput := task.TaskOutput{
		TestResults: task.TestResultOutput{Version: task.TestResultServiceEvergreen},
	}
	baseReq := model.SelectTestsRequest{
		Project:      projectID,
		Requester:    evergreen.PatchVersionRequester,
		BuildVariant: bvName,
		TaskID:       taskID,
		TaskName:     taskName,
	}

	setupTask := func(t *testing.T) {
		require.NoError(t, db.ClearCollections(task.Collection))
		require.NoError(t, task.ClearTestResults(ctx, env))
		t.Cleanup(func() {
			assert.NoError(t, db.ClearCollections(task.Collection))
			assert.NoError(t, task.ClearTestResults(context.Background(), env))
		})
		tsk := &task.Task{
			Id:             taskID,
			Execution:      0,
			Project:        projectID,
			BuildVariant:   bvName,
			DisplayName:    taskName,
			Version:        "version_id",
			Requester:      evergreen.PatchVersionRequester,
			Status:         evergreen.TaskStarted,
			TaskOutputInfo: &taskOutput,
		}
		require.NoError(t, tsk.Insert(ctx))
	}

	newTSSServer := func(t *testing.T, testStates map[string]map[string]any, variantState map[string]map[string]any) *int {
		hits := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.Header().Set("Content-Type", "application/json")
			var body any = testStates
			if strings.Contains(r.URL.Path, "get_variant_state") {
				body = variantState
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)
		return &hits
	}

	findRecord := func(t *testing.T) testresult.DbTaskTestResults {
		var record testresult.DbTaskTestResults
		require.NoError(t, env.CedarDB().Collection(testresult.Collection).FindOne(ctx, task.ByTaskIDAndExecution(taskID, 0)).Decode(&record))
		return record
	}
	countRecords := func(t *testing.T) int64 {
		count, err := env.CedarDB().Collection(testresult.Collection).CountDocuments(ctx, task.ByTaskIDAndExecution(taskID, 0))
		require.NoError(t, err)
		return count
	}
	findTask := func(t *testing.T) *task.Task {
		dbTask, err := task.FindOneId(ctx, taskID)
		require.NoError(t, err)
		require.NotNil(t, dbTask)
		return dbTask
	}

	t.Run("NamedTestsPathRecordsQuarantineSkippedTests", func(t *testing.T) {
		setupTask(t)
		newTSSServer(t, map[string]map[string]any{
			"test1": {"state": "manually_quarantined"},
			"test2": {"state": "stable"},
		}, nil)

		req := baseReq
		req.Tests = []string{"test0", "test1", "test2"}
		require.NoError(t, RecordQuarantinedTestsSkipped(ctx, env, req, []string{"test0"}))

		record := findRecord(t)
		assert.Equal(t, record.Info.ID(), record.ID)
		assert.Equal(t, projectID, record.Info.Project)
		assert.Equal(t, taskName, record.Info.TaskName)
		assert.Equal(t, 1, record.QuarantinedTestsCount)
		assert.Equal(t, []testresult.QuarantinedTest{{TestName: "test1"}}, record.QuarantinedTests)
		assert.Equal(t, 1, findTask(t).NumQuarantinedTestsSkipped)
	})

	t.Run("AllTestsSelectedSkipsStatusCheckAndRecordsNothing", func(t *testing.T) {
		setupTask(t)
		hits := newTSSServer(t, nil, nil)

		req := baseReq
		req.Tests = []string{"test0", "test1"}
		require.NoError(t, RecordQuarantinedTestsSkipped(ctx, env, req, []string{"test0", "test1"}))

		assert.Zero(t, *hits, "no status call should be made when no tests were skipped")
		assert.Zero(t, countRecords(t))
		assert.Zero(t, findTask(t).NumQuarantinedTestsSkipped)
	})

	t.Run("KnownTestsPathUsesVariantState", func(t *testing.T) {
		setupTask(t)
		newTSSServer(t, nil, map[string]map[string]any{
			taskName: {
				"task_name": taskName,
				"test_stats": map[string]any{
					"test0": map[string]any{"state": "stable"},
					"test1": map[string]any{"state": "manually_quarantined"},
					"test2": map[string]any{"state": "manually_quarantined"},
					"test3": map[string]any{"state": "manually_quarantined"},
				},
			},
		})

		require.NoError(t, RecordQuarantinedTestsSkipped(ctx, env, baseReq, []string{"test0", "test3"}))

		record := findRecord(t)
		assert.Equal(t, 2, record.QuarantinedTestsCount)
		assert.Equal(t, []testresult.QuarantinedTest{{TestName: "test1"}, {TestName: "test2"}}, record.QuarantinedTests, "quarantined tests that were still selected should not be recorded")
		assert.Equal(t, 2, findTask(t).NumQuarantinedTestsSkipped)
	})

	t.Run("FullyQuarantinedTaskRecordsAndReturnsSnapshot", func(t *testing.T) {
		setupTask(t)
		newTSSServer(t, map[string]map[string]any{
			"test0": {"state": "manually_quarantined"},
			"test1": {"state": "manually_quarantined"},
		}, nil)

		req := baseReq
		req.Tests = []string{"test0", "test1"}
		require.NoError(t, RecordQuarantinedTestsSkipped(ctx, env, req, nil))

		dbTask := findTask(t)
		assert.Equal(t, 2, dbTask.NumQuarantinedTestsSkipped)
		samples, err := task.GetQuarantinedTestSamples(ctx, env, []task.Task{*dbTask}, 10)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, 2, samples[0].QuarantinedTestsSkippedCount)
		assert.Len(t, samples[0].QuarantinedTests, 2)
	})
}
