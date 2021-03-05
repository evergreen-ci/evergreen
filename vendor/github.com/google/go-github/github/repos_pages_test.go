// Copyright 2014 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"
)

func TestRepositoriesService_EnablePages(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &Pages{
		Source: &PagesSource{
			Branch: String("master"),
			Path:   String("/"),
		},
		CNAME: String("www.my-domain.com"), // not passed along.
	}

	mux.HandleFunc("/repos/o/r/pages", func(w http.ResponseWriter, r *http.Request) {
		v := new(createPagesRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		testHeader(t, r, "Accept", mediaTypeEnablePagesAPIPreview)
		want := &createPagesRequest{Source: &PagesSource{Branch: String("master"), Path: String("/")}}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Request body = %+v, want %+v", v, want)
		}

		fmt.Fprint(w, `{"url":"u","status":"s","cname":"c","custom_404":false,"html_url":"h", "source": {"branch":"master", "path":"/"}}`)
	})

	ctx := context.Background()
	page, _, err := client.Repositories.EnablePages(ctx, "o", "r", input)
	if err != nil {
		t.Errorf("Repositories.EnablePages returned error: %v", err)
	}

	want := &Pages{URL: String("u"), Status: String("s"), CNAME: String("c"), Custom404: Bool(false), HTMLURL: String("h"), Source: &PagesSource{Branch: String("master"), Path: String("/")}}

	if !reflect.DeepEqual(page, want) {
		t.Errorf("Repositories.EnablePages returned %v, want %v", page, want)
	}

	const methodName = "EnablePages"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.EnablePages(ctx, "\n", "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.EnablePages(ctx, "o", "r", input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_UpdatePages(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &PagesUpdate{
		CNAME:  String("www.my-domain.com"),
		Source: String("gh-pages"),
	}

	mux.HandleFunc("/repos/o/r/pages", func(w http.ResponseWriter, r *http.Request) {
		v := new(PagesUpdate)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PUT")
		want := &PagesUpdate{CNAME: String("www.my-domain.com"), Source: String("gh-pages")}
		if !reflect.DeepEqual(v, want) {
			t.Errorf("Request body = %+v, want %+v", v, want)
		}

		fmt.Fprint(w, `{"cname":"www.my-domain.com","source":"gh-pages"}`)
	})

	ctx := context.Background()
	_, err := client.Repositories.UpdatePages(ctx, "o", "r", input)
	if err != nil {
		t.Errorf("Repositories.UpdatePages returned error: %v", err)
	}

	const methodName = "UpdatePages"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Repositories.UpdatePages(ctx, "\n", "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Repositories.UpdatePages(ctx, "o", "r", input)
	})
}

func TestRepositoriesService_UpdatePages_NullCNAME(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &PagesUpdate{
		Source: String("gh-pages"),
	}

	mux.HandleFunc("/repos/o/r/pages", func(w http.ResponseWriter, r *http.Request) {
		got, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("unable to read body: %v", err)
		}

		want := []byte(`{"cname":null,"source":"gh-pages"}` + "\n")
		if !bytes.Equal(got, want) {
			t.Errorf("Request body = %+v, want %+v", got, want)
		}

		fmt.Fprint(w, `{"cname":null,"source":"gh-pages"}`)
	})

	ctx := context.Background()
	_, err := client.Repositories.UpdatePages(ctx, "o", "r", input)
	if err != nil {
		t.Errorf("Repositories.UpdatePages returned error: %v", err)
	}
}

func TestRepositoriesService_DisablePages(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
		testHeader(t, r, "Accept", mediaTypeEnablePagesAPIPreview)
	})

	ctx := context.Background()
	_, err := client.Repositories.DisablePages(ctx, "o", "r")
	if err != nil {
		t.Errorf("Repositories.DisablePages returned error: %v", err)
	}

	const methodName = "DisablePages"
	testBadOptions(t, methodName, func() (err error) {
		_, err = client.Repositories.DisablePages(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		return client.Repositories.DisablePages(ctx, "o", "r")
	})
}

func TestRepositoriesService_GetPagesInfo(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"url":"u","status":"s","cname":"c","custom_404":false,"html_url":"h"}`)
	})

	ctx := context.Background()
	page, _, err := client.Repositories.GetPagesInfo(ctx, "o", "r")
	if err != nil {
		t.Errorf("Repositories.GetPagesInfo returned error: %v", err)
	}

	want := &Pages{URL: String("u"), Status: String("s"), CNAME: String("c"), Custom404: Bool(false), HTMLURL: String("h")}
	if !reflect.DeepEqual(page, want) {
		t.Errorf("Repositories.GetPagesInfo returned %+v, want %+v", page, want)
	}

	const methodName = "GetPagesInfo"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetPagesInfo(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetPagesInfo(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListPagesBuilds(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages/builds", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `[{"url":"u","status":"s","commit":"c"}]`)
	})

	ctx := context.Background()
	pages, _, err := client.Repositories.ListPagesBuilds(ctx, "o", "r", nil)
	if err != nil {
		t.Errorf("Repositories.ListPagesBuilds returned error: %v", err)
	}

	want := []*PagesBuild{{URL: String("u"), Status: String("s"), Commit: String("c")}}
	if !reflect.DeepEqual(pages, want) {
		t.Errorf("Repositories.ListPagesBuilds returned %+v, want %+v", pages, want)
	}

	const methodName = "ListPagesBuilds"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.ListPagesBuilds(ctx, "\n", "\n", nil)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.ListPagesBuilds(ctx, "o", "r", nil)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_ListPagesBuilds_withOptions(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages/builds", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"page": "2",
		})
		fmt.Fprint(w, `[]`)
	})

	ctx := context.Background()
	_, _, err := client.Repositories.ListPagesBuilds(ctx, "o", "r", &ListOptions{Page: 2})
	if err != nil {
		t.Errorf("Repositories.ListPagesBuilds returned error: %v", err)
	}
}

func TestRepositoriesService_GetLatestPagesBuild(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages/builds/latest", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"url":"u","status":"s","commit":"c"}`)
	})

	ctx := context.Background()
	build, _, err := client.Repositories.GetLatestPagesBuild(ctx, "o", "r")
	if err != nil {
		t.Errorf("Repositories.GetLatestPagesBuild returned error: %v", err)
	}

	want := &PagesBuild{URL: String("u"), Status: String("s"), Commit: String("c")}
	if !reflect.DeepEqual(build, want) {
		t.Errorf("Repositories.GetLatestPagesBuild returned %+v, want %+v", build, want)
	}

	const methodName = "GetLatestPagesBuild"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetLatestPagesBuild(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetLatestPagesBuild(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_GetPageBuild(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages/builds/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"url":"u","status":"s","commit":"c"}`)
	})

	ctx := context.Background()
	build, _, err := client.Repositories.GetPageBuild(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("Repositories.GetPageBuild returned error: %v", err)
	}

	want := &PagesBuild{URL: String("u"), Status: String("s"), Commit: String("c")}
	if !reflect.DeepEqual(build, want) {
		t.Errorf("Repositories.GetPageBuild returned %+v, want %+v", build, want)
	}

	const methodName = "GetPageBuild"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.GetPageBuild(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.GetPageBuild(ctx, "o", "r", 1)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestRepositoriesService_RequestPageBuild(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pages/builds", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "POST")
		fmt.Fprint(w, `{"url":"u","status":"s"}`)
	})

	ctx := context.Background()
	build, _, err := client.Repositories.RequestPageBuild(ctx, "o", "r")
	if err != nil {
		t.Errorf("Repositories.RequestPageBuild returned error: %v", err)
	}

	want := &PagesBuild{URL: String("u"), Status: String("s")}
	if !reflect.DeepEqual(build, want) {
		t.Errorf("Repositories.RequestPageBuild returned %+v, want %+v", build, want)
	}

	const methodName = "RequestPageBuild"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.Repositories.RequestPageBuild(ctx, "\n", "\n")
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.Repositories.RequestPageBuild(ctx, "o", "r")
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}
