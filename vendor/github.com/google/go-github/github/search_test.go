// Copyright 2013 The go-github AUTHORS. All rights reserved.
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

func TestSearchService_Repositories(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q":        "blah",
			"sort":     "forks",
			"order":    "desc",
			"page":     "2",
			"per_page": "2",
		})

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": false, "items": [{"id":1},{"id":2}]}`)
	})

	opts := &SearchOptions{Sort: "forks", Order: "desc", ListOptions: ListOptions{Page: 2, PerPage: 2}}
	result, _, err := client.Search.Repositories(context.Background(), "blah", opts)
	if err != nil {
		t.Errorf("Search.Repositories returned error: %v", err)
	}

	want := &RepositoriesSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(false),
		Repositories:      []Repository{{ID: Int64(1)}, {ID: Int64(2)}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Repositories returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Commits(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/commits", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q":     "blah",
			"sort":  "author-date",
			"order": "desc",
		})

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": false, "items": [{"sha":"random_hash1"},{"sha":"random_hash2"}]}`)
	})

	opts := &SearchOptions{Sort: "author-date", Order: "desc"}
	result, _, err := client.Search.Commits(context.Background(), "blah", opts)
	if err != nil {
		t.Errorf("Search.Commits returned error: %v", err)
	}

	want := &CommitsSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(false),
		Commits:           []*CommitResult{{SHA: String("random_hash1")}, {SHA: String("random_hash2")}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Commits returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Issues(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q":        "blah",
			"sort":     "forks",
			"order":    "desc",
			"page":     "2",
			"per_page": "2",
		})

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": true, "items": [{"number":1},{"number":2}]}`)
	})

	opts := &SearchOptions{Sort: "forks", Order: "desc", ListOptions: ListOptions{Page: 2, PerPage: 2}}
	result, _, err := client.Search.Issues(context.Background(), "blah", opts)
	if err != nil {
		t.Errorf("Search.Issues returned error: %v", err)
	}

	want := &IssuesSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(true),
		Issues:            []Issue{{Number: Int(1)}, {Number: Int(2)}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Issues returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Issues_withQualifiersNoOpts(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	const q = "gopher is:issue label:bug language:c++ pushed:>=2018-01-01 stars:>=200"

	var requestURI string
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q": q,
		})
		requestURI = r.RequestURI

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": true, "items": [{"number":1},{"number":2}]}`)
	})

	opts := &SearchOptions{}
	result, _, err := client.Search.Issues(context.Background(), q, opts)
	if err != nil {
		t.Errorf("Search.Issues returned error: %v", err)
	}

	if want := "/api-v3/search/issues?q=gopher+is%3Aissue+label%3Abug+language%3Ac%2B%2B+pushed%3A%3E%3D2018-01-01+stars%3A%3E%3D200"; requestURI != want {
		t.Fatalf("URI encoding failed: got %v, want %v", requestURI, want)
	}

	want := &IssuesSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(true),
		Issues:            []Issue{{Number: Int(1)}, {Number: Int(2)}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Issues returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Issues_withQualifiersAndOpts(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	const q = "gopher is:issue label:bug language:c++ pushed:>=2018-01-01 stars:>=200"

	var requestURI string
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q":    q,
			"sort": "forks",
		})
		requestURI = r.RequestURI

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": true, "items": [{"number":1},{"number":2}]}`)
	})

	opts := &SearchOptions{Sort: "forks"}
	result, _, err := client.Search.Issues(context.Background(), q, opts)
	if err != nil {
		t.Errorf("Search.Issues returned error: %v", err)
	}

	if want := "/api-v3/search/issues?q=gopher+is%3Aissue+label%3Abug+language%3Ac%2B%2B+pushed%3A%3E%3D2018-01-01+stars%3A%3E%3D200&sort=forks"; requestURI != want {
		t.Fatalf("URI encoding failed: got %v, want %v", requestURI, want)
	}

	want := &IssuesSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(true),
		Issues:            []Issue{{Number: Int(1)}, {Number: Int(2)}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Issues returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Users(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/users", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q":        "blah",
			"sort":     "forks",
			"order":    "desc",
			"page":     "2",
			"per_page": "2",
		})

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": false, "items": [{"id":1},{"id":2}]}`)
	})

	opts := &SearchOptions{Sort: "forks", Order: "desc", ListOptions: ListOptions{Page: 2, PerPage: 2}}
	result, _, err := client.Search.Users(context.Background(), "blah", opts)
	if err != nil {
		t.Errorf("Search.Issues returned error: %v", err)
	}

	want := &UsersSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(false),
		Users:             []User{{ID: Int64(1)}, {ID: Int64(2)}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Users returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Code(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"q":        "blah",
			"sort":     "forks",
			"order":    "desc",
			"page":     "2",
			"per_page": "2",
		})

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": false, "items": [{"name":"1"},{"name":"2"}]}`)
	})

	opts := &SearchOptions{Sort: "forks", Order: "desc", ListOptions: ListOptions{Page: 2, PerPage: 2}}
	result, _, err := client.Search.Code(context.Background(), "blah", opts)
	if err != nil {
		t.Errorf("Search.Code returned error: %v", err)
	}

	want := &CodeSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(false),
		CodeResults:       []CodeResult{{Name: String("1")}, {Name: String("2")}},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Code returned %+v, want %+v", result, want)
	}
}

func TestSearchService_CodeTextMatch(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")

		textMatchResponse := `
		{
			"total_count": 1,
			"incomplete_results": false,
			"items": [
				{
					"name":"gopher1",
					"text_matches": [
						{
							"fragment": "I'm afraid my friend what you have found\nIs a gopher who lives to feed",
							"matches": [
								{
									"text": "gopher",
									"indices": [
										14,
										21
							  	]
								}
						  ]
					  }
				  ]
				}
			]
		}
    `

		fmt.Fprint(w, textMatchResponse)
	})

	opts := &SearchOptions{Sort: "forks", Order: "desc", ListOptions: ListOptions{Page: 2, PerPage: 2}, TextMatch: true}
	result, _, err := client.Search.Code(context.Background(), "blah", opts)
	if err != nil {
		t.Errorf("Search.Code returned error: %v", err)
	}

	wantedCodeResult := CodeResult{
		Name: String("gopher1"),
		TextMatches: []TextMatch{{
			Fragment: String("I'm afraid my friend what you have found\nIs a gopher who lives to feed"),
			Matches:  []Match{{Text: String("gopher"), Indices: []int{14, 21}}},
		},
		},
	}

	want := &CodeSearchResult{
		Total:             Int(1),
		IncompleteResults: Bool(false),
		CodeResults:       []CodeResult{wantedCodeResult},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Code returned %+v, want %+v", result, want)
	}
}

func TestSearchService_Labels(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/search/labels", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"repository_id": "1234",
			"q":             "blah",
			"sort":          "updated",
			"order":         "desc",
			"page":          "2",
			"per_page":      "2",
		})

		fmt.Fprint(w, `{"total_count": 4, "incomplete_results": false, "items": [{"id": 1234, "name":"bug", "description": "some text"},{"id": 4567, "name":"feature"}]}`)
	})

	opts := &SearchOptions{Sort: "updated", Order: "desc", ListOptions: ListOptions{Page: 2, PerPage: 2}}
	result, _, err := client.Search.Labels(context.Background(), 1234, "blah", opts)
	if err != nil {
		t.Errorf("Search.Code returned error: %v", err)
	}

	want := &LabelsSearchResult{
		Total:             Int(4),
		IncompleteResults: Bool(false),
		Labels: []*LabelResult{
			{ID: Int64(1234), Name: String("bug"), Description: String("some text")},
			{ID: Int64(4567), Name: String("feature")},
		},
	}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("Search.Labels returned %+v, want %+v", result, want)
	}
}
