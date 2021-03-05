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

func TestBillingService_GetActionsBillingOrg(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/settings/billing/actions", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
				"total_minutes_used": 305,
				"total_paid_minutes_used": 0,
				"included_minutes": 3000,
				"minutes_used_breakdown": {
					"UBUNTU": 205,
					"MACOS": 10,
					"WINDOWS": 90
				}
			}`)
	})

	ctx := context.Background()
	hook, _, err := client.Billing.GetActionsBillingOrg(ctx, "o")
	if err != nil {
		t.Errorf("Billing.GetActionsBillingOrg returned error: %v", err)
	}

	want := &ActionBilling{
		TotalMinutesUsed:     305,
		TotalPaidMinutesUsed: 0,
		IncludedMinutes:      3000,
		MinutesUsedBreakdown: MinutesUsedBreakdown{
			Ubuntu:  205,
			MacOS:   10,
			Windows: 90,
		},
	}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Billing.GetActionsBillingOrg returned %+v, want %+v", hook, want)
	}

	const methodName = "GetActionsBillingOrg"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Billing.GetActionsBillingOrg(ctx, "\n")
		return err
	})
}

func TestBillingService_GetActionsBillingOrg_invalidOrg(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Billing.GetActionsBillingOrg(ctx, "%")
	testURLParseError(t, err)
}

func TestBillingService_GetPackagesBillingOrg(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/settings/billing/packages", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
				"total_gigabytes_bandwidth_used": 50,
				"total_paid_gigabytes_bandwidth_used": 40,
				"included_gigabytes_bandwidth": 10
			}`)
	})

	ctx := context.Background()
	hook, _, err := client.Billing.GetPackagesBillingOrg(ctx, "o")
	if err != nil {
		t.Errorf("Billing.GetPackagesBillingOrg returned error: %v", err)
	}

	want := &PackageBilling{
		TotalGigabytesBandwidthUsed:     50,
		TotalPaidGigabytesBandwidthUsed: 40,
		IncludedGigabytesBandwidth:      10,
	}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Billing.GetPackagesBillingOrg returned %+v, want %+v", hook, want)
	}

	const methodName = "GetPackagesBillingOrg"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Billing.GetPackagesBillingOrg(ctx, "\n")
		return err
	})
}

func TestBillingService_GetPackagesBillingOrg_invalidOrg(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Billing.GetPackagesBillingOrg(ctx, "%")
	testURLParseError(t, err)
}

func TestBillingService_GetStorageBillingOrg(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/settings/billing/shared-storage", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
				"days_left_in_billing_cycle": 20,
				"estimated_paid_storage_for_month": 15,
				"estimated_storage_for_month": 40
			}`)
	})

	ctx := context.Background()
	hook, _, err := client.Billing.GetStorageBillingOrg(ctx, "o")
	if err != nil {
		t.Errorf("Billing.GetStorageBillingOrg returned error: %v", err)
	}

	want := &StorageBilling{
		DaysLeftInBillingCycle:       20,
		EstimatedPaidStorageForMonth: 15,
		EstimatedStorageForMonth:     40,
	}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Billing.GetStorageBillingOrg returned %+v, want %+v", hook, want)
	}

	const methodName = "GetStorageBillingOrg"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Billing.GetStorageBillingOrg(ctx, "\n")
		return err
	})
}

func TestBillingService_GetStorageBillingOrg_invalidOrg(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Billing.GetStorageBillingOrg(ctx, "%")
	testURLParseError(t, err)
}

func TestBillingService_GetActionsBillingUser(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/users/u/settings/billing/actions", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
				"total_minutes_used": 10,
				"total_paid_minutes_used": 0,
				"included_minutes": 3000,
				"minutes_used_breakdown": {
					"UBUNTU": 205,
					"MACOS": 10,
					"WINDOWS": 90
				}
			}`)
	})

	ctx := context.Background()
	hook, _, err := client.Billing.GetActionsBillingUser(ctx, "u")
	if err != nil {
		t.Errorf("Billing.GetActionsBillingUser returned error: %v", err)
	}

	want := &ActionBilling{
		TotalMinutesUsed:     10,
		TotalPaidMinutesUsed: 0,
		IncludedMinutes:      3000,
		MinutesUsedBreakdown: MinutesUsedBreakdown{
			Ubuntu:  205,
			MacOS:   10,
			Windows: 90,
		},
	}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Billing.GetActionsBillingUser returned %+v, want %+v", hook, want)
	}

	const methodName = "GetActionsBillingUser"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Billing.GetActionsBillingOrg(ctx, "\n")
		return err
	})
}

func TestBillingService_GetActionsBillingUser_invalidUser(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Billing.GetActionsBillingUser(ctx, "%")
	testURLParseError(t, err)
}

func TestBillingService_GetPackagesBillingUser(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/users/u/settings/billing/packages", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
				"total_gigabytes_bandwidth_used": 50,
				"total_paid_gigabytes_bandwidth_used": 40,
				"included_gigabytes_bandwidth": 10
			}`)
	})

	ctx := context.Background()
	hook, _, err := client.Billing.GetPackagesBillingUser(ctx, "u")
	if err != nil {
		t.Errorf("Billing.GetPackagesBillingUser returned error: %v", err)
	}

	want := &PackageBilling{
		TotalGigabytesBandwidthUsed:     50,
		TotalPaidGigabytesBandwidthUsed: 40,
		IncludedGigabytesBandwidth:      10,
	}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Billing.GetPackagesBillingUser returned %+v, want %+v", hook, want)
	}

	const methodName = "GetPackagesBillingUser"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Billing.GetPackagesBillingUser(ctx, "\n")
		return err
	})
}

func TestBillingService_GetPackagesBillingUser_invalidUser(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Billing.GetPackagesBillingUser(ctx, "%")
	testURLParseError(t, err)
}

func TestBillingService_GetStorageBillingUser(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/users/u/settings/billing/shared-storage", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
				"days_left_in_billing_cycle": 20,
				"estimated_paid_storage_for_month": 15,
				"estimated_storage_for_month": 40
			}`)
	})

	ctx := context.Background()
	hook, _, err := client.Billing.GetStorageBillingUser(ctx, "u")
	if err != nil {
		t.Errorf("Billing.GetStorageBillingUser returned error: %v", err)
	}

	want := &StorageBilling{
		DaysLeftInBillingCycle:       20,
		EstimatedPaidStorageForMonth: 15,
		EstimatedStorageForMonth:     40,
	}
	if !reflect.DeepEqual(hook, want) {
		t.Errorf("Billing.GetStorageBillingUser returned %+v, want %+v", hook, want)
	}

	const methodName = "GetStorageBillingUser"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Billing.GetStorageBillingUser(ctx, "\n")
		return err
	})
}

func TestBillingService_GetStorageBillingUser_invalidUser(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.Billing.GetStorageBillingUser(ctx, "%")
	testURLParseError(t, err)
}
