// Copyright 2020 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestActionsService_ListWorkflowRunsByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/29679449/runs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":4,"workflow_runs":[{"id":399444496,"run_number":296,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"},{"id":399444497,"run_number":296,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"}]}`)
	})

	opts := &ListWorkflowRunsOptions{ListOptions: ListOptions{Page: 2, PerPage: 2}}
	ctx := context.Background()
	runs, _, err := client.Actions.ListWorkflowRunsByID(ctx, "o", "r", 29679449, opts)
	if err != nil {
		t.Errorf("Actions.ListWorkFlowRunsByID returned error: %v", err)
	}

	want := &WorkflowRuns{
		TotalCount: Int(4),
		WorkflowRuns: []*WorkflowRun{
			{ID: Int64(399444496), RunNumber: Int(296), CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)}},
			{ID: Int64(399444497), RunNumber: Int(296), CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)}},
		},
	}
	if !reflect.DeepEqual(runs, want) {
		t.Errorf("Actions.ListWorkflowRunsByID returned %+v, want %+v", runs, want)
	}

	const methodName = "ListWorkflowRunsByID"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListWorkflowRunsByID(ctx, "\n", "\n", 29679449, opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListWorkflowRunsByID(ctx, "o", "r", 29679449, opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_ListWorkflowRunsFileName(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/29679449/runs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":4,"workflow_runs":[{"id":399444496,"run_number":296,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"},{"id":399444497,"run_number":296,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"}]}`)
	})

	opts := &ListWorkflowRunsOptions{ListOptions: ListOptions{Page: 2, PerPage: 2}}
	ctx := context.Background()
	runs, _, err := client.Actions.ListWorkflowRunsByFileName(ctx, "o", "r", "29679449", opts)
	if err != nil {
		t.Errorf("Actions.ListWorkFlowRunsByFileName returned error: %v", err)
	}

	want := &WorkflowRuns{
		TotalCount: Int(4),
		WorkflowRuns: []*WorkflowRun{
			{ID: Int64(399444496), RunNumber: Int(296), CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)}},
			{ID: Int64(399444497), RunNumber: Int(296), CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)}},
		},
	}
	if !reflect.DeepEqual(runs, want) {
		t.Errorf("Actions.ListWorkflowRunsByFileName returned %+v, want %+v", runs, want)
	}

	const methodName = "ListWorkflowRunsByFileName"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListWorkflowRunsByFileName(ctx, "\n", "\n", "29679449", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListWorkflowRunsByFileName(ctx, "o", "r", "29679449", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetWorkflowRunByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/29679449", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":399444496,"run_number":296,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"}}`)
	})

	ctx := context.Background()
	runs, _, err := client.Actions.GetWorkflowRunByID(ctx, "o", "r", 29679449)
	if err != nil {
		t.Errorf("Actions.GetWorkflowRunByID returned error: %v", err)
	}

	want := &WorkflowRun{
		ID:        Int64(399444496),
		RunNumber: Int(296),
		CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)},
		UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
	}

	if !reflect.DeepEqual(runs, want) {
		t.Errorf("Actions.GetWorkflowRunByID returned %+v, want %+v", runs, want)
	}

	const methodName = "GetWorkflowRunByID"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowRunByID(ctx, "\n", "\n", 29679449)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetWorkflowRunByID(ctx, "o", "r", 29679449)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_RerunWorkflowRunByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/3434/rerun", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		w.WriteHeader(http.StatusCreated)
	})

	ctx := context.Background()
	resp, err := client.Actions.RerunWorkflowByID(ctx, "o", "r", 3434)
	if err != nil {
		t.Errorf("Actions.RerunWorkflowByID returned error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Actions.RerunWorkflowRunByID returned status: %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	const methodName = "RerunWorkflowByID"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.RerunWorkflowByID(ctx, "\n", "\n", 3434)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.RerunWorkflowByID(ctx, "o", "r", 3434)
	})
}

func TestActionsService_CancelWorkflowRunByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/3434/cancel", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		w.WriteHeader(http.StatusAccepted)
	})

	ctx := context.Background()
	resp, err := client.Actions.CancelWorkflowRunByID(ctx, "o", "r", 3434)
	if _, ok := err.(*AcceptedError); !ok {
		t.Errorf("Actions.CancelWorkflowRunByID returned error: %v (want AcceptedError)", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("Actions.CancelWorkflowRunByID returned status: %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	const methodName = "CancelWorkflowRunByID"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.CancelWorkflowRunByID(ctx, "\n", "\n", 3434)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.CancelWorkflowRunByID(ctx, "o", "r", 3434)
	})
}

func TestActionsService_GetWorkflowRunLogs(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/399444496/logs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		http.Redirect(w, r, "http://github.com/a", http.StatusFound)
	})

	ctx := context.Background()
	url, resp, err := client.Actions.GetWorkflowRunLogs(ctx, "o", "r", 399444496, true)
	if err != nil {
		t.Errorf("Actions.GetWorkflowRunLogs returned error: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("Actions.GetWorkflowRunLogs returned status: %d, want %d", resp.StatusCode, http.StatusFound)
	}
	want := "http://github.com/a"
	if url.String() != want {
		t.Errorf("Actions.GetWorkflowRunLogs returned %+v, want %+v", url.String(), want)
	}

	const methodName = "GetWorkflowRunLogs"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowRunLogs(ctx, "\n", "\n", 399444496, true)
		return err
	})
}

func TestActionsService_GetWorkflowRunLogs_StatusMovedPermanently_dontFollowRedirects(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/399444496/logs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		http.Redirect(w, r, "http://github.com/a", http.StatusMovedPermanently)
	})

	ctx := context.Background()
	_, resp, _ := client.Actions.GetWorkflowRunLogs(ctx, "o", "r", 399444496, false)
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("Actions.GetWorkflowJobLogs returned status: %d, want %d", resp.StatusCode, http.StatusMovedPermanently)
	}
}

func TestActionsService_GetWorkflowRunLogs_StatusMovedPermanently_followRedirects(t *testing.T) {
	client, mux, serverURL, teardown := setup()
	defer teardown()

	// Mock a redirect link, which leads to an archive link
	mux.HandleFunc("/repos/o/r/actions/runs/399444496/logs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		redirectURL, _ := url.Parse(serverURL + baseURLPath + "/redirect")
		http.Redirect(w, r, redirectURL.String(), http.StatusMovedPermanently)
	})

	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		http.Redirect(w, r, "http://github.com/a", http.StatusFound)
	})

	ctx := context.Background()
	url, resp, err := client.Actions.GetWorkflowRunLogs(ctx, "o", "r", 399444496, true)
	if err != nil {
		t.Errorf("Actions.GetWorkflowJobLogs returned error: %v", err)
	}

	if resp.StatusCode != http.StatusFound {
		t.Errorf("Actions.GetWorkflowJobLogs returned status: %d, want %d", resp.StatusCode, http.StatusFound)
	}

	want := "http://github.com/a"
	if url.String() != want {
		t.Errorf("Actions.GetWorkflowJobLogs returned %+v, want %+v", url.String(), want)
	}

	const methodName = "GetWorkflowRunLogs"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowRunLogs(ctx, "\n", "\n", 399444496, true)
		return err
	})
}

func TestActionService_ListRepositoryWorkflowRuns(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":2,
		"workflow_runs":[
			{"id":298499444,"run_number":301,"created_at":"2020-04-11T11:14:54Z","updated_at":"2020-04-11T11:14:54Z"},
			{"id":298499445,"run_number":302,"created_at":"2020-04-11T11:14:54Z","updated_at":"2020-04-11T11:14:54Z"}]}`)

	})

	opts := &ListWorkflowRunsOptions{ListOptions: ListOptions{Page: 2, PerPage: 2}}
	ctx := context.Background()
	runs, _, err := client.Actions.ListRepositoryWorkflowRuns(ctx, "o", "r", opts)

	if err != nil {
		t.Errorf("Actions.ListRepositoryWorkflowRuns returned error: %v", err)
	}

	expected := &WorkflowRuns{
		TotalCount: Int(2),
		WorkflowRuns: []*WorkflowRun{
			{ID: Int64(298499444), RunNumber: Int(301), CreatedAt: &Timestamp{time.Date(2020, time.April, 11, 11, 14, 54, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.April, 11, 11, 14, 54, 0, time.UTC)}},
			{ID: Int64(298499445), RunNumber: Int(302), CreatedAt: &Timestamp{time.Date(2020, time.April, 11, 11, 14, 54, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.April, 11, 11, 14, 54, 0, time.UTC)}},
		},
	}

	if !reflect.DeepEqual(runs, expected) {
		t.Errorf("Actions.ListRepositoryWorkflowRuns returned %+v, want %+v", runs, expected)
	}

	const methodName = "ListRepositoryWorkflowRuns"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListRepositoryWorkflowRuns(ctx, "\n", "\n", opts)

		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListRepositoryWorkflowRuns(ctx, "o", "r", opts)

		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionService_DeleteWorkflowRunLogs(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/399444496/logs", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")

		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	if _, err := client.Actions.DeleteWorkflowRunLogs(ctx, "o", "r", 399444496); err != nil {
		t.Errorf("DeleteWorkflowRunLogs returned error: %v", err)
	}

	const methodName = "DeleteWorkflowRunLogs"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.DeleteWorkflowRunLogs(ctx, "\n", "\n", 399444496)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.DeleteWorkflowRunLogs(ctx, "o", "r", 399444496)
	})
}

func TestActionsService_GetWorkflowRunUsageByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runs/29679449/timing", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"billable":{"UBUNTU":{"total_ms":180000,"jobs":1},"MACOS":{"total_ms":240000,"jobs":4},"WINDOWS":{"total_ms":300000,"jobs":2}},"run_duration_ms":500000}`)
	})

	ctx := context.Background()
	workflowRunUsage, _, err := client.Actions.GetWorkflowRunUsageByID(ctx, "o", "r", 29679449)
	if err != nil {
		t.Errorf("Actions.GetWorkflowRunUsageByID returned error: %v", err)
	}

	want := &WorkflowRunUsage{
		Billable: &WorkflowRunEnvironment{
			Ubuntu: &WorkflowRunBill{
				TotalMS: Int64(180000),
				Jobs:    Int(1),
			},
			MacOS: &WorkflowRunBill{
				TotalMS: Int64(240000),
				Jobs:    Int(4),
			},
			Windows: &WorkflowRunBill{
				TotalMS: Int64(300000),
				Jobs:    Int(2),
			},
		},
		RunDurationMS: Int64(500000),
	}

	if !reflect.DeepEqual(workflowRunUsage, want) {
		t.Errorf("Actions.GetWorkflowRunUsageByID returned %+v, want %+v", workflowRunUsage, want)
	}

	const methodName = "GetWorkflowRunUsageByID"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowRunUsageByID(ctx, "\n", "\n", 29679449)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetWorkflowRunUsageByID(ctx, "o", "r", 29679449)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}
