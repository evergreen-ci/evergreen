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
	"testing"
	"time"
)

func TestActivityService_ListNotification(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/notifications", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"all":           "true",
			"participating": "true",
			"since":         "2006-01-02T15:04:05Z",
			"before":        "2007-03-04T15:04:05Z",
		})

		fmt.Fprint(w, `[{"id":"1", "subject":{"title":"t"}}]`)
	})

	opt := &NotificationListOptions{
		All:           true,
		Participating: true,
		Since:         time.Date(2006, time.January, 02, 15, 04, 05, 0, time.UTC),
		Before:        time.Date(2007, time.March, 04, 15, 04, 05, 0, time.UTC),
	}
	ctx := context.Background()
	notifications, _, err := client.Activity.ListNotifications(ctx, opt)
	if err != nil {
		t.Errorf("Activity.ListNotifications returned error: %v", err)
	}

	want := []*Notification{{ID: String("1"), Subject: &NotificationSubject{Title: String("t")}}}
	if !reflect.DeepEqual(notifications, want) {
		t.Errorf("Activity.ListNotifications returned %+v, want %+v", notifications, want)
	}

	const methodName = "ListNotifications"
	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Activity.ListNotifications(ctx, opt)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActivityService_ListRepositoryNotification(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/notifications", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `[{"id":"1"}]`)
	})

	ctx := context.Background()
	notifications, _, err := client.Activity.ListRepositoryNotifications(ctx, "o", "r", nil)
	if err != nil {
		t.Errorf("Activity.ListRepositoryNotifications returned error: %v", err)
	}

	want := []*Notification{{ID: String("1")}}
	if !reflect.DeepEqual(notifications, want) {
		t.Errorf("Activity.ListRepositoryNotifications returned %+v, want %+v", notifications, want)
	}

	const methodName = "ListRepositoryNotifications"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Activity.ListRepositoryNotifications(ctx, "\n", "\n", nil)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Activity.ListRepositoryNotifications(ctx, "o", "r", nil)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActivityService_MarkNotificationsRead(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/notifications", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		testHeader(t, r, "Content-Type", "application/json")
		testBody(t, r, `{"last_read_at":"2006-01-02T15:04:05Z"}`+"\n")

		w.WriteHeader(http.StatusResetContent)
	})

	ctx := context.Background()
	_, err := client.Activity.MarkNotificationsRead(ctx, time.Date(2006, time.January, 02, 15, 04, 05, 0, time.UTC))
	if err != nil {
		t.Errorf("Activity.MarkNotificationsRead returned error: %v", err)
	}

	const methodName = "MarkNotificationsRead"
	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Activity.MarkNotificationsRead(ctx, time.Date(2006, time.January, 02, 15, 04, 05, 0, time.UTC))
	})
}

func TestActivityService_MarkRepositoryNotificationsRead(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/notifications", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		testHeader(t, r, "Content-Type", "application/json")
		testBody(t, r, `{"last_read_at":"2006-01-02T15:04:05Z"}`+"\n")

		w.WriteHeader(http.StatusResetContent)
	})

	ctx := context.Background()
	_, err := client.Activity.MarkRepositoryNotificationsRead(ctx, "o", "r", time.Date(2006, time.January, 02, 15, 04, 05, 0, time.UTC))
	if err != nil {
		t.Errorf("Activity.MarkRepositoryNotificationsRead returned error: %v", err)
	}

	const methodName = "MarkRepositoryNotificationsRead"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Activity.MarkRepositoryNotificationsRead(ctx, "\n", "\n", time.Date(2006, time.January, 02, 15, 04, 05, 0, time.UTC))
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Activity.MarkRepositoryNotificationsRead(ctx, "o", "r", time.Date(2006, time.January, 02, 15, 04, 05, 0, time.UTC))
	})
}

func TestActivityService_GetThread(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/notifications/threads/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":"1"}`)
	})

	ctx := context.Background()
	notification, _, err := client.Activity.GetThread(ctx, "1")
	if err != nil {
		t.Errorf("Activity.GetThread returned error: %v", err)
	}

	want := &Notification{ID: String("1")}
	if !reflect.DeepEqual(notification, want) {
		t.Errorf("Activity.GetThread returned %+v, want %+v", notification, want)
	}

	const methodName = "GetThread"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Activity.GetThread(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Activity.GetThread(ctx, "1")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActivityService_MarkThreadRead(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/notifications/threads/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PATCH")
		w.WriteHeader(http.StatusResetContent)
	})

	ctx := context.Background()
	_, err := client.Activity.MarkThreadRead(ctx, "1")
	if err != nil {
		t.Errorf("Activity.MarkThreadRead returned error: %v", err)
	}

	const methodName = "MarkThreadRead"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Activity.MarkThreadRead(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Activity.MarkThreadRead(ctx, "1")
	})
}

func TestActivityService_GetThreadSubscription(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/notifications/threads/1/subscription", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"subscribed":true}`)
	})

	ctx := context.Background()
	sub, _, err := client.Activity.GetThreadSubscription(ctx, "1")
	if err != nil {
		t.Errorf("Activity.GetThreadSubscription returned error: %v", err)
	}

	want := &Subscription{Subscribed: Bool(true)}
	if !reflect.DeepEqual(sub, want) {
		t.Errorf("Activity.GetThreadSubscription returned %+v, want %+v", sub, want)
	}

	const methodName = "GetThreadSubscription"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Activity.GetThreadSubscription(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Activity.GetThreadSubscription(ctx, "1")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActivityService_SetThreadSubscription(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &Subscription{Subscribed: Bool(true)}

	mux.HandleFunc("/notifications/threads/1/subscription", func(w http.ResponseWriter, r *http.Request) {
		v := new(Subscription)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PUT")
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"ignored":true}`)
	})

	ctx := context.Background()
	sub, _, err := client.Activity.SetThreadSubscription(ctx, "1", input)
	if err != nil {
		t.Errorf("Activity.SetThreadSubscription returned error: %v", err)
	}

	want := &Subscription{Ignored: Bool(true)}
	if !reflect.DeepEqual(sub, want) {
		t.Errorf("Activity.SetThreadSubscription returned %+v, want %+v", sub, want)
	}

	const methodName = "SetThreadSubscription"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Activity.SetThreadSubscription(ctx, "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Activity.SetThreadSubscription(ctx, "1", input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestActivityService_DeleteThreadSubscription(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/notifications/threads/1/subscription", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	_, err := client.Activity.DeleteThreadSubscription(ctx, "1")
	if err != nil {
		t.Errorf("Activity.DeleteThreadSubscription returned error: %v", err)
	}

	const methodName = "DeleteThreadSubscription"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Activity.DeleteThreadSubscription(ctx, "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Activity.DeleteThreadSubscription(ctx, "1")
	})
}
