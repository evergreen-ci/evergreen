// Copyright 2021 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func TestActionsService_ListOrganizationRunnerGroups(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":3,"runner_groups":[{"id":1,"name":"Default","visibility":"all","default":true,"runners_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/1/runners","inherited":false,"allows_public_repositories":true},{"id":2,"name":"octo-runner-group","visibility":"selected","default":false,"selected_repositories_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories","runners_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners","inherited":true,"allows_public_repositories":true},{"id":3,"name":"expensive-hardware","visibility":"private","default":false,"runners_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/3/runners","inherited":false,"allows_public_repositories":true}]}`)
	})

	opts := &ListOptions{Page: 2, PerPage: 2}
	ctx := context.Background()
	groups, _, err := client.Actions.ListOrganizationRunnerGroups(ctx, "o", opts)
	if err != nil {
		t.Errorf("Actions.ListOrganizationRunnerGroups returned error: %v", err)
	}

	want := &RunnerGroups{
		TotalCount: 3,
		RunnerGroups: []*RunnerGroup{
			{ID: Int64(1), Name: String("Default"), Visibility: String("all"), Default: Bool(true), RunnersURL: String("https://api.github.com/orgs/octo-org/actions/runner_groups/1/runners"), Inherited: Bool(false), AllowsPublicRepositories: Bool(true)},
			{ID: Int64(2), Name: String("octo-runner-group"), Visibility: String("selected"), Default: Bool(false), SelectedRepositoriesURL: String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories"), RunnersURL: String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners"), Inherited: Bool(true), AllowsPublicRepositories: Bool(true)},
			{ID: Int64(3), Name: String("expensive-hardware"), Visibility: String("private"), Default: Bool(false), RunnersURL: String("https://api.github.com/orgs/octo-org/actions/runner_groups/3/runners"), Inherited: Bool(false), AllowsPublicRepositories: Bool(true)},
		},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("Actions.ListOrganizationRunnerGroups returned %+v, want %+v", groups, want)
	}

	const methodName = "ListOrganizationRunnerGroups"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListOrganizationRunnerGroups(ctx, "\n", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListOrganizationRunnerGroups(ctx, "o", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetOrganizationRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":2,"name":"octo-runner-group","visibility":"selected","default":false,"selected_repositories_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories","runners_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners","inherited":false,"allows_public_repositories":true}`)
	})

	ctx := context.Background()
	group, _, err := client.Actions.GetOrganizationRunnerGroup(ctx, "o", 2)
	if err != nil {
		t.Errorf("Actions.ListOrganizationRunnerGroups returned error: %v", err)
	}

	want := &RunnerGroup{
		ID:                       Int64(2),
		Name:                     String("octo-runner-group"),
		Visibility:               String("selected"),
		Default:                  Bool(false),
		SelectedRepositoriesURL:  String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories"),
		RunnersURL:               String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners"),
		Inherited:                Bool(false),
		AllowsPublicRepositories: Bool(true),
	}

	if !reflect.DeepEqual(group, want) {
		t.Errorf("Actions.GetOrganizationRunnerGroup returned %+v, want %+v", group, want)
	}

	const methodName = "GetOrganizationRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetOrganizationRunnerGroup(ctx, "\n", 2)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetOrganizationRunnerGroup(ctx, "o", 2)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_DeleteOrganizationRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Actions.DeleteOrganizationRunnerGroup(ctx, "o", 2)
	if err != nil {
		t.Errorf("Actions.DeleteOrganizationRunnerGroup returned error: %v", err)
	}

	const methodName = "DeleteOrganizationRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.DeleteOrganizationRunnerGroup(ctx, "\n", 2)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.DeleteOrganizationRunnerGroup(ctx, "o", 2)
	})
}

func TestActionsService_CreateOrganizationRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"id":2,"name":"octo-runner-group","visibility":"selected","default":false,"selected_repositories_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories","runners_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners","inherited":false,"allows_public_repositories":true}`)
	})

	ctx := context.Background()
	req := CreateRunnerGroupRequest{
		Name:       String("octo-runner-group"),
		Visibility: String("selected"),
	}
	group, _, err := client.Actions.CreateOrganizationRunnerGroup(ctx, "o", req)
	if err != nil {
		t.Errorf("Actions.CreateOrganizationRunnerGroup returned error: %v", err)
	}

	want := &RunnerGroup{
		ID:                       Int64(2),
		Name:                     String("octo-runner-group"),
		Visibility:               String("selected"),
		Default:                  Bool(false),
		SelectedRepositoriesURL:  String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories"),
		RunnersURL:               String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners"),
		Inherited:                Bool(false),
		AllowsPublicRepositories: Bool(true),
	}

	if !reflect.DeepEqual(group, want) {
		t.Errorf("Actions.CreateOrganizationRunnerGroup returned %+v, want %+v", group, want)
	}

	const methodName = "CreateOrganizationRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.CreateOrganizationRunnerGroup(ctx, "\n", req)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.CreateOrganizationRunnerGroup(ctx, "o", req)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_UpdateOrganizationRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PATCH")
		fmt.Fprint(w, `{"id":2,"name":"octo-runner-group","visibility":"selected","default":false,"selected_repositories_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories","runners_url":"https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners","inherited":false,"allows_public_repositories":true}`)
	})

	ctx := context.Background()
	req := UpdateRunnerGroupRequest{
		Name:       String("octo-runner-group"),
		Visibility: String("selected"),
	}
	group, _, err := client.Actions.UpdateOrganizationRunnerGroup(ctx, "o", 2, req)
	if err != nil {
		t.Errorf("Actions.UpdateOrganizationRunnerGroup returned error: %v", err)
	}

	want := &RunnerGroup{
		ID:                       Int64(2),
		Name:                     String("octo-runner-group"),
		Visibility:               String("selected"),
		Default:                  Bool(false),
		SelectedRepositoriesURL:  String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/repositories"),
		RunnersURL:               String("https://api.github.com/orgs/octo-org/actions/runner_groups/2/runners"),
		Inherited:                Bool(false),
		AllowsPublicRepositories: Bool(true),
	}

	if !reflect.DeepEqual(group, want) {
		t.Errorf("Actions.UpdateOrganizationRunnerGroup returned %+v, want %+v", group, want)
	}

	const methodName = "UpdateOrganizationRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.UpdateOrganizationRunnerGroup(ctx, "\n", 2, req)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.UpdateOrganizationRunnerGroup(ctx, "o", 2, req)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_ListRepositoryAccessRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/repositories", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"total_count": 1, "repositories": [{"id": 43, "node_id": "MDEwOlJlcG9zaXRvcnkxMjk2MjY5", "name": "Hello-World", "full_name": "octocat/Hello-World"}]}`)
	})

	ctx := context.Background()
	groups, _, err := client.Actions.ListRepositoryAccessRunnerGroup(ctx, "o", 2)
	if err != nil {
		t.Errorf("Actions.ListRepositoryAccessRunnerGroup returned error: %v", err)
	}

	want := &ListRepositories{
		TotalCount: Int(1),
		Repositories: []*Repository{
			{ID: Int64(43), NodeID: String("MDEwOlJlcG9zaXRvcnkxMjk2MjY5"), Name: String("Hello-World"), FullName: String("octocat/Hello-World")},
		},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("Actions.ListRepositoryAccessRunnerGroup returned %+v, want %+v", groups, want)
	}

	const methodName = "ListRepositoryAccessRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListRepositoryAccessRunnerGroup(ctx, "\n", 2)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListRepositoryAccessRunnerGroup(ctx, "o", 2)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_SetRepositoryAccessRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/repositories", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
	})

	req := SetRepoAccessRunnerGroupRequest{
		SelectedRepositoryIDs: []int64{
			1,
			2,
		},
	}

	ctx := context.Background()
	_, err := client.Actions.SetRepositoryAccessRunnerGroup(ctx, "o", 2, req)
	if err != nil {
		t.Errorf("Actions.SetRepositoryAccessRunnerGroup returned error: %v", err)
	}

	const methodName = "SetRepositoryAccessRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.SetRepositoryAccessRunnerGroup(ctx, "\n", 2, req)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.SetRepositoryAccessRunnerGroup(ctx, "o", 2, req)
	})
}

func TestActionsService_AddRepositoryAccessRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/repositories/42", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
	})

	ctx := context.Background()
	_, err := client.Actions.AddRepositoryAccessRunnerGroup(ctx, "o", 2, 42)
	if err != nil {
		t.Errorf("Actions.AddRepositoryAccessRunnerGroup returned error: %v", err)
	}

	const methodName = "AddRepositoryAccessRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.AddRepositoryAccessRunnerGroup(ctx, "\n", 2, 42)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.AddRepositoryAccessRunnerGroup(ctx, "o", 2, 42)
	})
}

func TestActionsService_RemoveRepositoryAccessRunnerGroup(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/repositories/42", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Actions.RemoveRepositoryAccessRunnerGroup(ctx, "o", 2, 42)
	if err != nil {
		t.Errorf("Actions.RemoveRepositoryAccessRunnerGroup returned error: %v", err)
	}

	const methodName = "RemoveRepositoryAccessRunnerGroup"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.RemoveRepositoryAccessRunnerGroup(ctx, "\n", 2, 42)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.RemoveRepositoryAccessRunnerGroup(ctx, "o", 2, 42)
	})
}

func TestActionsService_ListRunerGroupRunners(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/runners", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":2,"runners":[{"id":23,"name":"MBP","os":"macos","status":"online"},{"id":24,"name":"iMac","os":"macos","status":"offline"}]}`)
	})

	opts := &ListOptions{Page: 2, PerPage: 2}
	ctx := context.Background()
	runners, _, err := client.Actions.ListRunerGroupRunners(ctx, "o", 2, opts)
	if err != nil {
		t.Errorf("Actions.ListRunerGroupRunners returned error: %v", err)
	}

	want := &Runners{
		TotalCount: 2,
		Runners: []*Runner{
			{ID: Int64(23), Name: String("MBP"), OS: String("macos"), Status: String("online")},
			{ID: Int64(24), Name: String("iMac"), OS: String("macos"), Status: String("offline")},
		},
	}
	if !reflect.DeepEqual(runners, want) {
		t.Errorf("Actions.ListRunerGroupRunners returned %+v, want %+v", runners, want)
	}

	const methodName = "ListRunerGroupRunners"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListRunerGroupRunners(ctx, "\n", 2, opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListRunerGroupRunners(ctx, "o", 2, opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_SetRunerGroupRunners(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/runners", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
	})

	req := SetRunnerGroupRunnersRequest{
		Runners: []int64{
			1,
			2,
		},
	}

	ctx := context.Background()
	_, err := client.Actions.SetRunerGroupRunners(ctx, "o", 2, req)
	if err != nil {
		t.Errorf("Actions.SetRunerGroupRunners returned error: %v", err)
	}

	const methodName = "SetRunerGroupRunners"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.SetRunerGroupRunners(ctx, "\n", 2, req)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.SetRunerGroupRunners(ctx, "o", 2, req)
	})
}

func TestActionsService_AddRunerGroupRunners(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/runners/42", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
	})

	ctx := context.Background()
	_, err := client.Actions.AddRunerGroupRunners(ctx, "o", 2, 42)
	if err != nil {
		t.Errorf("Actions.AddRunerGroupRunners returned error: %v", err)
	}

	const methodName = "AddRunerGroupRunners"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.AddRunerGroupRunners(ctx, "\n", 2, 42)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.AddRunerGroupRunners(ctx, "o", 2, 42)
	})
}

func TestActionsService_RemoveRunerGroupRunners(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runner-groups/2/runners/42", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Actions.RemoveRunerGroupRunners(ctx, "o", 2, 42)
	if err != nil {
		t.Errorf("Actions.RemoveRunerGroupRunners returned error: %v", err)
	}

	const methodName = "RemoveRunerGroupRunners"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.RemoveRunerGroupRunners(ctx, "\n", 2, 42)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.RemoveRunerGroupRunners(ctx, "o", 2, 42)
	})
}
