// Copyright 2018 The go-github AUTHORS. All rights reserved.
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

func TestTeamsService_ListDiscussionsByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/organizations/1/team/2/discussions", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"direction": "desc",
			"page":      "2",
		})
		fmt.Fprintf(w,
			`[
				{
					"author": {
						"login": "author",
						"id": 0,
						"avatar_url": "https://avatars1.githubusercontent.com/u/0?v=4",
						"gravatar_id": "",
						"url": "https://api.github.com/users/author",
						"html_url": "https://github.com/author",
						"followers_url": "https://api.github.com/users/author/followers",
						"following_url": "https://api.github.com/users/author/following{/other_user}",
						"gists_url": "https://api.github.com/users/author/gists{/gist_id}",
						"starred_url": "https://api.github.com/users/author/starred{/owner}{/repo}",
						"subscriptions_url": "https://api.github.com/users/author/subscriptions",
						"organizations_url": "https://api.github.com/users/author/orgs",
						"repos_url": "https://api.github.com/users/author/repos",
						"events_url": "https://api.github.com/users/author/events{/privacy}",
						"received_events_url": "https://api.github.com/users/author/received_events",
						"type": "User",
						"site_admin": false
					},
					"body": "test",
					"body_html": "<p>test</p>",
					"body_version": "version",
					"comments_count": 1,
					"comments_url": "https://api.github.com/teams/2/discussions/3/comments",
					"created_at": "2018-01-01T00:00:00Z",
					"last_edited_at": null,
					"html_url": "https://github.com/orgs/1/teams/2/discussions/3",
					"node_id": "node",
					"number": 3,
					"pinned": false,
					"private": false,
					"team_url": "https://api.github.com/teams/2",
					"title": "test",
					"updated_at": "2018-01-01T00:00:00Z",
					"url": "https://api.github.com/teams/2/discussions/3"
				}
			]`)
	})
	discussions, _, err := client.Teams.ListDiscussionsByID(context.Background(), 1, 2, &DiscussionListOptions{"desc", ListOptions{Page: 2}})
	if err != nil {
		t.Errorf("Teams.ListDiscussionsByID returned error: %v", err)
	}

	want := []*TeamDiscussion{
		{
			Author: &User{
				Login:             String("author"),
				ID:                Int64(0),
				AvatarURL:         String("https://avatars1.githubusercontent.com/u/0?v=4"),
				GravatarID:        String(""),
				URL:               String("https://api.github.com/users/author"),
				HTMLURL:           String("https://github.com/author"),
				FollowersURL:      String("https://api.github.com/users/author/followers"),
				FollowingURL:      String("https://api.github.com/users/author/following{/other_user}"),
				GistsURL:          String("https://api.github.com/users/author/gists{/gist_id}"),
				StarredURL:        String("https://api.github.com/users/author/starred{/owner}{/repo}"),
				SubscriptionsURL:  String("https://api.github.com/users/author/subscriptions"),
				OrganizationsURL:  String("https://api.github.com/users/author/orgs"),
				ReposURL:          String("https://api.github.com/users/author/repos"),
				EventsURL:         String("https://api.github.com/users/author/events{/privacy}"),
				ReceivedEventsURL: String("https://api.github.com/users/author/received_events"),
				Type:              String("User"),
				SiteAdmin:         Bool(false),
			},
			Body:          String("test"),
			BodyHTML:      String("<p>test</p>"),
			BodyVersion:   String("version"),
			CommentsCount: Int(1),
			CommentsURL:   String("https://api.github.com/teams/2/discussions/3/comments"),
			CreatedAt:     &Timestamp{time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)},
			LastEditedAt:  nil,
			HTMLURL:       String("https://github.com/orgs/1/teams/2/discussions/3"),
			NodeID:        String("node"),
			Number:        Int(3),
			Pinned:        Bool(false),
			Private:       Bool(false),
			TeamURL:       String("https://api.github.com/teams/2"),
			Title:         String("test"),
			UpdatedAt:     &Timestamp{time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)},
			URL:           String("https://api.github.com/teams/2/discussions/3"),
		},
	}
	if !reflect.DeepEqual(discussions, want) {
		t.Errorf("Teams.ListDiscussionsByID returned %+v, want %+v", discussions, want)
	}
}

func TestTeamsService_ListDiscussionsBySlug(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/teams/s/discussions", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testFormValues(t, r, values{
			"direction": "desc",
			"page":      "2",
		})
		fmt.Fprintf(w,
			`[
				{
					"author": {
						"login": "author",
						"id": 0,
						"avatar_url": "https://avatars1.githubusercontent.com/u/0?v=4",
						"gravatar_id": "",
						"url": "https://api.github.com/users/author",
						"html_url": "https://github.com/author",
						"followers_url": "https://api.github.com/users/author/followers",
						"following_url": "https://api.github.com/users/author/following{/other_user}",
						"gists_url": "https://api.github.com/users/author/gists{/gist_id}",
						"starred_url": "https://api.github.com/users/author/starred{/owner}{/repo}",
						"subscriptions_url": "https://api.github.com/users/author/subscriptions",
						"organizations_url": "https://api.github.com/users/author/orgs",
						"repos_url": "https://api.github.com/users/author/repos",
						"events_url": "https://api.github.com/users/author/events{/privacy}",
						"received_events_url": "https://api.github.com/users/author/received_events",
						"type": "User",
						"site_admin": false
					},
					"body": "test",
					"body_html": "<p>test</p>",
					"body_version": "version",
					"comments_count": 1,
					"comments_url": "https://api.github.com/teams/2/discussions/3/comments",
					"created_at": "2018-01-01T00:00:00Z",
					"last_edited_at": null,
					"html_url": "https://github.com/orgs/1/teams/2/discussions/3",
					"node_id": "node",
					"number": 3,
					"pinned": false,
					"private": false,
					"team_url": "https://api.github.com/teams/2",
					"title": "test",
					"updated_at": "2018-01-01T00:00:00Z",
					"url": "https://api.github.com/teams/2/discussions/3"
				}
			]`)
	})
	discussions, _, err := client.Teams.ListDiscussionsBySlug(context.Background(), "o", "s", &DiscussionListOptions{"desc", ListOptions{Page: 2}})
	if err != nil {
		t.Errorf("Teams.ListDiscussionsBySlug returned error: %v", err)
	}

	want := []*TeamDiscussion{
		{
			Author: &User{
				Login:             String("author"),
				ID:                Int64(0),
				AvatarURL:         String("https://avatars1.githubusercontent.com/u/0?v=4"),
				GravatarID:        String(""),
				URL:               String("https://api.github.com/users/author"),
				HTMLURL:           String("https://github.com/author"),
				FollowersURL:      String("https://api.github.com/users/author/followers"),
				FollowingURL:      String("https://api.github.com/users/author/following{/other_user}"),
				GistsURL:          String("https://api.github.com/users/author/gists{/gist_id}"),
				StarredURL:        String("https://api.github.com/users/author/starred{/owner}{/repo}"),
				SubscriptionsURL:  String("https://api.github.com/users/author/subscriptions"),
				OrganizationsURL:  String("https://api.github.com/users/author/orgs"),
				ReposURL:          String("https://api.github.com/users/author/repos"),
				EventsURL:         String("https://api.github.com/users/author/events{/privacy}"),
				ReceivedEventsURL: String("https://api.github.com/users/author/received_events"),
				Type:              String("User"),
				SiteAdmin:         Bool(false),
			},
			Body:          String("test"),
			BodyHTML:      String("<p>test</p>"),
			BodyVersion:   String("version"),
			CommentsCount: Int(1),
			CommentsURL:   String("https://api.github.com/teams/2/discussions/3/comments"),
			CreatedAt:     &Timestamp{time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)},
			LastEditedAt:  nil,
			HTMLURL:       String("https://github.com/orgs/1/teams/2/discussions/3"),
			NodeID:        String("node"),
			Number:        Int(3),
			Pinned:        Bool(false),
			Private:       Bool(false),
			TeamURL:       String("https://api.github.com/teams/2"),
			Title:         String("test"),
			UpdatedAt:     &Timestamp{time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)},
			URL:           String("https://api.github.com/teams/2/discussions/3"),
		},
	}
	if !reflect.DeepEqual(discussions, want) {
		t.Errorf("Teams.ListDiscussionsBySlug returned %+v, want %+v", discussions, want)
	}
}

func TestTeamsService_GetDiscussionByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/organizations/1/team/2/discussions/3", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"number":3}`)
	})

	discussion, _, err := client.Teams.GetDiscussionByID(context.Background(), 1, 2, 3)
	if err != nil {
		t.Errorf("Teams.GetDiscussionByID returned error: %v", err)
	}

	want := &TeamDiscussion{Number: Int(3)}
	if !reflect.DeepEqual(discussion, want) {
		t.Errorf("Teams.GetDiscussionByID returned %+v, want %+v", discussion, want)
	}
}

func TestTeamsService_GetDiscussionBySlug(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/teams/s/discussions/3", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"number":3}`)
	})

	discussion, _, err := client.Teams.GetDiscussionBySlug(context.Background(), "o", "s", 3)
	if err != nil {
		t.Errorf("Teams.GetDiscussionBySlug returned error: %v", err)
	}

	want := &TeamDiscussion{Number: Int(3)}
	if !reflect.DeepEqual(discussion, want) {
		t.Errorf("Teams.GetDiscussionBySlug returned %+v, want %+v", discussion, want)
	}
}

func TestTeamsService_CreateDiscussionByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := TeamDiscussion{Title: String("c_t"), Body: String("c_b")}

	mux.HandleFunc("/organizations/1/team/2/discussions", func(w http.ResponseWriter, r *http.Request) {
		v := new(TeamDiscussion)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		if !reflect.DeepEqual(v, &input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":3}`)
	})

	comment, _, err := client.Teams.CreateDiscussionByID(context.Background(), 1, 2, input)
	if err != nil {
		t.Errorf("Teams.CreateDiscussionByID returned error: %v", err)
	}

	want := &TeamDiscussion{Number: Int(3)}
	if !reflect.DeepEqual(comment, want) {
		t.Errorf("Teams.CreateDiscussionByID returned %+v, want %+v", comment, want)
	}
}

func TestTeamsService_CreateDiscussionBySlug(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := TeamDiscussion{Title: String("c_t"), Body: String("c_b")}

	mux.HandleFunc("/orgs/o/teams/s/discussions", func(w http.ResponseWriter, r *http.Request) {
		v := new(TeamDiscussion)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "POST")
		if !reflect.DeepEqual(v, &input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":3}`)
	})

	comment, _, err := client.Teams.CreateDiscussionBySlug(context.Background(), "o", "s", input)
	if err != nil {
		t.Errorf("Teams.CreateDiscussionBySlug returned error: %v", err)
	}

	want := &TeamDiscussion{Number: Int(3)}
	if !reflect.DeepEqual(comment, want) {
		t.Errorf("Teams.CreateDiscussionBySlug returned %+v, want %+v", comment, want)
	}
}

func TestTeamsService_EditDiscussionByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := TeamDiscussion{Title: String("e_t"), Body: String("e_b")}

	mux.HandleFunc("/organizations/1/team/2/discussions/3", func(w http.ResponseWriter, r *http.Request) {
		v := new(TeamDiscussion)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PATCH")
		if !reflect.DeepEqual(v, &input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":3}`)
	})

	comment, _, err := client.Teams.EditDiscussionByID(context.Background(), 1, 2, 3, input)
	if err != nil {
		t.Errorf("Teams.EditDiscussionByID returned error: %v", err)
	}

	want := &TeamDiscussion{Number: Int(3)}
	if !reflect.DeepEqual(comment, want) {
		t.Errorf("Teams.EditDiscussionByID returned %+v, want %+v", comment, want)
	}
}

func TestTeamsService_EditDiscussionBySlug(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	input := TeamDiscussion{Title: String("e_t"), Body: String("e_b")}

	mux.HandleFunc("/orgs/o/teams/s/discussions/3", func(w http.ResponseWriter, r *http.Request) {
		v := new(TeamDiscussion)
		json.NewDecoder(r.Body).Decode(v)

		testMethod(t, r, "PATCH")
		if !reflect.DeepEqual(v, &input) {
			t.Errorf("Request body = %+v, want %+v", v, input)
		}

		fmt.Fprint(w, `{"number":3}`)
	})

	comment, _, err := client.Teams.EditDiscussionBySlug(context.Background(), "o", "s", 3, input)
	if err != nil {
		t.Errorf("Teams.EditDiscussionBySlug returned error: %v", err)
	}

	want := &TeamDiscussion{Number: Int(3)}
	if !reflect.DeepEqual(comment, want) {
		t.Errorf("Teams.EditDiscussionBySlug returned %+v, want %+v", comment, want)
	}
}

func TestTeamsService_DeleteDiscussionByID(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/organizations/1/team/2/discussions/3", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	_, err := client.Teams.DeleteDiscussionByID(context.Background(), 1, 2, 3)
	if err != nil {
		t.Errorf("Teams.DeleteDiscussionByID returned error: %v", err)
	}
}

func TestTeamsService_DeleteDiscussionBySlug(t *testing.T) {
	client, mux, _, teardown := setup()
	defer teardown()

	mux.HandleFunc("/orgs/o/teams/s/discussions/3", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "DELETE")
	})

	_, err := client.Teams.DeleteDiscussionBySlug(context.Background(), "o", "s", 3)
	if err != nil {
		t.Errorf("Teams.DeleteDiscussionBySlug returned error: %v", err)
	}
}
