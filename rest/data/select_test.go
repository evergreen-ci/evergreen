package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", nil)
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
			DisplayTaskName  string   `json:"display_task_name"`
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

		statuses, err := GetTestsQuarantineStatus(t.Context(), "my/project", "ubuntu/2204", "my/task", "my/display/task", []string{"test/name"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test/name": false}, statuses)
		assert.Equal(t, "/api/test_selection/get_tests_state/", capturedPath)
		assert.Equal(t, "my/project", capturedBody.ProjectID)
		assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
		assert.Equal(t, "my/task", capturedBody.TaskName)
		assert.Equal(t, "my/display/task", capturedBody.DisplayTaskName)
		assert.Equal(t, []string{"test/name"}, capturedBody.TestNames)
	})

	t.Run("OmitsDisplayTaskNameWhenNotSet", func(t *testing.T) {
		var rawBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&rawBody))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		_, err := GetTestsQuarantineStatus(t.Context(), "my/project", "ubuntu/2204", "my/task", "", []string{"test/name"})
		require.NoError(t, err)
		assert.NotContains(t, rawBody, "display_task_name", "display task name should be omitted when there is none")
	})

	t.Run("StateManuallyQuarantinedReturnsTrue", func(t *testing.T) {
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true}, statuses)
	})

	t.Run("NonQuarantinedStateReturnsFalse", func(t *testing.T) {
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "stable"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("OverrideStateTakesPrecedenceOverStateTrueCase", func(t *testing.T) {
		// State alone would return false; OverrideState flips it to true.
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "stable", "override_state": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true}, statuses)
	})

	t.Run("OverrideStateTakesPrecedenceOverStateFalseCase", func(t *testing.T) {
		// State alone would return true; OverrideState flips it to false.
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined", "override_state": "stable"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("ExplicitNullOverrideStateFallsBackToState", func(t *testing.T) {
		// override_state present but null should not override; State wins.
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined", "override_state": nil},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true}, statuses)
	})

	t.Run("MissingTestInResponseDefaultsToFalse", func(t *testing.T) {
		srv, _ := newServer(t, map[string]map[string]any{
			"test_a": {"state": "manually_quarantined"},
		})
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a", "test_missing"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": true, "test_missing": false}, statuses)
	})

	t.Run("ServiceErrorReturnsDefaultStatuses", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
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
		statuses, err := GetTestsQuarantineStatus(ctx, projectID, bvName, taskName, "", []string{"test_a"})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"test_a": false}, statuses)
	})

	t.Run("NotFoundReturnsDefaultStatuses", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)
		setTSSURL(t, srv.URL)

		statuses, err := GetTestsQuarantineStatus(t.Context(), projectID, bvName, taskName, "", []string{"test_a"})
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
		DisplayTaskName       string   `json:"display_task_name"`
		TestNames             []string `json:"test_names"`
		IsManuallyQuarantined bool     `json:"is_manually_quarantined"`
		Metadata              struct {
			InitiatedBy         string   `json:"initiated_by"`
			TaskInitiatedFrom   string   `json:"task_initiated_from"`
			JiraAssignTo        string   `json:"jira_assign_to"`
			JiraTicketsToLinkTo []string `json:"jira_tickets_to_link_to"`
			Justification       string   `json:"justification"`
		} `json:"metadata"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	t.Cleanup(srv.Close)
	setTSSURLForTest(t, srv.URL)

	metadata := QuarantineMetadata{
		InitiatedBy:               "me",
		TaskInitiatedFrom:         "https://spruce/task/t1/0",
		JiraAssignTo:              "assigned_to_jira_team_devprod",
		ExistingJiraTicketsToLink: []string{"BF-123"},
		Justification:             "flaky",
	}
	require.NoError(t, SetTestQuarantined(t.Context(), "my/project", "ubuntu/2204", "my/task", "my/display/task", "test/name", true, metadata))
	assert.Equal(t, "/api/test_selection/transition_tests/", capturedPath)
	assert.Equal(t, "my/project", capturedBody.ProjectID)
	assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
	assert.Equal(t, "my/task", capturedBody.TaskName)
	assert.Equal(t, "my/display/task", capturedBody.DisplayTaskName)
	assert.Equal(t, []string{"test/name"}, capturedBody.TestNames)
	assert.True(t, capturedBody.IsManuallyQuarantined)
	assert.Equal(t, "me", capturedBody.Metadata.InitiatedBy)
	assert.Equal(t, "https://spruce/task/t1/0", capturedBody.Metadata.TaskInitiatedFrom)
	assert.Equal(t, "assigned_to_jira_team_devprod", capturedBody.Metadata.JiraAssignTo)
	assert.Equal(t, []string{"BF-123"}, capturedBody.Metadata.JiraTicketsToLinkTo)
	assert.Equal(t, "flaky", capturedBody.Metadata.Justification)
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
			DisplayTaskName       string `json:"display_task_name"`
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

		require.NoError(t, SetTaskQuarantined(t.Context(), "my/project", "ubuntu/2204", "my/task", "my/display/task", true, QuarantineMetadata{}))
		assert.Equal(t, "/api/test_selection/transition_task/", capturedPath)
		assert.Equal(t, "my/project", capturedBody.ProjectID)
		assert.Equal(t, "ubuntu/2204", capturedBody.BuildVariantName)
		assert.Equal(t, "my/task", capturedBody.TaskName)
		assert.Equal(t, "my/display/task", capturedBody.DisplayTaskName)
		assert.True(t, capturedBody.IsManuallyQuarantined)
	})

	t.Run("ServiceErrorIncludesBody", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		err := SetTaskQuarantined(t.Context(), projectID, bvName, taskName, "", true, QuarantineMetadata{})
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

		require.NoError(t, SetVariantQuarantined(t.Context(), "my/project", "ubuntu/2204", false, QuarantineMetadata{}))
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
	ctx := t.Context()
	env := testutil.NewEnvironment(ctx, t)
	taskOutput := task.TaskOutput{
		TestResults: task.TestResultOutput{Version: task.TestResultServiceEvergreen},
	}

	setup := func(t *testing.T, tasks ...task.Task) {
		require.NoError(t, db.ClearCollections(task.Collection))
		require.NoError(t, task.ClearTestResults(ctx, env))
		t.Cleanup(func() {
			assert.NoError(t, db.ClearCollections(task.Collection))
			assert.NoError(t, task.ClearTestResults(context.Background(), env))
		})
		for _, tsk := range tasks {
			require.NoError(t, tsk.Insert(ctx))
		}
	}
	recordCheckedTests := func(t *testing.T, info testresult.TestResultsInfo, checkedTests ...string) {
		record := testresult.DbTaskTestResults{ID: info.ID(), Info: info}
		require.NoError(t, task.NewTestResultService(env).AppendTestSelectionResults(ctx, record, checkedTests, nil))
	}
	// statusServer returns a TSS server that resolves quarantine state from a
	// per-task-name map and records the tests it was asked about.
	statusServer := func(t *testing.T, statesByTaskName map[string]map[string]string) *[]string {
		var (
			mu        sync.Mutex
			requested []string
		)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			var request struct {
				TaskName  string   `json:"task_name"`
				TestNames []string `json:"test_names"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			mu.Lock()
			requested = append(requested, request.TestNames...)
			mu.Unlock()
			body := map[string]map[string]any{}
			for testName, state := range statesByTaskName[request.TaskName] {
				body[testName] = map[string]any{"state": state}
			}
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)
		return &requested
	}

	execTask := task.Task{
		Id:             "exec_task",
		Project:        "p",
		BuildVariant:   "bv",
		DisplayName:    "exec_task",
		HasTestResults: true,
		TaskOutputInfo: &taskOutput,
	}
	execTaskInfo := testresult.TestResultsInfo{
		Project:  "p",
		Variant:  "bv",
		TaskID:   "exec_task",
		TaskName: "exec_task",
	}

	t.Run("NoOpWhenResultsEmpty", func(t *testing.T) {
		setup(t)
		require.NoError(t, DecorateQuarantineStatus(ctx, env, &execTask, nil))
	})

	t.Run("OnlyTestsCheckedByTestSelectionCanBeQuarantined", func(t *testing.T) {
		setup(t, execTask)
		recordCheckedTests(t, execTaskInfo, "TestFoo", "TestBar")
		requested := statusServer(t, map[string]map[string]string{
			"exec_task": {"TestFoo": "manually_quarantined", "TestBar": "stable"},
		})

		results := []testresult.TestResult{
			{TaskID: "exec_task", TestName: "TestFoo"},
			{TaskID: "exec_task", TestName: "TestBar"},
			{TaskID: "exec_task", TestName: "job0_fixture_setup_0"},
		}
		tsk := execTask
		require.NoError(t, DecorateQuarantineStatus(ctx, env, &tsk, results))
		assert.True(t, results[0].CanQuarantine)
		assert.True(t, results[0].IsManuallyQuarantined)
		assert.True(t, results[1].CanQuarantine)
		assert.False(t, results[1].IsManuallyQuarantined)
		assert.False(t, results[2].CanQuarantine)
		assert.False(t, results[2].IsManuallyQuarantined)
		assert.ElementsMatch(t, []string{"TestFoo", "TestBar"}, *requested, "TSS should only be asked about checked tests")
	})

	t.Run("ChecksDisplayTestNames", func(t *testing.T) {
		setup(t, execTask)
		recordCheckedTests(t, execTaskInfo, "jstests/core/foo.js")
		statusServer(t, map[string]map[string]string{
			"exec_task": {"jstests/core/foo.js": "manually_quarantined"},
		})

		results := []testresult.TestResult{{TaskID: "exec_task", TestName: "foo_id", DisplayTestName: "jstests/core/foo.js"}}
		tsk := execTask
		require.NoError(t, DecorateQuarantineStatus(ctx, env, &tsk, results))
		assert.True(t, results[0].CanQuarantine)
		assert.True(t, results[0].IsManuallyQuarantined)
	})

	t.Run("OnlyUsesCheckedTestsFromTheSameExecution", func(t *testing.T) {
		restarted := execTask
		restarted.Execution = 1
		setup(t, restarted)
		recordCheckedTests(t, execTaskInfo, "TestFoo")
		requested := statusServer(t, map[string]map[string]string{
			"exec_task": {"TestFoo": "manually_quarantined"},
		})

		results := []testresult.TestResult{{TaskID: "exec_task", Execution: 1, TestName: "TestFoo"}}
		require.NoError(t, DecorateQuarantineStatus(ctx, env, &restarted, results))
		assert.False(t, results[0].CanQuarantine, "test selection only checked the test in execution 0")
		assert.False(t, results[0].IsManuallyQuarantined)
		assert.Empty(t, *requested)
	})

	t.Run("TaskRunWithoutCheckedTestsSkipsTSS", func(t *testing.T) {
		setup(t, execTask)
		requested := statusServer(t, map[string]map[string]string{
			"exec_task": {"TestFoo": "manually_quarantined"},
		})

		results := []testresult.TestResult{{TaskID: "exec_task", TestName: "TestFoo"}}
		tsk := execTask
		require.NoError(t, DecorateQuarantineStatus(ctx, env, &tsk, results))
		assert.False(t, results[0].CanQuarantine)
		assert.False(t, results[0].IsManuallyQuarantined)
		assert.Empty(t, *requested)
	})

	t.Run("TSSFailureStillAllowsQuarantiningCheckedTests", func(t *testing.T) {
		setup(t, execTask)
		recordCheckedTests(t, execTaskInfo, "TestFoo")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		results := []testresult.TestResult{{TaskID: "exec_task", TestName: "TestFoo"}}
		tsk := execTask
		require.NoError(t, DecorateQuarantineStatus(ctx, env, &tsk, results))
		assert.True(t, results[0].CanQuarantine)
		assert.False(t, results[0].IsManuallyQuarantined)
	})

	t.Run("DisplayTaskUsesEachExecutionTaskRun", func(t *testing.T) {
		execA := task.Task{Id: "exec_a", DisplayName: "task_a", HasTestResults: true, TaskOutputInfo: &taskOutput}
		execB := task.Task{Id: "exec_b", DisplayName: "task_b", HasTestResults: true, TaskOutputInfo: &taskOutput}
		setup(t, execA, execB)
		recordCheckedTests(t, testresult.TestResultsInfo{Project: "p", Variant: "bv", TaskID: "exec_a", TaskName: "task_a", DisplayTaskName: "display_task"}, "TestA1")
		recordCheckedTests(t, testresult.TestResultsInfo{Project: "p", Variant: "bv", TaskID: "exec_b", TaskName: "task_b", DisplayTaskName: "display_task"}, "TestB2")
		statusServer(t, map[string]map[string]string{
			"task_a": {"TestA1": "manually_quarantined"},
			"task_b": {"TestB1": "manually_quarantined", "TestB2": "manually_quarantined"},
		})

		display := &task.Task{
			Id:             "display_task",
			DisplayName:    "display_task",
			DisplayOnly:    true,
			ExecutionTasks: []string{"exec_a", "exec_b"},
		}
		results := []testresult.TestResult{
			{TaskID: "exec_a", TestName: "TestA1"},
			{TaskID: "exec_b", TestName: "TestB1"},
			{TaskID: "exec_b", TestName: "TestB2"},
		}
		require.NoError(t, DecorateQuarantineStatus(ctx, env, display, results))
		assert.True(t, results[0].CanQuarantine)
		assert.True(t, results[0].IsManuallyQuarantined)
		assert.False(t, results[1].CanQuarantine, "TestB1 was not checked by test selection in its task run")
		assert.False(t, results[1].IsManuallyQuarantined)
		assert.True(t, results[2].CanQuarantine)
		assert.True(t, results[2].IsManuallyQuarantined)
	})
}

func TestRecordTestSelection(t *testing.T) {
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
		require.NoError(t, RecordTestSelection(ctx, env, req, []string{"test0"}))

		record := findRecord(t)
		assert.Equal(t, record.Info.ID(), record.ID)
		assert.Equal(t, projectID, record.Info.Project)
		assert.Equal(t, taskName, record.Info.TaskName)
		assert.Equal(t, 1, record.QuarantinedTestsCount)
		assert.Equal(t, []testresult.QuarantinedTest{{TestName: "test1"}}, record.QuarantinedTests)
		assert.ElementsMatch(t, []string{"test0", "test1", "test2"}, record.TestSelectionCheckedTests)
		assert.Equal(t, 1, findTask(t).NumQuarantinedTestsSkipped)
	})

	t.Run("AllTestsSelectedSkipsStatusCheckAndRecordsOnlyCheckedTests", func(t *testing.T) {
		setupTask(t)
		hits := newTSSServer(t, nil, nil)

		req := baseReq
		req.Tests = []string{"test0", "test1"}
		require.NoError(t, RecordTestSelection(ctx, env, req, []string{"test0", "test1"}))

		assert.Zero(t, *hits, "no status call should be made when no tests were skipped")
		record := findRecord(t)
		assert.ElementsMatch(t, []string{"test0", "test1"}, record.TestSelectionCheckedTests)
		assert.Empty(t, record.QuarantinedTests)
		assert.Zero(t, findTask(t).NumQuarantinedTestsSkipped)
	})

	t.Run("RepeatedRequestsAppendCheckedTestsOnce", func(t *testing.T) {
		setupTask(t)
		newTSSServer(t, nil, nil)

		req := baseReq
		req.Tests = []string{"test0", "test1"}
		require.NoError(t, RecordTestSelection(ctx, env, req, req.Tests))
		req.Tests = []string{"test1", "test2"}
		require.NoError(t, RecordTestSelection(ctx, env, req, req.Tests))

		assert.Equal(t, int64(1), countRecords(t))
		assert.ElementsMatch(t, []string{"test0", "test1", "test2"}, findRecord(t).TestSelectionCheckedTests)
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

		require.NoError(t, RecordTestSelection(ctx, env, baseReq, []string{"test0", "test3"}))

		record := findRecord(t)
		assert.Equal(t, 2, record.QuarantinedTestsCount)
		assert.Equal(t, []testresult.QuarantinedTest{{TestName: "test1"}, {TestName: "test2"}}, record.QuarantinedTests, "quarantined tests that were still selected should not be recorded")
		assert.ElementsMatch(t, []string{"test0", "test3", "test1", "test2"}, record.TestSelectionCheckedTests, "the known tests that were selected or skipped should be recorded as checked")
		assert.Equal(t, 2, findTask(t).NumQuarantinedTestsSkipped)
	})

	t.Run("KnownTestsPathRecordsSelectedTestsWhenVariantStateFails", func(t *testing.T) {
		setupTask(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		setTSSURLForTest(t, srv.URL)

		assert.Error(t, RecordTestSelection(ctx, env, baseReq, []string{"test0", "test3"}))

		record := findRecord(t)
		assert.ElementsMatch(t, []string{"test0", "test3"}, record.TestSelectionCheckedTests)
		assert.Empty(t, record.QuarantinedTests)
	})

	t.Run("FullyQuarantinedTaskRecordsAndReturnsSnapshot", func(t *testing.T) {
		setupTask(t)
		newTSSServer(t, map[string]map[string]any{
			"test0": {"state": "manually_quarantined"},
			"test1": {"state": "manually_quarantined"},
		}, nil)

		req := baseReq
		req.Tests = []string{"test0", "test1"}
		require.NoError(t, RecordTestSelection(ctx, env, req, nil))

		dbTask := findTask(t)
		assert.Equal(t, 2, dbTask.NumQuarantinedTestsSkipped)
		samples, err := task.GetQuarantinedTestSamples(ctx, env, []task.Task{*dbTask}, 10)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, 2, samples[0].QuarantinedTestsSkippedCount)
		assert.Len(t, samples[0].QuarantinedTests, 2)
	})
}
