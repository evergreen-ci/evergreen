// Copyright 2016 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestAppsService_Get_authenticatedApp(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":1}`)
	})

	ctx := context.Background()
	app, _, err := client.Apps.Get(ctx, "")
	if err != nil {
		t.Errorf("Apps.Get returned error: %v", err)
	}

	want := &App{ID: Int64(1)}
	if !reflect.DeepEqual(app, want) {
		t.Errorf("Apps.Get returned %+v, want %+v", app, want)
	}

	const methodName = "Get"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.Get(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.Get(ctx, "")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_Get_specifiedApp(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/apps/a", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"html_url":"https://github.com/apps/a"}`)
	})

	ctx := context.Background()
	app, _, err := client.Apps.Get(ctx, "a")
	if err != nil {
		t.Errorf("Apps.Get returned error: %v", err)
	}

	want := &App{HTMLURL: String("https://github.com/apps/a")}
	if !reflect.DeepEqual(app, want) {
		t.Errorf("Apps.Get returned %+v, want %+v", *app.HTMLURL, *want.HTMLURL)
	}
}

func TestAppsService_ListInstallations(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app/installations", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"page":     "1",
			"per_page": "2",
		})
		fmt.Fprint(w, `[{
                                   "id":1,
                                   "app_id":1,
                                   "target_id":1,
                                   "target_type": "Organization",
                                   "permissions": {
                                       "actions": "read",
                                       "administration": "read",
                                       "checks": "read",
                                       "contents": "read",
                                       "content_references": "read",
                                       "deployments": "read",
                                       "issues": "write",
                                       "metadata": "read",
                                       "members": "read",
                                       "organization_administration": "write",
                                       "organization_hooks": "write",
                                       "organization_plan": "read",
                                       "organization_pre_receive_hooks": "write",
                                       "organization_projects": "read",
                                       "organization_user_blocking": "write",
                                       "packages": "read",
                                       "pages": "read",
                                       "pull_requests": "write",
                                       "repository_hooks": "write",
                                       "repository_projects": "read",
                                       "repository_pre_receive_hooks": "read",
                                       "single_file": "write",
                                       "statuses": "write",
                                       "team_discussions": "read",
                                       "vulnerability_alerts": "read"
                                   },
                                  "events": [
                                      "push",
                                      "pull_request"
                                  ],
                                 "single_file_name": "config.yml",
                                 "repository_selection": "selected",
                                 "created_at": "2018-01-01T00:00:00Z",
                                 "updated_at": "2018-01-01T00:00:00Z"}]`,
		)
	})

	opt := &ListOptions{Page: 1, PerPage: 2}
	ctx := context.Background()
	installations, _, err := client.Apps.ListInstallations(ctx, opt)
	if err != nil {
		t.Errorf("Apps.ListInstallations returned error: %v", err)
	}

	date := Timestamp{Time: time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)}
	want := []*Installation{{
		ID:                  Int64(1),
		AppID:               Int64(1),
		TargetID:            Int64(1),
		TargetType:          String("Organization"),
		SingleFileName:      String("config.yml"),
		RepositorySelection: String("selected"),
		Permissions: &InstallationPermissions{
			Actions:                     String("read"),
			Administration:              String("read"),
			Checks:                      String("read"),
			Contents:                    String("read"),
			ContentReferences:           String("read"),
			Deployments:                 String("read"),
			Issues:                      String("write"),
			Metadata:                    String("read"),
			Members:                     String("read"),
			OrganizationAdministration:  String("write"),
			OrganizationHooks:           String("write"),
			OrganizationPlan:            String("read"),
			OrganizationPreReceiveHooks: String("write"),
			OrganizationProjects:        String("read"),
			OrganizationUserBlocking:    String("write"),
			Packages:                    String("read"),
			Pages:                       String("read"),
			PullRequests:                String("write"),
			RepositoryHooks:             String("write"),
			RepositoryProjects:          String("read"),
			RepositoryPreReceiveHooks:   String("read"),
			SingleFile:                  String("write"),
			Statuses:                    String("write"),
			TeamDiscussions:             String("read"),
			VulnerabilityAlerts:         String("read")},
		Events:    []string{"push", "pull_request"},
		CreatedAt: &date,
		UpdatedAt: &date,
	}}
	if !reflect.DeepEqual(installations, want) {
		t.Errorf("Apps.ListInstallations returned %+v, want %+v", installations, want)
	}

	const methodName = "ListInstallations"
	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.ListInstallations(ctx, opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_GetInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app/installations/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":1, "app_id":1, "target_id":1, "target_type": "Organization"}`)
	})

	ctx := context.Background()
	installation, _, err := client.Apps.GetInstallation(ctx, 1)
	if err != nil {
		t.Errorf("Apps.GetInstallation returned error: %v", err)
	}

	want := &Installation{ID: Int64(1), AppID: Int64(1), TargetID: Int64(1), TargetType: String("Organization")}
	if !reflect.DeepEqual(installation, want) {
		t.Errorf("Apps.GetInstallation returned %+v, want %+v", installation, want)
	}

	const methodName = "GetInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.GetInstallation(ctx, -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.GetInstallation(ctx, 1)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_ListUserInstallations(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/user/installations", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"page":     "1",
			"per_page": "2",
		})
		fmt.Fprint(w, `{"installations":[{"id":1, "app_id":1, "target_id":1, "target_type": "Organization"}]}`)
	})

	opt := &ListOptions{Page: 1, PerPage: 2}
	ctx := context.Background()
	installations, _, err := client.Apps.ListUserInstallations(ctx, opt)
	if err != nil {
		t.Errorf("Apps.ListUserInstallations returned error: %v", err)
	}

	want := []*Installation{{ID: Int64(1), AppID: Int64(1), TargetID: Int64(1), TargetType: String("Organization")}}
	if !reflect.DeepEqual(installations, want) {
		t.Errorf("Apps.ListUserInstallations returned %+v, want %+v", installations, want)
	}

	const methodName = "ListUserInstallations"
	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.ListUserInstallations(ctx, opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_SuspendInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app/installations/1/suspended", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")

		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	if _, err := client.Apps.SuspendInstallation(ctx, 1); err != nil {
		t.Errorf("Apps.SuspendInstallation returned error: %v", err)
	}

	const methodName = "SuspendInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Apps.SuspendInstallation(ctx, -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Apps.SuspendInstallation(ctx, 1)
	})
}

func TestAppsService_UnsuspendInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app/installations/1/suspended", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")

		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	if _, err := client.Apps.UnsuspendInstallation(ctx, 1); err != nil {
		t.Errorf("Apps.UnsuspendInstallation returned error: %v", err)
	}

	const methodName = "UnsuspendInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Apps.UnsuspendInstallation(ctx, -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Apps.UnsuspendInstallation(ctx, 1)
	})
}

func TestAppsService_DeleteInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app/installations/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	_, err := client.Apps.DeleteInstallation(ctx, 1)
	if err != nil {
		t.Errorf("Apps.DeleteInstallation returned error: %v", err)
	}

	const methodName = "DeleteInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Apps.DeleteInstallation(ctx, -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Apps.DeleteInstallation(ctx, 1)
	})
}

func TestAppsService_CreateInstallationToken(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/app/installations/1/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"token":"t"}`)
	})

	ctx := context.Background()
	token, _, err := client.Apps.CreateInstallationToken(ctx, 1, nil)
	if err != nil {
		t.Errorf("Apps.CreateInstallationToken returned error: %v", err)
	}

	want := &InstallationToken{Token: String("t")}
	if !reflect.DeepEqual(token, want) {
		t.Errorf("Apps.CreateInstallationToken returned %+v, want %+v", token, want)
	}

	const methodName = "CreateInstallationToken"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.CreateInstallationToken(ctx, -1, nil)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.CreateInstallationToken(ctx, 1, nil)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_CreateInstallationTokenWithOptions(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	installationTokenOptions := &InstallationTokenOptions{
		RepositoryIDs: []int64{1234},
		Permissions: &InstallationPermissions{
			Contents: String("write"),
			Issues:   String("read"),
		},
	}

	// Convert InstallationTokenOptions into an io.ReadCloser object for comparison.
	wantBody, err := GetReadCloser(installationTokenOptions)
	if err != nil {
		t.Errorf("GetReadCloser returned error: %v", err)
	}

	mux.HandleFunc("/app/installations/1/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		// Read request body contents.
		var gotBodyBytes []byte
		gotBodyBytes, err = ioutil.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll returned error: %v", err)
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(gotBodyBytes))

		if !reflect.DeepEqual(r.Body, wantBody) {
			t.Errorf("request sent %+v, want %+v", r.Body, wantBody)
		}

		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"token":"t"}`)
	})

	ctx := context.Background()
	token, _, err := client.Apps.CreateInstallationToken(ctx, 1, installationTokenOptions)
	if err != nil {
		t.Errorf("Apps.CreateInstallationToken returned error: %v", err)
	}

	want := &InstallationToken{Token: String("t")}
	if !reflect.DeepEqual(token, want) {
		t.Errorf("Apps.CreateInstallationToken returned %+v, want %+v", token, want)
	}
}

func TestAppsService_CreateAttachement(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/content_references/11/attachments", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		testHeader(t, r, "Accept", mediaTypeContentAttachmentsPreview)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":1,"title":"title1","body":"body1"}`))
	})

	ctx := context.Background()
	got, _, err := client.Apps.CreateAttachment(ctx, 11, "title1", "body1")
	if err != nil {
		t.Errorf("CreateAttachment returned error: %v", err)
	}

	want := &Attachment{ID: Int64(1), Title: String("title1"), Body: String("body1")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CreateAttachment = %+v, want %+v", got, want)
	}

	const methodName = "CreateAttachment"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.CreateAttachment(ctx, -11, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.CreateAttachment(ctx, 11, "title1", "body1")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_FindOrganizationInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/installation", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":1, "app_id":1, "target_id":1, "target_type": "Organization"}`)
	})

	ctx := context.Background()
	installation, _, err := client.Apps.FindOrganizationInstallation(ctx, "o")
	if err != nil {
		t.Errorf("Apps.FindOrganizationInstallation returned error: %v", err)
	}

	want := &Installation{ID: Int64(1), AppID: Int64(1), TargetID: Int64(1), TargetType: String("Organization")}
	if !reflect.DeepEqual(installation, want) {
		t.Errorf("Apps.FindOrganizationInstallation returned %+v, want %+v", installation, want)
	}

	const methodName = "FindOrganizationInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.FindOrganizationInstallation(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.FindOrganizationInstallation(ctx, "o")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_FindRepositoryInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/installation", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":1, "app_id":1, "target_id":1, "target_type": "Organization"}`)
	})

	ctx := context.Background()
	installation, _, err := client.Apps.FindRepositoryInstallation(ctx, "o", "r")
	if err != nil {
		t.Errorf("Apps.FindRepositoryInstallation returned error: %v", err)
	}

	want := &Installation{ID: Int64(1), AppID: Int64(1), TargetID: Int64(1), TargetType: String("Organization")}
	if !reflect.DeepEqual(installation, want) {
		t.Errorf("Apps.FindRepositoryInstallation returned %+v, want %+v", installation, want)
	}

	const methodName = "FindRepositoryInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.FindRepositoryInstallation(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.FindRepositoryInstallation(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_FindRepositoryInstallationByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repositories/1/installation", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":1, "app_id":1, "target_id":1, "target_type": "Organization"}`)
	})

	ctx := context.Background()
	installation, _, err := client.Apps.FindRepositoryInstallationByID(ctx, 1)
	if err != nil {
		t.Errorf("Apps.FindRepositoryInstallationByID returned error: %v", err)
	}

	want := &Installation{ID: Int64(1), AppID: Int64(1), TargetID: Int64(1), TargetType: String("Organization")}
	if !reflect.DeepEqual(installation, want) {
		t.Errorf("Apps.FindRepositoryInstallationByID returned %+v, want %+v", installation, want)
	}

	const methodName = "FindRepositoryInstallationByID"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.FindRepositoryInstallationByID(ctx, -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.FindRepositoryInstallationByID(ctx, 1)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestAppsService_FindUserInstallation(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/users/u/installation", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":1, "app_id":1, "target_id":1, "target_type": "User"}`)
	})

	ctx := context.Background()
	installation, _, err := client.Apps.FindUserInstallation(ctx, "u")
	if err != nil {
		t.Errorf("Apps.FindUserInstallation returned error: %v", err)
	}

	want := &Installation{ID: Int64(1), AppID: Int64(1), TargetID: Int64(1), TargetType: String("User")}
	if !reflect.DeepEqual(installation, want) {
		t.Errorf("Apps.FindUserInstallation returned %+v, want %+v", installation, want)
	}

	const methodName = "FindUserInstallation"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Apps.FindUserInstallation(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Apps.FindUserInstallation(ctx, "u")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

// GetReadWriter converts a body interface into an io.ReadWriter object.
func GetReadWriter(body interface{}) (io.ReadWriter, error) {
	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := enc.Encode(body)
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// GetReadCloser converts a body interface into an io.ReadCloser object.
func GetReadCloser(body interface{}) (io.ReadCloser, error) {
	buf, err := GetReadWriter(body)
	if err != nil {
		return nil, err
	}

	all, err := ioutil.ReadAll(buf)
	if err != nil {
		return nil, err
	}
	return ioutil.NopCloser(bytes.NewBuffer(all)), nil
}
