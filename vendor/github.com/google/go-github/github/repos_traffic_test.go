// Copyright 2016 The go-github AUTHORS. All rights reserved.
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

func TestRepositoriesService_ListTrafficReferrers(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/traffic/popular/referrers", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprintf(w, `[{
			"referrer": "Google",
			"count": 4,
			"uniques": 3
 		}]`)
	})
	ctx := context.Background()
	got, _, err := client.Repositories.ListTrafficReferrers(ctx, "o", "r")
	if err != nil {
		t.Errorf("Repositories.ListTrafficReferrers returned error: %+v", err)
	}

	want := []*TrafficReferrer{{
		Referrer: String("Google"),
		Count:    Int(4),
		Uniques:  Int(3),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Repositories.ListTrafficReferrers returned %+v, want %+v", got, want)
	}

	const methodName = "ListTrafficReferrers"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListTrafficReferrers(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListTrafficReferrers(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListTrafficPaths(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/traffic/popular/paths", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprintf(w, `[{
			"path": "/github/hubot",
			"title": "github/hubot: A customizable life embetterment robot.",
			"count": 3542,
			"uniques": 2225
 		}]`)
	})
	ctx := context.Background()
	got, _, err := client.Repositories.ListTrafficPaths(ctx, "o", "r")
	if err != nil {
		t.Errorf("Repositories.ListTrafficPaths returned error: %+v", err)
	}

	want := []*TrafficPath{{
		Path:    String("/github/hubot"),
		Title:   String("github/hubot: A customizable life embetterment robot."),
		Count:   Int(3542),
		Uniques: Int(2225),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Repositories.ListTrafficPaths returned %+v, want %+v", got, want)
	}

	const methodName = "ListTrafficPaths"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListTrafficPaths(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListTrafficPaths(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListTrafficViews(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/traffic/views", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprintf(w, `{"count": 7,
			"uniques": 6,
			"views": [{
				"timestamp": "2016-05-31T16:00:00.000Z",
				"count": 7,
				"uniques": 6
		}]}`)
	})

	ctx := context.Background()
	got, _, err := client.Repositories.ListTrafficViews(ctx, "o", "r", nil)
	if err != nil {
		t.Errorf("Repositories.ListTrafficViews returned error: %+v", err)
	}

	want := &TrafficViews{
		Views: []*TrafficData{{
			Timestamp: &Timestamp{time.Date(2016, time.May, 31, 16, 0, 0, 0, time.UTC)},
			Count:     Int(7),
			Uniques:   Int(6),
		}},
		Count:   Int(7),
		Uniques: Int(6),
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Repositories.ListTrafficViews returned %+v, want %+v", got, want)
	}

	const methodName = "ListTrafficViews"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListTrafficViews(ctx, "\n", "\n", &TrafficBreakdownOptions{})
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListTrafficViews(ctx, "o", "r", nil)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListTrafficClones(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/traffic/clones", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprintf(w, `{"count": 7,
			"uniques": 6,
			"clones": [{
				"timestamp": "2016-05-31T16:00:00.00Z",
				"count": 7,
				"uniques": 6
		}]}`)
	})

	ctx := context.Background()
	got, _, err := client.Repositories.ListTrafficClones(ctx, "o", "r", nil)
	if err != nil {
		t.Errorf("Repositories.ListTrafficClones returned error: %+v", err)
	}

	want := &TrafficClones{
		Clones: []*TrafficData{{
			Timestamp: &Timestamp{time.Date(2016, time.May, 31, 16, 0, 0, 0, time.UTC)},
			Count:     Int(7),
			Uniques:   Int(6),
		}},
		Count:   Int(7),
		Uniques: Int(6),
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Repositories.ListTrafficClones returned %+v, want %+v", got, want)
	}

	const methodName = "ListTrafficClones"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListTrafficClones(ctx, "\n", "\n", &TrafficBreakdownOptions{})
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListTrafficClones(ctx, "o", "r", nil)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}
