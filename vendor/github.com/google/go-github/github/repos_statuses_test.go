// Copyright 2013 The go-github AUTHORS. All rights reserved.
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
)

func TestRepositoriesService_ListStatuses(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/commits/r/statuses", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"page": "2"})
		fmt.Fprint(w, `[{"id":1}]`)
	})

	opt := &ListOptions{Page: 2}
	ctx := context.Background()
	statuses, _, err := client.Repositories.ListStatuses(ctx, "o", "r", "r", opt)
	if err != nil {
		t.Errorf("Repositories.ListStatuses returned error: %v", err)
	}

	want := []*RepoStatus{{ID: Int64(1)}}
	if !reflect.DeepEqual(statuses, want) {
		t.Errorf("Repositories.ListStatuses returned %+v, want %+v", statuses, want)
	}

	const methodName = "ListStatuses"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListStatuses(ctx, "\n", "\n", "\n", opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListStatuses(ctx, "o", "r", "r", opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListStatuses_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Repositories.ListStatuses(ctx, "%", "r", "r", nil)
	testURLParseError(t, err)
}

func TestRepositoriesService_CreateStatus(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &RepoStatus{State: String("s"), TargetURL: String("t"), Description: String("d")}

	mux.HandleFunc("/repos/o/r/statuses/r", func(w http.ResponseWriter, r *http.Request) {
		v := new(RepoStatus)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}
		fmt.Fprint(w, `{"id":1}`)
	})

	ctx := context.Background()
	status, _, err := client.Repositories.CreateStatus(ctx, "o", "r", "r", input)
	if err != nil {
		t.Errorf("Repositories.CreateStatus returned error: %v", err)
	}

	want := &RepoStatus{ID: Int64(1)}
	if !reflect.DeepEqual(status, want) {
		t.Errorf("Repositories.CreateStatus returned %+v, want %+v", status, want)
	}

	const methodName = "CreateStatus"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.CreateStatus(ctx, "\n", "\n", "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.CreateStatus(ctx, "o", "r", "r", input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_CreateStatus_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Repositories.CreateStatus(ctx, "%", "r", "r", nil)
	testURLParseError(t, err)
}

func TestRepositoriesService_GetCombinedStatus(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/commits/r/status", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"page": "2"})
		fmt.Fprint(w, `{"state":"success", "statuses":[{"id":1}]}`)
	})

	opt := &ListOptions{Page: 2}
	ctx := context.Background()
	status, _, err := client.Repositories.GetCombinedStatus(ctx, "o", "r", "r", opt)
	if err != nil {
		t.Errorf("Repositories.GetCombinedStatus returned error: %v", err)
	}

	want := &CombinedStatus{State: String("success"), Statuses: []*RepoStatus{{ID: Int64(1)}}}
	if !reflect.DeepEqual(status, want) {
		t.Errorf("Repositories.GetCombinedStatus returned %+v, want %+v", status, want)
	}

	const methodName = "GetCombinedStatus"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetCombinedStatus(ctx, "\n", "\n", "\n", opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetCombinedStatus(ctx, "o", "r", "r", opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}
