// Copyright 2021 The go-github AUTHORS. All rights reserved.
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

func TestOrganizationsService_GetActionsPermissions(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/permissions", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"enabled_repositories": "all", "allowed_actions": "all"}`)
	})

	ctx := context.Background()
	org, _, err := client.Organizations.GetActionsPermissions(ctx, "o")
	if err != nil {
		t.Errorf("Organizations.GetActionsPermissions returned error: %v", err)
	}
	want := &ActionsPermissions{EnabledRepositories: String("all"), AllowedActions: String("all")}
	if !reflect.DeepEqual(org, want) {
		t.Errorf("Organizations.GetActionsPermissions returned %+v, want %+v", org, want)
	}

	const methodName = "GetActionsPermissions"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Organizations.GetActionsPermissions(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Organizations.GetActionsPermissions(ctx, "o")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestOrganizationsService_EditActionsPermissions(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &ActionsPermissions{EnabledRepositories: String("all"), AllowedActions: String("selected")}

	mux.HandleFunc("/orgs/o/actions/permissions", func(w http.ResponseWriter, r *http.Request) {
		v := new(ActionsPermissions)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PUT")
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"enabled_repositories": "all", "allowed_actions": "selected"}`)
	})

	ctx := context.Background()
	org, _, err := client.Organizations.EditActionsPermissions(ctx, "o", *input)
	if err != nil {
		t.Errorf("Organizations.EditActionsPermissions returned error: %v", err)
	}

	want := &ActionsPermissions{EnabledRepositories: String("all"), AllowedActions: String("selected")}
	if !reflect.DeepEqual(org, want) {
		t.Errorf("Organizations.EditActionsPermissions returned %+v, want %+v", org, want)
	}

	const methodName = "EditActionsPermissions"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Organizations.EditActionsPermissions(ctx, "\n", *input)
		return err
	})
}
