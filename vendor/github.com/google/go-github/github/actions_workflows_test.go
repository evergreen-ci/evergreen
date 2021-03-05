// Copyright 2020 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestActionsService_ListWorkflows(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":4,"workflows":[{"id":72844,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"},{"id":72845,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"}]}`)
	})

	opts := &ListOptions{Page: 2, PerPage: 2}
	ctx := context.Background()
	workflows, _, err := client.Actions.ListWorkflows(ctx, "o", "r", opts)
	if err != nil {
		t.Errorf("Actions.ListWorkflows returned error: %v", err)
	}

	want := &Workflows{
		TotalCount: Int(4),
		Workflows: []*Workflow{
			{ID: Int64(72844), CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)}},
			{ID: Int64(72845), CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)}, UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)}},
		},
	}
	if !reflect.DeepEqual(workflows, want) {
		t.Errorf("Actions.ListWorkflows returned %+v, want %+v", workflows, want)
	}

	const methodName = "ListWorkflows"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListWorkflows(ctx, "\n", "\n", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListWorkflows(ctx, "o", "r", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetWorkflowByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/72844", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":72844,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"}`)
	})

	ctx := context.Background()
	workflow, _, err := client.Actions.GetWorkflowByID(ctx, "o", "r", 72844)
	if err != nil {
		t.Errorf("Actions.GetWorkflowByID returned error: %v", err)
	}

	want := &Workflow{
		ID:        Int64(72844),
		CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)},
		UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
	}
	if !reflect.DeepEqual(workflow, want) {
		t.Errorf("Actions.GetWorkflowByID returned %+v, want %+v", workflow, want)
	}

	const methodName = "GetWorkflowByID"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowByID(ctx, "\n", "\n", -72844)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetWorkflowByID(ctx, "o", "r", 72844)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetWorkflowByFileName(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/main.yml", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":72844,"created_at":"2019-01-02T15:04:05Z","updated_at":"2020-01-02T15:04:05Z"}`)
	})

	ctx := context.Background()
	workflow, _, err := client.Actions.GetWorkflowByFileName(ctx, "o", "r", "main.yml")
	if err != nil {
		t.Errorf("Actions.GetWorkflowByFileName returned error: %v", err)
	}

	want := &Workflow{
		ID:        Int64(72844),
		CreatedAt: &Timestamp{time.Date(2019, time.January, 02, 15, 04, 05, 0, time.UTC)},
		UpdatedAt: &Timestamp{time.Date(2020, time.January, 02, 15, 04, 05, 0, time.UTC)},
	}
	if !reflect.DeepEqual(workflow, want) {
		t.Errorf("Actions.GetWorkflowByFileName returned %+v, want %+v", workflow, want)
	}

	const methodName = "GetWorkflowByFileName"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowByFileName(ctx, "\n", "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetWorkflowByFileName(ctx, "o", "r", "main.yml")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetWorkflowUsageByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/72844/timing", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"billable":{"UBUNTU":{"total_ms":180000},"MACOS":{"total_ms":240000},"WINDOWS":{"total_ms":300000}}}`)
	})

	ctx := context.Background()
	workflowUsage, _, err := client.Actions.GetWorkflowUsageByID(ctx, "o", "r", 72844)
	if err != nil {
		t.Errorf("Actions.GetWorkflowUsageByID returned error: %v", err)
	}

	want := &WorkflowUsage{
		Billable: &WorkflowEnvironment{
			Ubuntu: &WorkflowBill{
				TotalMS: Int64(180000),
			},
			MacOS: &WorkflowBill{
				TotalMS: Int64(240000),
			},
			Windows: &WorkflowBill{
				TotalMS: Int64(300000),
			},
		},
	}
	if !reflect.DeepEqual(workflowUsage, want) {
		t.Errorf("Actions.GetWorkflowUsageByID returned %+v, want %+v", workflowUsage, want)
	}

	const methodName = "GetWorkflowUsageByID"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowUsageByID(ctx, "\n", "\n", -72844)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetWorkflowUsageByID(ctx, "o", "r", 72844)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetWorkflowUsageByFileName(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/main.yml/timing", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"billable":{"UBUNTU":{"total_ms":180000},"MACOS":{"total_ms":240000},"WINDOWS":{"total_ms":300000}}}`)
	})

	ctx := context.Background()
	workflowUsage, _, err := client.Actions.GetWorkflowUsageByFileName(ctx, "o", "r", "main.yml")
	if err != nil {
		t.Errorf("Actions.GetWorkflowUsageByFileName returned error: %v", err)
	}

	want := &WorkflowUsage{
		Billable: &WorkflowEnvironment{
			Ubuntu: &WorkflowBill{
				TotalMS: Int64(180000),
			},
			MacOS: &WorkflowBill{
				TotalMS: Int64(240000),
			},
			Windows: &WorkflowBill{
				TotalMS: Int64(300000),
			},
		},
	}
	if !reflect.DeepEqual(workflowUsage, want) {
		t.Errorf("Actions.GetWorkflowUsageByFileName returned %+v, want %+v", workflowUsage, want)
	}

	const methodName = "GetWorkflowUsageByFileName"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetWorkflowUsageByFileName(ctx, "\n", "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetWorkflowUsageByFileName(ctx, "o", "r", "main.yml")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_CreateWorkflowDispatchEventByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	event := CreateWorkflowDispatchEventRequest{
		Ref: "d4cfb6e7",
		Inputs: map[string]interface{}{
			"key": "value",
		},
	}
	mux.HandleFunc("/repos/o/r/actions/workflows/72844/dispatches", func(w http.ResponseWriter, r *http.Request) {
		var v CreateWorkflowDispatchEventRequest
		json.NewDecoder(r.Body).Decode(&v)

		testMethod(t, r, "POST")
		if !reflect.DeepEqual(v, event) {
			t.Errorf("Request body = %+v, want %+v", v, event)
		}
	})

	ctx := context.Background()
	_, err := client.Actions.CreateWorkflowDispatchEventByID(ctx, "o", "r", 72844, event)
	if err != nil {
		t.Errorf("Actions.CreateWorkflowDispatchEventByID returned error: %v", err)
	}

	// Test s.client.NewRequest failure
	client.BaseURL.Path = ""
	_, err = client.Actions.CreateWorkflowDispatchEventByID(ctx, "o", "r", 72844, event)
	if err == nil {
		t.Error("client.BaseURL.Path='' CreateWorkflowDispatchEventByID err = nil, want error")
	}

	const methodName = "CreateWorkflowDispatchEventByID"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.CreateWorkflowDispatchEventByID(ctx, "o", "r", 72844, event)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.CreateWorkflowDispatchEventByID(ctx, "o", "r", 72844, event)
	})
}

func TestActionsService_CreateWorkflowDispatchEventByFileName(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	event := CreateWorkflowDispatchEventRequest{
		Ref: "d4cfb6e7",
		Inputs: map[string]interface{}{
			"key": "value",
		},
	}
	mux.HandleFunc("/repos/o/r/actions/workflows/main.yml/dispatches", func(w http.ResponseWriter, r *http.Request) {
		var v CreateWorkflowDispatchEventRequest
		json.NewDecoder(r.Body).Decode(&v)

		testMethod(t, r, "POST")
		if !reflect.DeepEqual(v, event) {
			t.Errorf("Request body = %+v, want %+v", v, event)
		}
	})

	ctx := context.Background()
	_, err := client.Actions.CreateWorkflowDispatchEventByFileName(ctx, "o", "r", "main.yml", event)
	if err != nil {
		t.Errorf("Actions.CreateWorkflowDispatchEventByFileName returned error: %v", err)
	}

	// Test s.client.NewRequest failure
	client.BaseURL.Path = ""
	_, err = client.Actions.CreateWorkflowDispatchEventByFileName(ctx, "o", "r", "main.yml", event)
	if err == nil {
		t.Error("client.BaseURL.Path='' CreateWorkflowDispatchEventByFileName err = nil, want error")
	}

	const methodName = "CreateWorkflowDispatchEventByFileName"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.CreateWorkflowDispatchEventByFileName(ctx, "o", "r", "main.yml", event)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.CreateWorkflowDispatchEventByFileName(ctx, "o", "r", "main.yml", event)
	})
}

func TestActionsService_EnableWorkflowByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/72844/enable", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		if r.Body != http.NoBody {
			t.Errorf("Request body = %+v, want %+v", r.Body, http.NoBody)
		}
	})

	ctx := context.Background()
	_, err := client.Actions.EnableWorkflowByID(ctx, "o", "r", 72844)
	if err != nil {
		t.Errorf("Actions.EnableWorkflowByID returned error: %v", err)
	}

	// Test s.client.NewRequest failure
	client.BaseURL.Path = ""
	_, err = client.Actions.EnableWorkflowByID(ctx, "o", "r", 72844)
	if err == nil {
		t.Error("client.BaseURL.Path='' EnableWorkflowByID err = nil, want error")
	}

	const methodName = "EnableWorkflowByID"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.EnableWorkflowByID(ctx, "o", "r", 72844)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.EnableWorkflowByID(ctx, "o", "r", 72844)
	})
}

func TestActionsService_EnableWorkflowByFilename(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/main.yml/enable", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		if r.Body != http.NoBody {
			t.Errorf("Request body = %+v, want %+v", r.Body, http.NoBody)
		}
	})

	ctx := context.Background()
	_, err := client.Actions.EnableWorkflowByFileName(ctx, "o", "r", "main.yml")
	if err != nil {
		t.Errorf("Actions.EnableWorkflowByFilename returned error: %v", err)
	}

	// Test s.client.NewRequest failure
	client.BaseURL.Path = ""
	_, err = client.Actions.EnableWorkflowByFileName(ctx, "o", "r", "main.yml")
	if err == nil {
		t.Error("client.BaseURL.Path='' EnableWorkflowByFilename err = nil, want error")
	}

	const methodName = "EnableWorkflowByFileName"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.EnableWorkflowByFileName(ctx, "o", "r", "main.yml")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.EnableWorkflowByFileName(ctx, "o", "r", "main.yml")
	})
}

func TestActionsService_DisableWorkflowByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/72844/disable", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		if r.Body != http.NoBody {
			t.Errorf("Request body = %+v, want %+v", r.Body, http.NoBody)
		}
	})

	ctx := context.Background()
	_, err := client.Actions.DisableWorkflowByID(ctx, "o", "r", 72844)
	if err != nil {
		t.Errorf("Actions.DisableWorkflowByID returned error: %v", err)
	}

	// Test s.client.NewRequest failure
	client.BaseURL.Path = ""
	_, err = client.Actions.DisableWorkflowByID(ctx, "o", "r", 72844)
	if err == nil {
		t.Error("client.BaseURL.Path='' DisableWorkflowByID err = nil, want error")
	}

	const methodName = "DisableWorkflowByID"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.DisableWorkflowByID(ctx, "o", "r", 72844)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.DisableWorkflowByID(ctx, "o", "r", 72844)
	})
}

func TestActionsService_DisableWorkflowByFileName(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/workflows/main.yml/disable", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		if r.Body != http.NoBody {
			t.Errorf("Request body = %+v, want %+v", r.Body, http.NoBody)
		}
	})

	ctx := context.Background()
	_, err := client.Actions.DisableWorkflowByFileName(ctx, "o", "r", "main.yml")
	if err != nil {
		t.Errorf("Actions.DisableWorkflowByFileName returned error: %v", err)
	}

	// Test s.client.NewRequest failure
	client.BaseURL.Path = ""
	_, err = client.Actions.DisableWorkflowByFileName(ctx, "o", "r", "main.yml")
	if err == nil {
		t.Error("client.BaseURL.Path='' DisableWorkflowByFileName err = nil, want error")
	}

	const methodName = "DisableWorkflowByFileName"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.DisableWorkflowByFileName(ctx, "o", "r", "main.yml")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.DisableWorkflowByFileName(ctx, "o", "r", "main.yml")
	})
}
