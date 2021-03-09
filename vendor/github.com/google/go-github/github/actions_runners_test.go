// Copyright 2020 The go-github AUTHORS. All rights reserved.
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
	"time"
)

func TestActionsService_ListRunnerApplicationDownloads(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runners/downloads", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `[{"os":"osx","architecture":"x64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-osx-x64-2.164.0.tar.gz","filename":"actions-runner-osx-x64-2.164.0.tar.gz"},{"os":"linux","architecture":"x64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-x64-2.164.0.tar.gz","filename":"actions-runner-linux-x64-2.164.0.tar.gz"},{"os": "linux","architecture":"arm","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm-2.164.0.tar.gz","filename":"actions-runner-linux-arm-2.164.0.tar.gz"},{"os":"win","architecture":"x64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-win-x64-2.164.0.zip","filename":"actions-runner-win-x64-2.164.0.zip"},{"os":"linux","architecture":"arm64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm64-2.164.0.tar.gz","filename":"actions-runner-linux-arm64-2.164.0.tar.gz"}]`)
	})

	ctx := context.Background()
	downloads, _, err := client.Actions.ListRunnerApplicationDownloads(ctx, "o", "r")
	if err != nil {
		t.Errorf("Actions.ListRunnerApplicationDownloads returned error: %v", err)
	}

	want := []*RunnerApplicationDownload{
		{OS: String("osx"), Architecture: String("x64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-osx-x64-2.164.0.tar.gz"), Filename: String("actions-runner-osx-x64-2.164.0.tar.gz")},
		{OS: String("linux"), Architecture: String("x64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-x64-2.164.0.tar.gz"), Filename: String("actions-runner-linux-x64-2.164.0.tar.gz")},
		{OS: String("linux"), Architecture: String("arm"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm-2.164.0.tar.gz"), Filename: String("actions-runner-linux-arm-2.164.0.tar.gz")},
		{OS: String("win"), Architecture: String("x64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-win-x64-2.164.0.zip"), Filename: String("actions-runner-win-x64-2.164.0.zip")},
		{OS: String("linux"), Architecture: String("arm64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm64-2.164.0.tar.gz"), Filename: String("actions-runner-linux-arm64-2.164.0.tar.gz")},
	}
	if !reflect.DeepEqual(downloads, want) {
		t.Errorf("Actions.ListRunnerApplicationDownloads returned %+v, want %+v", downloads, want)
	}

	const methodName = "ListRunnerApplicationDownloads"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListRunnerApplicationDownloads(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListRunnerApplicationDownloads(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_CreateRegistrationToken(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runners/registration-token", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"token":"LLBF3JGZDX3P5PMEXLND6TS6FCWO6","expires_at":"2020-01-22T12:13:35.123Z"}`)
	})

	ctx := context.Background()
	token, _, err := client.Actions.CreateRegistrationToken(ctx, "o", "r")
	if err != nil {
		t.Errorf("Actions.CreateRegistrationToken returned error: %v", err)
	}

	want := &RegistrationToken{Token: String("LLBF3JGZDX3P5PMEXLND6TS6FCWO6"),
		ExpiresAt: &Timestamp{time.Date(2020, time.January, 22, 12, 13, 35,
			123000000, time.UTC)}}
	if !reflect.DeepEqual(token, want) {
		t.Errorf("Actions.CreateRegistrationToken returned %+v, want %+v", token, want)
	}

	const methodName = "CreateRegistrationToken"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.CreateRegistrationToken(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.CreateRegistrationToken(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_ListRunners(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runners", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":2,"runners":[{"id":23,"name":"MBP","os":"macos","status":"online"},{"id":24,"name":"iMac","os":"macos","status":"offline"}]}`)
	})

	opts := &ListOptions{Page: 2, PerPage: 2}
	ctx := context.Background()
	runners, _, err := client.Actions.ListRunners(ctx, "o", "r", opts)
	if err != nil {
		t.Errorf("Actions.ListRunners returned error: %v", err)
	}

	want := &Runners{
		TotalCount: 2,
		Runners: []*Runner{
			{ID: Int64(23), Name: String("MBP"), OS: String("macos"), Status: String("online")},
			{ID: Int64(24), Name: String("iMac"), OS: String("macos"), Status: String("offline")},
		},
	}
	if !reflect.DeepEqual(runners, want) {
		t.Errorf("Actions.ListRunners returned %+v, want %+v", runners, want)
	}

	const methodName = "ListRunners"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListRunners(ctx, "\n", "\n", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListRunners(ctx, "o", "r", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetRunner(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runners/23", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":23,"name":"MBP","os":"macos","status":"online"}`)
	})

	ctx := context.Background()
	runner, _, err := client.Actions.GetRunner(ctx, "o", "r", 23)
	if err != nil {
		t.Errorf("Actions.GetRunner returned error: %v", err)
	}

	want := &Runner{
		ID:     Int64(23),
		Name:   String("MBP"),
		OS:     String("macos"),
		Status: String("online"),
	}
	if !reflect.DeepEqual(runner, want) {
		t.Errorf("Actions.GetRunner returned %+v, want %+v", runner, want)
	}

	const methodName = "GetRunner"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetRunner(ctx, "\n", "\n", 23)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetRunner(ctx, "o", "r", 23)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_CreateRemoveToken(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runners/remove-token", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"token":"AABF3JGZDX3P5PMEXLND6TS6FCWO6","expires_at":"2020-01-29T12:13:35.123Z"}`)
	})

	ctx := context.Background()
	token, _, err := client.Actions.CreateRemoveToken(ctx, "o", "r")
	if err != nil {
		t.Errorf("Actions.CreateRemoveToken returned error: %v", err)
	}

	want := &RemoveToken{Token: String("AABF3JGZDX3P5PMEXLND6TS6FCWO6"), ExpiresAt: &Timestamp{time.Date(2020, time.January, 29, 12, 13, 35, 123000000, time.UTC)}}
	if !reflect.DeepEqual(token, want) {
		t.Errorf("Actions.CreateRemoveToken returned %+v, want %+v", token, want)
	}

	const methodName = "CreateRemoveToken"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.CreateRemoveToken(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.CreateRemoveToken(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_RemoveRunner(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/actions/runners/21", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Actions.RemoveRunner(ctx, "o", "r", 21)
	if err != nil {
		t.Errorf("Actions.RemoveRunner returned error: %v", err)
	}

	const methodName = "RemoveRunner"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.RemoveRunner(ctx, "\n", "\n", 21)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.RemoveRunner(ctx, "o", "r", 21)
	})
}

func TestActionsService_ListOrganizationRunnerApplicationDownloads(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runners/downloads", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `[{"os":"osx","architecture":"x64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-osx-x64-2.164.0.tar.gz","filename":"actions-runner-osx-x64-2.164.0.tar.gz"},{"os":"linux","architecture":"x64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-x64-2.164.0.tar.gz","filename":"actions-runner-linux-x64-2.164.0.tar.gz"},{"os": "linux","architecture":"arm","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm-2.164.0.tar.gz","filename":"actions-runner-linux-arm-2.164.0.tar.gz"},{"os":"win","architecture":"x64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-win-x64-2.164.0.zip","filename":"actions-runner-win-x64-2.164.0.zip"},{"os":"linux","architecture":"arm64","download_url":"https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm64-2.164.0.tar.gz","filename":"actions-runner-linux-arm64-2.164.0.tar.gz"}]`)
	})

	ctx := context.Background()
	downloads, _, err := client.Actions.ListOrganizationRunnerApplicationDownloads(ctx, "o")
	if err != nil {
		t.Errorf("Actions.ListRunnerApplicationDownloads returned error: %v", err)
	}

	want := []*RunnerApplicationDownload{
		{OS: String("osx"), Architecture: String("x64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-osx-x64-2.164.0.tar.gz"), Filename: String("actions-runner-osx-x64-2.164.0.tar.gz")},
		{OS: String("linux"), Architecture: String("x64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-x64-2.164.0.tar.gz"), Filename: String("actions-runner-linux-x64-2.164.0.tar.gz")},
		{OS: String("linux"), Architecture: String("arm"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm-2.164.0.tar.gz"), Filename: String("actions-runner-linux-arm-2.164.0.tar.gz")},
		{OS: String("win"), Architecture: String("x64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-win-x64-2.164.0.zip"), Filename: String("actions-runner-win-x64-2.164.0.zip")},
		{OS: String("linux"), Architecture: String("arm64"), DownloadURL: String("https://github.com/actions/runner/releases/download/v2.164.0/actions-runner-linux-arm64-2.164.0.tar.gz"), Filename: String("actions-runner-linux-arm64-2.164.0.tar.gz")},
	}
	if !reflect.DeepEqual(downloads, want) {
		t.Errorf("Actions.ListOrganizationRunnerApplicationDownloads returned %+v, want %+v", downloads, want)
	}

	const methodName = "ListOrganizationRunnerApplicationDownloads"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListOrganizationRunnerApplicationDownloads(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListOrganizationRunnerApplicationDownloads(ctx, "o")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_CreateOrganizationRegistrationToken(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runners/registration-token", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"token":"LLBF3JGZDX3P5PMEXLND6TS6FCWO6","expires_at":"2020-01-22T12:13:35.123Z"}`)
	})

	ctx := context.Background()
	token, _, err := client.Actions.CreateOrganizationRegistrationToken(ctx, "o")
	if err != nil {
		t.Errorf("Actions.CreateRegistrationToken returned error: %v", err)
	}

	want := &RegistrationToken{Token: String("LLBF3JGZDX3P5PMEXLND6TS6FCWO6"),
		ExpiresAt: &Timestamp{time.Date(2020, time.January, 22, 12, 13, 35,
			123000000, time.UTC)}}
	if !reflect.DeepEqual(token, want) {
		t.Errorf("Actions.CreateRegistrationToken returned %+v, want %+v", token, want)
	}

	const methodName = "CreateOrganizationRegistrationToken"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.CreateOrganizationRegistrationToken(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.CreateOrganizationRegistrationToken(ctx, "o")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_ListOrganizationRunners(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runners", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"per_page": "2", "page": "2"})
		fmt.Fprint(w, `{"total_count":2,"runners":[{"id":23,"name":"MBP","os":"macos","status":"online"},{"id":24,"name":"iMac","os":"macos","status":"offline"}]}`)
	})

	opts := &ListOptions{Page: 2, PerPage: 2}
	ctx := context.Background()
	runners, _, err := client.Actions.ListOrganizationRunners(ctx, "o", opts)
	if err != nil {
		t.Errorf("Actions.ListRunners returned error: %v", err)
	}

	want := &Runners{
		TotalCount: 2,
		Runners: []*Runner{
			{ID: Int64(23), Name: String("MBP"), OS: String("macos"), Status: String("online")},
			{ID: Int64(24), Name: String("iMac"), OS: String("macos"), Status: String("offline")},
		},
	}
	if !reflect.DeepEqual(runners, want) {
		t.Errorf("Actions.ListRunners returned %+v, want %+v", runners, want)
	}

	const methodName = "ListOrganizationRunners"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListOrganizationRunners(ctx, "\n", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListOrganizationRunners(ctx, "o", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_ListEnabledReposInOrg(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/permissions/repositories", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"page": "1",
		})
		fmt.Fprint(w, `{"total_count":2,"repositories":[{"id":2}, {"id": 3}]}`)
	})

	ctx := context.Background()
	opt := &ListOptions{
		Page: 1,
	}
	got, _, err := client.Actions.ListEnabledReposInOrg(ctx, "o", opt)
	if err != nil {
		t.Errorf("Actions.ListEnabledReposInOrg returned error: %v", err)
	}

	want := &ActionsEnabledOnOrgRepos{TotalCount: int(2), Repositories: []*Repository{
		{ID: Int64(2)},
		{ID: Int64(3)},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Actions.ListEnabledReposInOrg returned %+v, want %+v", got, want)
	}

	const methodName = "ListEnabledReposInOrg"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.ListEnabledReposInOrg(ctx, "\n", opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.ListEnabledReposInOrg(ctx, "o", opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_GetOrganizationRunner(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runners/23", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":23,"name":"MBP","os":"macos","status":"online"}`)
	})

	ctx := context.Background()
	runner, _, err := client.Actions.GetOrganizationRunner(ctx, "o", 23)
	if err != nil {
		t.Errorf("Actions.GetRunner returned error: %v", err)
	}

	want := &Runner{
		ID:     Int64(23),
		Name:   String("MBP"),
		OS:     String("macos"),
		Status: String("online"),
	}
	if !reflect.DeepEqual(runner, want) {
		t.Errorf("Actions.GetRunner returned %+v, want %+v", runner, want)
	}

	const methodName = "GetOrganizationRunner"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.GetOrganizationRunner(ctx, "\n", 23)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.GetOrganizationRunner(ctx, "o", 23)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_CreateOrganizationRemoveToken(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runners/remove-token", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"token":"AABF3JGZDX3P5PMEXLND6TS6FCWO6","expires_at":"2020-01-29T12:13:35.123Z"}`)
	})

	ctx := context.Background()
	token, _, err := client.Actions.CreateOrganizationRemoveToken(ctx, "o")
	if err != nil {
		t.Errorf("Actions.CreateRemoveToken returned error: %v", err)
	}

	want := &RemoveToken{Token: String("AABF3JGZDX3P5PMEXLND6TS6FCWO6"), ExpiresAt: &Timestamp{time.Date(2020, time.January, 29, 12, 13, 35, 123000000, time.UTC)}}
	if !reflect.DeepEqual(token, want) {
		t.Errorf("Actions.CreateRemoveToken returned %+v, want %+v", token, want)
	}

	const methodName = "CreateOrganizationRemoveToken"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Actions.CreateOrganizationRemoveToken(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Actions.CreateOrganizationRemoveToken(ctx, "o")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActionsService_RemoveOrganizationRunner(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/actions/runners/21", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	ctx := context.Background()
	_, err := client.Actions.RemoveOrganizationRunner(ctx, "o", 21)
	if err != nil {
		t.Errorf("Actions.RemoveOganizationRunner returned error: %v", err)
	}

	const methodName = "RemoveOrganizationRunner"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Actions.RemoveOrganizationRunner(ctx, "\n", 21)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Actions.RemoveOrganizationRunner(ctx, "o", 21)
	})
}
