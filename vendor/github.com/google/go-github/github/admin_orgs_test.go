// Copyright 2019 The go-github AUTHORS. All rights reserved.
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

func TestAdminOrgs_Create(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &Organization{
		Login: String("github"),
	}

	mux.HandleFunc("/admin/organizations", func(w http.ResponseWriter, r *http.Request) {
		v := new(createOrgRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		want := &createOrgRequest{Login: String("github"), Admin: String("ghAdmin")}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Request body = %+v, want %+v", v, want)
		}

		fmt.Fprint(w, `{"login":"github","id":1}`)
	})

	ctx := context.Background()
	org, _, err := client.Admin.CreateOrg(ctx, input, "ghAdmin")
	if err != nil {
		t.Errorf("Admin.CreateOrg returned error: %v", err)
	}

	want := &Organization{ID: Int64(1), Login: String("github")}
	if !reflect.DeepEqual(org, want) {
		t.Errorf("Admin.CreateOrg returned %+v, want %+v", org, want)
	}

	const methodName = "CreateOrg"
	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Admin.CreateOrg(ctx, input, "ghAdmin")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAdminOrgs_Rename(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &Organization{
		Login: String("o"),
	}

	mux.HandleFunc("/admin/organizations/o", func(w http.ResponseWriter, r *http.Request) {
		v := new(renameOrgRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PATCH")
		want := &renameOrgRequest{Login: String("the-new-octocats")}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Request body = %+v, want %+v", v, want)
		}

		fmt.Fprint(w, `{"message":"Job queued to rename organization. It may take a few minutes to complete.","url":"https://<hostname>/api/v3/organizations/1"}`)
	})

	ctx := context.Background()
	resp, _, err := client.Admin.RenameOrg(ctx, input, "the-new-octocats")
	if err != nil {
		t.Errorf("Admin.RenameOrg returned error: %v", err)
	}

	want := &RenameOrgResponse{Message: String("Job queued to rename organization. It may take a few minutes to complete."), URL: String("https://<hostname>/api/v3/organizations/1")}
	if !reflect.DeepEqual(resp, want) {
		t.Errorf("Admin.RenameOrg returned %+v, want %+v", resp, want)
	}

	const methodName = "RenameOrg"
	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Admin.RenameOrg(ctx, input, "the-new-octocats")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAdminOrgs_RenameByName(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/admin/organizations/o", func(w http.ResponseWriter, r *http.Request) {
		v := new(renameOrgRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PATCH")
		want := &renameOrgRequest{Login: String("the-new-octocats")}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Request body = %+v, want %+v", v, want)
		}

		fmt.Fprint(w, `{"message":"Job queued to rename organization. It may take a few minutes to complete.","url":"https://<hostname>/api/v3/organizations/1"}`)
	})

	ctx := context.Background()
	resp, _, err := client.Admin.RenameOrgByName(ctx, "o", "the-new-octocats")
	if err != nil {
		t.Errorf("Admin.RenameOrg returned error: %v", err)
	}

	want := &RenameOrgResponse{Message: String("Job queued to rename organization. It may take a few minutes to complete."), URL: String("https://<hostname>/api/v3/organizations/1")}
	if !reflect.DeepEqual(resp, want) {
		t.Errorf("Admin.RenameOrg returned %+v, want %+v", resp, want)
	}

	const methodName = "RenameOrgByName"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Admin.RenameOrgByName(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Admin.RenameOrgByName(ctx, "o", "the-new-octocats")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}
