// Copyright 2018 The go-github AUTHORS. All rights reserved.
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

func TestRepositoriesService_ListPreReceiveHooks(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pre-receive-hooks", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", mediaTypePreReceiveHooksPreview)
		testFormValues(t, r, values{"page": "2"})
		fmt.Fprint(w, `[{"id":1}, {"id":2}]`)
	})

	opt := &ListOptions{Page: 2}

	ctx := context.Background()
	hooks, _, err := client.Repositories.ListPreReceiveHooks(ctx, "o", "r", opt)
	if err != nil {
		t.Errorf("Repositories.ListHooks returned error: %v", err)
	}

	want := []*PreReceiveHook{{ID: Int64(1)}, {ID: Int64(2)}}
	if !reflect.DeepEqual(hooks, want) {
		t.Errorf("Repositories.ListPreReceiveHooks returned %+v, want %+v", hooks, want)
	}

	const methodName = "ListPreReceiveHooks"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListPreReceiveHooks(ctx, "\n", "\n", opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListPreReceiveHooks(ctx, "o", "r", opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListPreReceiveHooks_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Repositories.ListPreReceiveHooks(ctx, "%", "%", nil)
	testURLParseError(t, err)
}

func TestRepositoriesService_GetPreReceiveHook(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pre-receive-hooks/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", mediaTypePreReceiveHooksPreview)
		fmt.Fprint(w, `{"id":1}`)
	})

	ctx := context.Background()
	hook, _, err := client.Repositories.GetPreReceiveHook(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("Repositories.GetPreReceiveHook returned error: %v", err)
	}

	want := &PreReceiveHook{ID: Int64(1)}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Repositories.GetPreReceiveHook returned %+v, want %+v", hook, want)
	}

	const methodName = "GetPreReceiveHook"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetPreReceiveHook(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetPreReceiveHook(ctx, "o", "r", 1)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_GetPreReceiveHook_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Repositories.GetPreReceiveHook(ctx, "%", "%", 1)
	testURLParseError(t, err)
}

func TestRepositoriesService_UpdatePreReceiveHook(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &PreReceiveHook{}

	mux.HandleFunc("/repos/o/r/pre-receive-hooks/1", func(w http.ResponseWriter, r *http.Request) {
		v := new(PreReceiveHook)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PATCH")
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"id":1}`)
	})

	ctx := context.Background()
	hook, _, err := client.Repositories.UpdatePreReceiveHook(ctx, "o", "r", 1, input)
	if err != nil {
		t.Errorf("Repositories.UpdatePreReceiveHook returned error: %v", err)
	}

	want := &PreReceiveHook{ID: Int64(1)}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Repositories.UpdatePreReceiveHook returned %+v, want %+v", hook, want)
	}

	const methodName = "UpdatePreReceiveHook"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.UpdatePreReceiveHook(ctx, "\n", "\n", -1, input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.UpdatePreReceiveHook(ctx, "o", "r", 1, input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_PreReceiveHook_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Repositories.UpdatePreReceiveHook(ctx, "%", "%", 1, nil)
	testURLParseError(t, err)
}

func TestRepositoriesService_DeletePreReceiveHook(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pre-receive-hooks/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Repositories.DeletePreReceiveHook(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("Repositories.DeletePreReceiveHook returned error: %v", err)
	}

	const methodName = "DeletePreReceiveHook"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Repositories.DeletePreReceiveHook(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Repositories.DeletePreReceiveHook(ctx, "o", "r", 1)
	})
}

func TestRepositoriesService_DeletePreReceiveHook_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, err := client.Repositories.DeletePreReceiveHook(ctx, "%", "%", 1)
	testURLParseError(t, err)
}
