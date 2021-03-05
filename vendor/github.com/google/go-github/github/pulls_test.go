// Copyright 2013 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestPullRequestsService_List(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"state":     "closed",
			"head":      "h",
			"base":      "b",
			"sort":      "created",
			"direction": "desc",
			"page":      "2",
		})
		fmt.Fprint(w, `[{"number":1}]`)
	})

	opts := &PullRequestListOptions{"closed", "h", "b", "created", "desc", ListOptions{Page: 2}}
	ctx := context.Background()
	pulls, _, err := client.PullRequests.List(ctx, "o", "r", opts)
	if err != nil {
		t.Errorf("PullRequests.List returned error: %v", err)
	}

	want := []*PullRequest{{Number: Int(1)}}
	if !reflect.DeepEqual(pulls, want) {
		t.Errorf("PullRequests.List returned %+v, want %+v", pulls, want)
	}

	const methodName = "List"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.List(ctx, "\n", "\n", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.List(ctx, "o", "r", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_ListPullRequestsWithCommit(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/commits/sha/pulls", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", mediaTypeListPullsOrBranchesForCommitPreview)
		testFormValues(t, r, values{
			"state":     "closed",
			"head":      "h",
			"base":      "b",
			"sort":      "created",
			"direction": "desc",
			"page":      "2",
		})
		fmt.Fprint(w, `[{"number":1}]`)
	})

	opts := &PullRequestListOptions{"closed", "h", "b", "created", "desc", ListOptions{Page: 2}}
	ctx := context.Background()
	pulls, _, err := client.PullRequests.ListPullRequestsWithCommit(ctx, "o", "r", "sha", opts)
	if err != nil {
		t.Errorf("PullRequests.ListPullRequestsWithCommit returned error: %v", err)
	}

	want := []*PullRequest{{Number: Int(1)}}
	if !reflect.DeepEqual(pulls, want) {
		t.Errorf("PullRequests.ListPullRequestsWithCommit returned %+v, want %+v", pulls, want)
	}

	const methodName = "ListPullRequestsWithCommit"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.ListPullRequestsWithCommit(ctx, "\n", "\n", "\n", opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.ListPullRequestsWithCommit(ctx, "o", "r", "sha", opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_List_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.PullRequests.List(ctx, "%", "r", nil)
	testURLParseError(t, err)
}

func TestPullRequestsService_Get(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"number":1}`)
	})

	ctx := context.Background()
	pull, _, err := client.PullRequests.Get(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("PullRequests.Get returned error: %v", err)
	}

	want := &PullRequest{Number: Int(1)}
	if !reflect.DeepEqual(pull, want) {
		t.Errorf("PullRequests.Get returned %+v, want %+v", pull, want)
	}

	const methodName = "Get"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.Get(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.Get(ctx, "o", "r", 1)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_GetRaw_diff(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	const rawStr = "@@diff content"

	mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", mediaTypeV3Diff)
		fmt.Fprint(w, rawStr)
	})

	ctx := context.Background()
	got, _, err := client.PullRequests.GetRaw(ctx, "o", "r", 1, RawOptions{Diff})
	if err != nil {
		t.Fatalf("PullRequests.GetRaw returned error: %v", err)
	}
	want := rawStr
	if got != want {
		t.Errorf("PullRequests.GetRaw returned %s want %s", got, want)
	}

	const methodName = "GetRaw"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.GetRaw(ctx, "\n", "\n", -1, RawOptions{Diff})
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.GetRaw(ctx, "o", "r", 1, RawOptions{Diff})
		if got != "" {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_GetRaw_patch(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	const rawStr = "@@patch content"

	mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testHeader(t, r, "Accept", mediaTypeV3Patch)
		fmt.Fprint(w, rawStr)
	})

	ctx := context.Background()
	got, _, err := client.PullRequests.GetRaw(ctx, "o", "r", 1, RawOptions{Patch})
	if err != nil {
		t.Fatalf("PullRequests.GetRaw returned error: %v", err)
	}
	want := rawStr
	if got != want {
		t.Errorf("PullRequests.GetRaw returned %s want %s", got, want)
	}
}

func TestPullRequestsService_GetRaw_invalid(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.PullRequests.GetRaw(ctx, "o", "r", 1, RawOptions{100})
	if err == nil {
		t.Fatal("PullRequests.GetRaw should return error")
	}
	if !strings.Contains(err.Error(), "unsupported raw type") {
		t.Error("PullRequests.GetRaw should return unsupported raw type error")
	}
}

func TestPullRequestsService_Get_links(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{
			"number":1,
			"_links":{
				"self":{"href":"https://api.github.com/repos/octocat/Hello-World/pulls/1347"},
				"html":{"href":"https://github.com/octocat/Hello-World/pull/1347"},
				"issue":{"href":"https://api.github.com/repos/octocat/Hello-World/issues/1347"},
				"comments":{"href":"https://api.github.com/repos/octocat/Hello-World/issues/1347/comments"},
				"review_comments":{"href":"https://api.github.com/repos/octocat/Hello-World/pulls/1347/comments"},
				"review_comment":{"href":"https://api.github.com/repos/octocat/Hello-World/pulls/comments{/number}"},
				"commits":{"href":"https://api.github.com/repos/octocat/Hello-World/pulls/1347/commits"},
				"statuses":{"href":"https://api.github.com/repos/octocat/Hello-World/statuses/6dcb09b5b57875f334f61aebed695e2e4193db5e"}
				}
			}`)
	})

	ctx := context.Background()
	pull, _, err := client.PullRequests.Get(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("PullRequests.Get returned error: %v", err)
	}

	want := &PullRequest{
		Number: Int(1),
		Links: &PRLinks{
			Self: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/pulls/1347"),
			}, HTML: &PRLink{
				HRef: String("https://github.com/octocat/Hello-World/pull/1347"),
			}, Issue: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/issues/1347"),
			}, Comments: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/issues/1347/comments"),
			}, ReviewComments: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/pulls/1347/comments"),
			}, ReviewComment: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/pulls/comments{/number}"),
			}, Commits: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/pulls/1347/commits"),
			}, Statuses: &PRLink{
				HRef: String("https://api.github.com/repos/octocat/Hello-World/statuses/6dcb09b5b57875f334f61aebed695e2e4193db5e"),
			},
		},
	}
	if !reflect.DeepEqual(pull, want) {
		t.Errorf("PullRequests.Get returned %+v, want %+v", pull, want)
	}
}

func TestPullRequestsService_Get_headAndBase(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"number":1,"head":{"ref":"r2","repo":{"id":2}},"base":{"ref":"r1","repo":{"id":1}}}`)
	})

	ctx := context.Background()
	pull, _, err := client.PullRequests.Get(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("PullRequests.Get returned error: %v", err)
	}

	want := &PullRequest{
		Number: Int(1),
		Head: &PullRequestBranch{
			Ref:  String("r2"),
			Repo: &Repository{ID: Int64(2)},
		},
		Base: &PullRequestBranch{
			Ref:  String("r1"),
			Repo: &Repository{ID: Int64(1)},
		},
	}
	if !reflect.DeepEqual(pull, want) {
		t.Errorf("PullRequests.Get returned %+v, want %+v", pull, want)
	}
}

func TestPullRequestsService_Get_urlFields(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"number":1,
			"url": "https://api.github.com/repos/octocat/Hello-World/pulls/1347",
			"html_url": "https://github.com/octocat/Hello-World/pull/1347",
			"issue_url": "https://api.github.com/repos/octocat/Hello-World/issues/1347",
			"statuses_url": "https://api.github.com/repos/octocat/Hello-World/statuses/6dcb09b5b57875f334f61aebed695e2e4193db5e",
			"diff_url": "https://github.com/octocat/Hello-World/pull/1347.diff",
			"patch_url": "https://github.com/octocat/Hello-World/pull/1347.patch",
			"review_comments_url": "https://api.github.com/repos/octocat/Hello-World/pulls/1347/comments",
			"review_comment_url": "https://api.github.com/repos/octocat/Hello-World/pulls/comments{/number}"}`)
	})

	ctx := context.Background()
	pull, _, err := client.PullRequests.Get(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("PullRequests.Get returned error: %v", err)
	}

	want := &PullRequest{
		Number:            Int(1),
		URL:               String("https://api.github.com/repos/octocat/Hello-World/pulls/1347"),
		HTMLURL:           String("https://github.com/octocat/Hello-World/pull/1347"),
		IssueURL:          String("https://api.github.com/repos/octocat/Hello-World/issues/1347"),
		StatusesURL:       String("https://api.github.com/repos/octocat/Hello-World/statuses/6dcb09b5b57875f334f61aebed695e2e4193db5e"),
		DiffURL:           String("https://github.com/octocat/Hello-World/pull/1347.diff"),
		PatchURL:          String("https://github.com/octocat/Hello-World/pull/1347.patch"),
		ReviewCommentsURL: String("https://api.github.com/repos/octocat/Hello-World/pulls/1347/comments"),
		ReviewCommentURL:  String("https://api.github.com/repos/octocat/Hello-World/pulls/comments{/number}"),
	}

	if !reflect.DeepEqual(pull, want) {
		t.Errorf("PullRequests.Get returned %+v, want %+v", pull, want)
	}
}

func TestPullRequestsService_Get_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.PullRequests.Get(ctx, "%", "r", 1)
	testURLParseError(t, err)
}

func TestPullRequestsService_Create(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := &NewPullRequest{Title: String("t")}

	mux.HandleFunc("/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		v := new(NewPullRequest)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		if !reflect.DeepEqual(v, input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":1}`)
	})

	ctx := context.Background()
	pull, _, err := client.PullRequests.Create(ctx, "o", "r", input)
	if err != nil {
		t.Errorf("PullRequests.Create returned error: %v", err)
	}

	want := &PullRequest{Number: Int(1)}
	if !reflect.DeepEqual(pull, want) {
		t.Errorf("PullRequests.Create returned %+v, want %+v", pull, want)
	}

	const methodName = "Create"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.Create(ctx, "\n", "\n", input)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.Create(ctx, "o", "r", input)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_Create_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.PullRequests.Create(ctx, "%", "r", nil)
	testURLParseError(t, err)
}

func TestPullRequestsService_UpdateBranch(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1/update-branch", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		testHeader(t, r, "Accept", mediaTypeUpdatePullRequestBranchPreview)
		fmt.Fprint(w, `
			{
			  "message": "Updating pull request branch.",
			  "url": "https://github.com/repos/o/r/pulls/1"
			}`)
	})

	opts := &PullRequestBranchUpdateOptions{
		ExpectedHeadSHA: String("s"),
	}

	ctx := context.Background()
	pull, _, err := client.PullRequests.UpdateBranch(ctx, "o", "r", 1, opts)
	if err != nil {
		t.Errorf("PullRequests.UpdateBranch returned error: %v", err)
	}

	want := &PullRequestBranchUpdateResponse{
		Message: String("Updating pull request branch."),
		URL:     String("https://github.com/repos/o/r/pulls/1"),
	}

	if !reflect.DeepEqual(pull, want) {
		t.Errorf("PullRequests.UpdateBranch returned %+v, want %+v", pull, want)
	}

	const methodName = "UpdateBranch"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.UpdateBranch(ctx, "\n", "\n", -1, opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.UpdateBranch(ctx, "o", "r", 1, opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_Edit(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	tests := []struct {
		input        *PullRequest
		sendResponse string

		wantUpdate string
		want       *PullRequest
	}{
		{
			input:        &PullRequest{Title: String("t")},
			sendResponse: `{"number":1}`,
			wantUpdate:   `{"title":"t"}`,
			want:         &PullRequest{Number: Int(1)},
		},
		{
			// base update
			input:        &PullRequest{Base: &PullRequestBranch{Ref: String("master")}},
			sendResponse: `{"number":1,"base":{"ref":"master"}}`,
			wantUpdate:   `{"base":"master"}`,
			want: &PullRequest{
				Number: Int(1),
				Base:   &PullRequestBranch{Ref: String("master")},
			},
		},
	}

	for i, tt := range tests {
		madeRequest := false
		mux.HandleFunc(fmt.Sprintf("/repos/o/r/pulls/%v", i), func(w http.ResponseWriter, r *http.Request) {
			testMethod(t, r, "PATCH")
			testBody(t, r, tt.wantUpdate+"\n")
			io.WriteString(w, tt.sendResponse)
			madeRequest = true
		})

		ctx := context.Background()
		pull, _, err := client.PullRequests.Edit(ctx, "o", "r", i, tt.input)
		if err != nil {
			t.Errorf("%d: PullRequests.Edit returned error: %v", i, err)
		}

		if !reflect.DeepEqual(pull, tt.want) {
			t.Errorf("%d: PullRequests.Edit returned %+v, want %+v", i, pull, tt.want)
		}

		if !madeRequest {
			t.Errorf("%d: PullRequest.Edit did not make the expected request", i)
		}

		const methodName = "Edit"
		testBadOptions(t, methodName, func() (err error) {
			_, _, err = client.PullRequests.Edit(ctx, "\n", "\n", -i, tt.input)
			return err
		})
	}
}

func TestPullRequestsService_Edit_invalidOwner(t *testing.T) {
	client, _, _, teardown := setup()
	defer teardown()

	ctx := context.Background()
	_, _, err := client.PullRequests.Edit(ctx, "%", "r", 1, &PullRequest{})
	testURLParseError(t, err)
}

func TestPullRequestsService_ListCommits(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1/commits", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"page": "2"})
		fmt.Fprint(w, `
			[
			  {
			    "sha": "3",
			    "parents": [
			      {
			        "sha": "2"
			      }
			    ]
			  },
			  {
			    "sha": "2",
			    "parents": [
			      {
			        "sha": "1"
			      }
			    ]
			  }
			]`)
	})

	opts := &ListOptions{Page: 2}
	ctx := context.Background()
	commits, _, err := client.PullRequests.ListCommits(ctx, "o", "r", 1, opts)
	if err != nil {
		t.Errorf("PullRequests.ListCommits returned error: %v", err)
	}

	want := []*RepositoryCommit{
		{
			SHA: String("3"),
			Parents: []*Commit{
				{
					SHA: String("2"),
				},
			},
		},
		{
			SHA: String("2"),
			Parents: []*Commit{
				{
					SHA: String("1"),
				},
			},
		},
	}
	if !reflect.DeepEqual(commits, want) {
		t.Errorf("PullRequests.ListCommits returned %+v, want %+v", commits, want)
	}

	const methodName = "ListCommits"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.ListCommits(ctx, "\n", "\n", -1, opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.ListCommits(ctx, "o", "r", 1, opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_ListFiles(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1/files", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{"page": "2"})
		fmt.Fprint(w, `
			[
			  {
			    "sha": "6dcb09b5b57875f334f61aebed695e2e4193db5e",
			    "filename": "file1.txt",
			    "status": "added",
			    "additions": 103,
			    "deletions": 21,
			    "changes": 124,
			    "patch": "@@ -132,7 +132,7 @@ module Test @@ -1000,7 +1000,7 @@ module Test"
			  },
			  {
			    "sha": "f61aebed695e2e4193db5e6dcb09b5b57875f334",
			    "filename": "file2.txt",
			    "status": "modified",
			    "additions": 5,
			    "deletions": 3,
			    "changes": 103,
			    "patch": "@@ -132,7 +132,7 @@ module Test @@ -1000,7 +1000,7 @@ module Test"
			  }
			]`)
	})

	opts := &ListOptions{Page: 2}
	ctx := context.Background()
	commitFiles, _, err := client.PullRequests.ListFiles(ctx, "o", "r", 1, opts)
	if err != nil {
		t.Errorf("PullRequests.ListFiles returned error: %v", err)
	}

	want := []*CommitFile{
		{
			SHA:       String("6dcb09b5b57875f334f61aebed695e2e4193db5e"),
			Filename:  String("file1.txt"),
			Additions: Int(103),
			Deletions: Int(21),
			Changes:   Int(124),
			Status:    String("added"),
			Patch:     String("@@ -132,7 +132,7 @@ module Test @@ -1000,7 +1000,7 @@ module Test"),
		},
		{
			SHA:       String("f61aebed695e2e4193db5e6dcb09b5b57875f334"),
			Filename:  String("file2.txt"),
			Additions: Int(5),
			Deletions: Int(3),
			Changes:   Int(103),
			Status:    String("modified"),
			Patch:     String("@@ -132,7 +132,7 @@ module Test @@ -1000,7 +1000,7 @@ module Test"),
		},
	}

	if !reflect.DeepEqual(commitFiles, want) {
		t.Errorf("PullRequests.ListFiles returned %+v, want %+v", commitFiles, want)
	}

	const methodName = "ListFiles"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.ListFiles(ctx, "\n", "\n", -1, opts)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.ListFiles(ctx, "o", "r", 1, opts)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_IsMerged(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		w.WriteHeader(http.StatusNoContent)
	})

	ctx := context.Background()
	isMerged, _, err := client.PullRequests.IsMerged(ctx, "o", "r", 1)
	if err != nil {
		t.Errorf("PullRequests.IsMerged returned error: %v", err)
	}

	want := true
	if !reflect.DeepEqual(isMerged, want) {
		t.Errorf("PullRequests.IsMerged returned %+v, want %+v", isMerged, want)
	}

	const methodName = "IsMerged"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.IsMerged(ctx, "\n", "\n", -1)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.IsMerged(ctx, "o", "r", 1)
		if got {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want false", methodName, got)
		}
		return resp, err
	})
}

func TestPullRequestsService_Merge(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		fmt.Fprint(w, `
			{
			  "sha": "6dcb09b5b57875f334f61aebed695e2e4193db5e",
			  "merged": true,
			  "message": "Pull Request successfully merged"
			}`)
	})

	options := &PullRequestOptions{MergeMethod: "rebase"}
	ctx := context.Background()
	merge, _, err := client.PullRequests.Merge(ctx, "o", "r", 1, "merging pull request", options)
	if err != nil {
		t.Errorf("PullRequests.Merge returned error: %v", err)
	}

	want := &PullRequestMergeResult{
		SHA:     String("6dcb09b5b57875f334f61aebed695e2e4193db5e"),
		Merged:  Bool(true),
		Message: String("Pull Request successfully merged"),
	}
	if !reflect.DeepEqual(merge, want) {
		t.Errorf("PullRequests.Merge returned %+v, want %+v", merge, want)
	}

	const methodName = "Merge"
	testBadOptions(t, methodName, func() (err error) {
		_, _, err = client.PullRequests.Merge(ctx, "\n", "\n", -1, "\n", options)
		return err
	})

	testNewRequestAndDoFailure(t, methodName, client, func() (*Response, error) {
		got, resp, err := client.PullRequests.Merge(ctx, "o", "r", 1, "merging pull request", options)
		if got != nil {
			t.Errorf("testNewRequestAndDoFailure %v = %#v, want nil", methodName, got)
		}
		return resp, err
	})
}

// Test that different merge options produce expected PUT requests. See issue https://github.com/google/go-github/issues/500.
func TestPullRequestsService_Merge_options(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	tests := []struct {
		options  *PullRequestOptions
		wantBody string
	}{
		{
			options:  nil,
			wantBody: `{"commit_message":"merging pull request"}`,
		},
		{
			options:  &PullRequestOptions{},
			wantBody: `{"commit_message":"merging pull request"}`,
		},
		{
			options:  &PullRequestOptions{MergeMethod: "rebase"},
			wantBody: `{"commit_message":"merging pull request","merge_method":"rebase"}`,
		},
		{
			options:  &PullRequestOptions{SHA: "6dcb09b5b57875f334f61aebed695e2e4193db5e"},
			wantBody: `{"commit_message":"merging pull request","sha":"6dcb09b5b57875f334f61aebed695e2e4193db5e"}`,
		},
		{
			options: &PullRequestOptions{
				CommitTitle: "Extra detail",
				SHA:         "6dcb09b5b57875f334f61aebed695e2e4193db5e",
				MergeMethod: "squash",
			},
			wantBody: `{"commit_message":"merging pull request","commit_title":"Extra detail","merge_method":"squash","sha":"6dcb09b5b57875f334f61aebed695e2e4193db5e"}`,
		},
		{
			options: &PullRequestOptions{
				DontDefaultIfBlank: true,
			},
			wantBody: `{"commit_message":"merging pull request"}`,
		},
	}

	for i, test := range tests {
		madeRequest := false
		mux.HandleFunc(fmt.Sprintf("/repos/o/r/pulls/%d/merge", i), func(w http.ResponseWriter, r *http.Request) {
			testMethod(t, r, "PUT")
			testBody(t, r, test.wantBody+"\n")
			madeRequest = true
		})
		ctx := context.Background()
		_, _, _ = client.PullRequests.Merge(ctx, "o", "r", i, "merging pull request", test.options)
		if !madeRequest {
			t.Errorf("%d: PullRequests.Merge(%#v): expected request was not made", i, test.options)
		}
	}
}

func TestPullRequestsService_Merge_Blank_Message(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()
	madeRequest := false
	expectedBody := ""
	mux.HandleFunc("/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "PUT")
		testBody(t, r, expectedBody+"\n")
		madeRequest = true
	})

	ctx := context.Background()
	expectedBody = `{}`
	_, _, _ = client.PullRequests.Merge(ctx, "o", "r", 1, "", nil)
	if !madeRequest {
		t.Error("TestPullRequestsService_Merge_Blank_Message #1 did not make request")
	}

	madeRequest = false
	opts := PullRequestOptions{
		DontDefaultIfBlank: true,
	}
	expectedBody = `{"commit_message":""}`
	_, _, _ = client.PullRequests.Merge(ctx, "o", "r", 1, "", &opts)
	if !madeRequest {
		t.Error("TestPullRequestsService_Merge_Blank_Message #2 did not make request")
	}
}
