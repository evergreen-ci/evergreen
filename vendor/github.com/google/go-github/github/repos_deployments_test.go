// Copyright 2014 The go-github AUTHORS. All rights reserved.
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
	"strings"
	"testing"
)

func TestRepositoriesService_ListDeployments(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/deployments", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"environment": "test"})
		fmt.Fprint(w, `[{"id":1}, {"id":2}]`)
	})

	opt := &DeploymentsListOptions{Environment: "test"}
	ctx := context.Background()
	deployments, _, err := client.Repositories.ListDeployments(ctx, "o", "r", opt)
	if err != nil {
		t.Errorf("Repositories.ListDeployments returned error: %v", err)
	}

	want := []*Deployment{{ID: Int64(1)}, {ID: Int64(2)}}
	if !reflect.DeepEqual(deployments, want) {
		t.Errorf("Repositories.ListDeployments returned %+v, want %+v", deployments, want)
	}

	const methodName = "ListDeployments"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListDeployments(ctx, "\n", "\n", opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListDeployments(ctx, "o", "r", opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_GetDeployment(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/deployments/3", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":3}`)
	})

	ctx := context.Background()
	deployment, _, err := client.Repositories.GetDeployment(ctx, "o", "r", 3)
	if err != nil {
		t.Errorf("Repositories.GetDeployment returned error: %v", err)
	}

	want := &Deployment{ID: Int64(3)}

	if !reflect.DeepEqual(deployment, want) {
		t.Errorf("Repositories.GetDeployment returned %+v, want %+v", deployment, want)
	}

	const methodName = "GetDeployment"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetDeployment(ctx, "\n", "\n", 3)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetDeployment(ctx, "o", "r", 3)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_CreateDeployment(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &DeploymentRequest{Ref: String("1111"), Task: String("deploy"), TransientEnvironment: Bool(true)}

	mux.HandleFunc("/repos/o/r/deployments", func(w http.ResponseWriter, r *http.Request) {
		v := new(DeploymentRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		wantAcceptHeaders := []string{mediaTypeDeploymentStatusPreview, mediaTypeExpandDeploymentStatusPreview}
		testHeader(t, r, "Accept", strings.Join(wantAcceptHeaders, ", "))
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"ref": "1111", "task": "deploy"}`)
	})

	ctx := context.Background()
	deployment, _, err := client.Repositories.CreateDeployment(ctx, "o", "r", input)
	if err != nil {
		t.Errorf("Repositories.CreateDeployment returned error: %v", err)
	}

	want := &Deployment{Ref: String("1111"), Task: String("deploy")}
	if !reflect.DeepEqual(deployment, want) {
		t.Errorf("Repositories.CreateDeployment returned %+v, want %+v", deployment, want)
	}

	const methodName = "CreateDeployment"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.CreateDeployment(ctx, "\n", "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.CreateDeployment(ctx, "o", "r", input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_DeleteDeployment(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/deployments/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	resp, err := client.Repositories.DeleteDeployment(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("Repositories.DeleteDeployment returned error: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Error("Repositories.DeleteDeployment should return a 204 status")
	}

	resp, err = client.Repositories.DeleteDeployment(ctx, "o", "r", 2)
	if err == nil {
		t.Error("Repositories.DeleteDeployment should return an error")
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Error("Repositories.DeleteDeployment should return a 404 status")
	}

	const methodName = "DeleteDeployment"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Repositories.DeleteDeployment(ctx, "\n", "\n", 1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Repositories.DeleteDeployment(ctx, "o", "r", 1)
	})
}

func TestRepositoriesService_ListDeploymentStatuses(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	wantAcceptHeaders := []string{mediaTypeDeploymentStatusPreview, mediaTypeExpandDeploymentStatusPreview}
	mux.HandleFunc("/repos/o/r/deployments/1/statuses", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", strings.Join(wantAcceptHeaders, ", "))
		testFormValues(t, r, values{"page": "2"})
		fmt.Fprint(w, `[{"id":1}, {"id":2}]`)
	})

	opt := &ListOptions{Page: 2}
	ctx := context.Background()
	statutses, _, err := client.Repositories.ListDeploymentStatuses(ctx, "o", "r", 1, opt)
	if err != nil {
		t.Errorf("Repositories.ListDeploymentStatuses returned error: %v", err)
	}

	want := []*DeploymentStatus{{ID: Int64(1)}, {ID: Int64(2)}}
	if !reflect.DeepEqual(statutses, want) {
		t.Errorf("Repositories.ListDeploymentStatuses returned %+v, want %+v", statutses, want)
	}

	const methodName = "ListDeploymentStatuses"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListDeploymentStatuses(ctx, "\n", "\n", 1, opt)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListDeploymentStatuses(ctx, "o", "r", 1, opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_GetDeploymentStatus(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	wantAcceptHeaders := []string{mediaTypeDeploymentStatusPreview, mediaTypeExpandDeploymentStatusPreview}
	mux.HandleFunc("/repos/o/r/deployments/3/statuses/4", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", strings.Join(wantAcceptHeaders, ", "))
		fmt.Fprint(w, `{"id":4}`)
	})

	ctx := context.Background()
	deploymentStatus, _, err := client.Repositories.GetDeploymentStatus(ctx, "o", "r", 3, 4)
	if err != nil {
		t.Errorf("Repositories.GetDeploymentStatus returned error: %v", err)
	}

	want := &DeploymentStatus{ID: Int64(4)}
	if !reflect.DeepEqual(deploymentStatus, want) {
		t.Errorf("Repositories.GetDeploymentStatus returned %+v, want %+v", deploymentStatus, want)
	}

	const methodName = "GetDeploymentStatus"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetDeploymentStatus(ctx, "\n", "\n", 3, 4)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetDeploymentStatus(ctx, "o", "r", 3, 4)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_CreateDeploymentStatus(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &DeploymentStatusRequest{State: String("inactive"), Description: String("deploy"), AutoInactive: Bool(false)}

	mux.HandleFunc("/repos/o/r/deployments/1/statuses", func(w http.ResponseWriter, r *http.Request) {
		v := new(DeploymentStatusRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		wantAcceptHeaders := []string{mediaTypeDeploymentStatusPreview, mediaTypeExpandDeploymentStatusPreview}
		testHeader(t, r, "Accept", strings.Join(wantAcceptHeaders, ", "))
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"state": "inactive", "description": "deploy"}`)
	})

	ctx := context.Background()
	deploymentStatus, _, err := client.Repositories.CreateDeploymentStatus(ctx, "o", "r", 1, input)
	if err != nil {
		t.Errorf("Repositories.CreateDeploymentStatus returned error: %v", err)
	}

	want := &DeploymentStatus{State: String("inactive"), Description: String("deploy")}
	if !reflect.DeepEqual(deploymentStatus, want) {
		t.Errorf("Repositories.CreateDeploymentStatus returned %+v, want %+v", deploymentStatus, want)
	}

	const methodName = "CreateDeploymentStatus"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.CreateDeploymentStatus(ctx, "\n", "\n", 1, input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.CreateDeploymentStatus(ctx, "o", "r", 1, input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}
