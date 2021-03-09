// Copyright 2020 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

func newActivitiesEventsPipeline() *pipelineSetup {
	return &pipelineSetup{
		baseURL:              "https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/",
		endpointsFromWebsite: activityEventsWant,
		filename:             "activity_events.go",
		serviceName:          "ActivityService",
		originalGoSource:     activityEventsGoFileOriginal,
		wantGoSource:         activityEventsGoFileWant,
		wantNumEndpoints:     7,
	}
}

func TestPipeline_ActivityEvents(t *testing.T) {
	ps := newActivitiesEventsPipeline()
	ps.setup(t, false, false)
	ps.validate(t)
}

func TestPipeline_ActivityEvents_FirstStripAllURLs(t *testing.T) {
	ps := newActivitiesEventsPipeline()
	ps.setup(t, true, false)
	ps.validate(t)
}

func TestPipeline_ActivityEvents_FirstDestroyReceivers(t *testing.T) {
	ps := newActivitiesEventsPipeline()
	ps.setup(t, false, true)
	ps.validate(t)
}

func TestPipeline_ActivityEvents_FirstStripAllURLsAndDestroyReceivers(t *testing.T) {
	ps := newActivitiesEventsPipeline()
	ps.setup(t, true, true)
	ps.validate(t)
}

func TestParseWebPageEndpoints_ActivityEvents(t *testing.T) {
	got, want := parseWebPageEndpoints(activityEventsTestWebPage), activityEventsWant
	testWebPageHelper(t, got, want)
}

var activityEventsWant = endpointsByFragmentID{
	"list-public-events": []*Endpoint{
		{urlFormats: []string{"events"}, httpMethod: "GET"},
	},

	"list-repository-events": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/events"}, httpMethod: "GET"},
	},

	"list-public-events-for-a-network-of-repositories": []*Endpoint{
		{urlFormats: []string{"networks/%v/%v/events"}, httpMethod: "GET"},
	},

	"list-events-received-by-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"users/%v/received_events"}, httpMethod: "GET"},
	},

	"list-events-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"users/%v/events"}, httpMethod: "GET"},
	},

	"list-public-events-for-a-user": []*Endpoint{
		{urlFormats: []string{"users/%v/events/public"}, httpMethod: "GET"},
	},

	"list-organization-events-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"users/%v/events/orgs/%v"}, httpMethod: "GET"},
	},

	"list-public-organization-events": []*Endpoint{
		{urlFormats: []string{"orgs/%v/events"}, httpMethod: "GET"},
	},

	"list-public-events-received-by-a-user": []*Endpoint{
		{urlFormats: []string{"users/%v/received_events/public"}, httpMethod: "GET"},
	},

	// Updated docs - consolidated into single page.

	"delete-a-thread-subscription": []*Endpoint{
		{urlFormats: []string{"notifications/threads/%v/subscription"}, httpMethod: "DELETE"},
	},

	"mark-notifications-as-read": []*Endpoint{
		{urlFormats: []string{"notifications"}, httpMethod: "PUT"},
	},

	"set-a-thread-subscription": []*Endpoint{
		{urlFormats: []string{"notifications/threads/%v/subscription"}, httpMethod: "PUT"},
	},

	"delete-a-repository-subscription": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/subscription"}, httpMethod: "DELETE"},
	},

	"star-a-repository-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"user/starred/%v/%v"}, httpMethod: "PUT"},
	},

	"list-repositories-starred-by-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"user/starred"}, httpMethod: "GET"},
	},

	"list-watchers": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/subscribers"}, httpMethod: "GET"},
	},

	"get-feeds": []*Endpoint{
		{urlFormats: []string{"feeds"}, httpMethod: "GET"},
	},

	"get-a-thread": []*Endpoint{
		{urlFormats: []string{"notifications/threads/%v"}, httpMethod: "GET"},
	},

	"mark-a-thread-as-read": []*Endpoint{
		{urlFormats: []string{"notifications/threads/%v"}, httpMethod: "PATCH"},
	},

	"list-stargazers": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/stargazers"}, httpMethod: "GET"},
	},

	"list-repositories-watched-by-a-user": []*Endpoint{
		{urlFormats: []string{"users/%v/subscriptions"}, httpMethod: "GET"},
	},

	"list-repository-notifications-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/notifications"}, httpMethod: "GET"},
	},

	"mark-repository-notifications-as-read": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/notifications"}, httpMethod: "PUT"},
	},

	"check-if-a-repository-is-starred-by-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"user/starred/%v/%v"}, httpMethod: "GET"},
	},

	"list-notifications-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"notifications"}, httpMethod: "GET"},
	},

	"get-a-thread-subscription-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"notifications/threads/%v/subscription"}, httpMethod: "GET"},
	},

	"unstar-a-repository-for-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"user/starred/%v/%v"}, httpMethod: "DELETE"},
	},

	"list-repositories-watched-by-the-authenticated-user": []*Endpoint{
		{urlFormats: []string{"user/subscriptions"}, httpMethod: "GET"},
	},

	"get-a-repository-subscription": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/subscription"}, httpMethod: "GET"},
	},

	"set-a-repository-subscription": []*Endpoint{
		{urlFormats: []string{"repos/%v/%v/subscription"}, httpMethod: "PUT"},
	},

	"list-repositories-starred-by-a-user": []*Endpoint{
		{urlFormats: []string{"users/%v/starred"}, httpMethod: "GET"},
	},
}

var activityEventsTestWebPage = `
<html lang="en">
  <head>
  <title>Activity - GitHub Docs</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="google-site-verification" content="OgdQc0GZfjDI52wDv1bkMT-SLpBUo_h5nn9mI9L22xQ" />
  <meta name="google-site-verification" content="c1kuD-K2HIVF635lypcsWPoD4kilo5-jA_wBFyT4uMY" />

  <!-- localized data needed by client-side JS  -->
  <meta name="site.data.ui.search.placeholder" content="Search topics, products...">
  <!-- end localized data -->

  

  <!-- hreflangs -->
  
    <link
      rel="alternate"
      hreflang="en"
      href="https://docs.github.com/en/free-pro-team@latest/rest/reference/activity"
    />
  
    <link
      rel="alternate"
      hreflang="zh-Hans"
      href="https://docs.github.com/cn/rest/reference/activity"
    />
  
    <link
      rel="alternate"
      hreflang="ja"
      href="https://docs.github.com/ja/rest/reference/activity"
    />
  
    <link
      rel="alternate"
      hreflang="es"
      href="https://docs.github.com/es/rest/reference/activity"
    />
  
    <link
      rel="alternate"
      hreflang="pt"
      href="https://docs.github.com/pt/rest/reference/activity"
    />
  
    <link
      rel="alternate"
      hreflang="de"
      href="https://docs.github.com/de/rest/reference/activity"
    />
  

  <link rel="stylesheet" href="/dist/index.css">
  <link rel="alternate icon" type="image/png" href="/assets/images/site/favicon.png">
  <link rel="icon" type="image/svg+xml" href="/assets/images/site/favicon.svg">
</head>


  <body class="d-lg-flex">
    <!-- product > category > maptopic > article -->
<div class="sidebar d-none d-lg-block">

  <div class="d-flex flex-items-center p-4" style="z-index: 3;" id="github-logo">
    <a href="/en" class="text-white" aria-hidden="true">
      <svg version="1.1" width="32" height="32" viewBox="0 0 16 16" class="octicon octicon-mark-github" aria-hidden="true"><path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path></svg>
    </a>
    <a href="/en" class="h4-mktg text-white no-underline no-wrap pl-2 flex-auto">GitHub Docs</a>
  </div>

    
    <ul class="sidebar-products">
      <!--
  Styling note:

  Categories, Maptopics, and Articles list items get a class of "active" when they correspond to content
  hierarchy of the current page. If an item's URL is also the same as the current URL, the item
  also gets an "is-current-page" class.
 -->


<li title="Home">
  <a href="/" class="f6 pl-4 pr-5 ml-n1 pb-1">
    <svg xmlns="http://www.w3.org/2000/svg" class="octicon" viewBox="0 0 16 16" width="16" height="16">  <path fill-rule="evenodd" clip-rule="evenodd" d="M7.78033 12.5303C7.48744 12.8232 7.01256 12.8232 6.71967 12.5303L2.46967 8.28033C2.17678 7.98744 2.17678 7.51256 2.46967 7.21967L6.71967 2.96967C7.01256 2.67678 7.48744 2.67678 7.78033 2.96967C8.07322 3.26256 8.07322 3.73744 7.78033 4.03033L4.81066 7L12.25 7C12.6642 7 13 7.33579 13 7.75C13 8.16421 12.6642 8.5 12.25 8.5L4.81066 8.5L7.78033 11.4697C8.07322 11.7626 8.07322 12.2374 7.78033 12.5303Z"></path></svg>
    All products
  </a>
</li>
<li title="REST API" class="sidebar-product mb-2">
  <a href="/en/rest" class="pl-4 pr-5 pb-1 f4">REST API</a>
</li>
<ul class="sidebar-categories list-style-none">
  
  

  <li class="sidebar-category py-1 ">
    <details class="dropdown-withArrow details details-reset" open>
      <summary>
        <div class="d-flex flex-justify-between">
          <a href="/en/free-pro-team@latest/rest/overview" class="pl-4 pr-2 py-2 f6 text-uppercase d-block flex-auto mr-3">Overview</a>
          <svg xmlns="http://www.w3.org/2000/svg" class="octicon flex-shrink-0 arrow mr-3" style="margin-top:7px" viewBox="0 0 16 16" width="16" height="16"> <path fill-rule="evenodd" clip-rule="evenodd" d="M12.7803 6.21967C13.0732 6.51256 13.0732 6.98744 12.7803 7.28033L8.53033 11.5303C8.23744 11.8232 7.76256 11.8232 7.46967 11.5303L3.21967 7.28033C2.92678 6.98744 2.92678 6.51256 3.21967 6.21967C3.51256 5.92678 3.98744 5.92678 4.28033 6.21967L8 9.93934L11.7197 6.21967C12.0126 5.92678 12.4874 5.92678 12.7803 6.21967Z"></path></svg>
        </div>
      </summary>
      <!-- some categories have maptopics with child articles -->
      
      <ul class="sidebar-articles list-style-none">
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/resources-in-the-rest-api" class="pl-4 pr-5 py-1">Resources in the REST API</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/media-types" class="pl-4 pr-5 py-1">Media types</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/other-authentication-methods" class="pl-4 pr-5 py-1">Other authentication methods</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/troubleshooting" class="pl-4 pr-5 py-1">Troubleshooting</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/api-previews" class="pl-4 pr-5 py-1">API previews</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/libraries" class="pl-4 pr-5 py-1">Libraries</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/overview/endpoints-available-for-github-apps" class="pl-4 pr-5 py-1 pb-2">Endpoints available for GitHub Apps</a>
        </li>
        
      </ul>
      
    </details>
  </li>
  
  

  <li class="sidebar-category py-1 active ">
    <details class="dropdown-withArrow details details-reset" open>
      <summary>
        <div class="d-flex flex-justify-between">
          <a href="/en/free-pro-team@latest/rest/reference" class="pl-4 pr-2 py-2 f6 text-uppercase d-block flex-auto mr-3">Reference</a>
          <svg xmlns="http://www.w3.org/2000/svg" class="octicon flex-shrink-0 arrow mr-3" style="margin-top:7px" viewBox="0 0 16 16" width="16" height="16"> <path fill-rule="evenodd" clip-rule="evenodd" d="M12.7803 6.21967C13.0732 6.51256 13.0732 6.98744 12.7803 7.28033L8.53033 11.5303C8.23744 11.8232 7.76256 11.8232 7.46967 11.5303L3.21967 7.28033C2.92678 6.98744 2.92678 6.51256 3.21967 6.21967C3.51256 5.92678 3.98744 5.92678 4.28033 6.21967L8 9.93934L11.7197 6.21967C12.0126 5.92678 12.4874 5.92678 12.7803 6.21967Z"></path></svg>
        </div>
      </summary>
      <!-- some categories have maptopics with child articles -->
      
      <ul class="sidebar-articles list-style-none">
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/actions" class="pl-4 pr-5 py-1">Actions</a>
        </li>
        
        
        <li class="sidebar-article active is-current-page">
          <a href="/en/free-pro-team@latest/rest/reference/activity" class="pl-4 pr-5 py-1">Activity</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/apps" class="pl-4 pr-5 py-1">Apps</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/billing" class="pl-4 pr-5 py-1">Billing</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/checks" class="pl-4 pr-5 py-1">Checks</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/code-scanning" class="pl-4 pr-5 py-1">Code Scanning</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/codes-of-conduct" class="pl-4 pr-5 py-1">Codes of conduct</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/emojis" class="pl-4 pr-5 py-1">Emojis</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/enterprise-admin" class="pl-4 pr-5 py-1">GitHub Enterprise administration</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/gists" class="pl-4 pr-5 py-1">Gists</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/git" class="pl-4 pr-5 py-1">Git database</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/gitignore" class="pl-4 pr-5 py-1">Gitignore</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/interactions" class="pl-4 pr-5 py-1">Interactions</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/issues" class="pl-4 pr-5 py-1">Issues</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/licenses" class="pl-4 pr-5 py-1">Licenses</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/markdown" class="pl-4 pr-5 py-1">Markdown</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/meta" class="pl-4 pr-5 py-1">Meta</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/migrations" class="pl-4 pr-5 py-1">Migrations</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/oauth-authorizations" class="pl-4 pr-5 py-1">OAuth Authorizations</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/orgs" class="pl-4 pr-5 py-1">Organizations</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/projects" class="pl-4 pr-5 py-1">Projects</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/pulls" class="pl-4 pr-5 py-1">Pulls</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/rate-limit" class="pl-4 pr-5 py-1">Rate limit</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/reactions" class="pl-4 pr-5 py-1">Reactions</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/repos" class="pl-4 pr-5 py-1">Repositories</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/scim" class="pl-4 pr-5 py-1">SCIM</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/search" class="pl-4 pr-5 py-1">Search</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/teams" class="pl-4 pr-5 py-1">Teams</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/users" class="pl-4 pr-5 py-1">Users</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/reference/permissions-required-for-github-apps" class="pl-4 pr-5 py-1 pb-2">Permissions required for GitHub Apps</a>
        </li>
        
      </ul>
      
    </details>
  </li>
  
  

  <li class="sidebar-category py-1 ">
    <details class="dropdown-withArrow details details-reset" open>
      <summary>
        <div class="d-flex flex-justify-between">
          <a href="/en/free-pro-team@latest/rest/guides" class="pl-4 pr-2 py-2 f6 text-uppercase d-block flex-auto mr-3">Guides</a>
          <svg xmlns="http://www.w3.org/2000/svg" class="octicon flex-shrink-0 arrow mr-3" style="margin-top:7px" viewBox="0 0 16 16" width="16" height="16"> <path fill-rule="evenodd" clip-rule="evenodd" d="M12.7803 6.21967C13.0732 6.51256 13.0732 6.98744 12.7803 7.28033L8.53033 11.5303C8.23744 11.8232 7.76256 11.8232 7.46967 11.5303L3.21967 7.28033C2.92678 6.98744 2.92678 6.51256 3.21967 6.21967C3.51256 5.92678 3.98744 5.92678 4.28033 6.21967L8 9.93934L11.7197 6.21967C12.0126 5.92678 12.4874 5.92678 12.7803 6.21967Z"></path></svg>
        </div>
      </summary>
      <!-- some categories have maptopics with child articles -->
      
      <ul class="sidebar-articles list-style-none">
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/getting-started-with-the-rest-api" class="pl-4 pr-5 py-1">Getting started with the REST API</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/basics-of-authentication" class="pl-4 pr-5 py-1">Basics of authentication</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/discovering-resources-for-a-user" class="pl-4 pr-5 py-1">Discovering resources for a user</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/delivering-deployments" class="pl-4 pr-5 py-1">Delivering deployments</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/rendering-data-as-graphs" class="pl-4 pr-5 py-1">Rendering data as graphs</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/working-with-comments" class="pl-4 pr-5 py-1">Working with comments</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/traversing-with-pagination" class="pl-4 pr-5 py-1">Traversing with pagination</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/building-a-ci-server" class="pl-4 pr-5 py-1">Building a CI server</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/best-practices-for-integrators" class="pl-4 pr-5 py-1">Best practices for integrators</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/getting-started-with-the-git-database-api" class="pl-4 pr-5 py-1">Getting started with the Git Database API</a>
        </li>
        
        
        <li class="sidebar-article ">
          <a href="/en/free-pro-team@latest/rest/guides/getting-started-with-the-checks-api" class="pl-4 pr-5 py-1 pb-2">Getting started with the Checks API</a>
        </li>
        
      </ul>
      
    </details>
  </li>
  
</ul>

    </ul>
    
</div>


    <main class="width-full">
      <div class="border-bottom border-gray-light no-print">

  

  

    <header class="container-xl px-3 px-md-6 pt-3 pb-2 position-relative d-flex flex-justify-between width-full ">

      <div class="d-flex flex-items-center d-md-none" style="z-index: 3;" id="github-logo-mobile">
        <a href="/en" aria-hidden="true">
          <svg version="1.1" width="32" height="32" viewBox="0 0 16 16" class="octicon octicon-mark-github text-black" aria-hidden="true"><path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path></svg>
        </a>
        <a href="/en" class="h4-mktg text-gray-dark no-underline no-wrap pl-2">GitHub Docs</a>
      </div>

      <div class="width-full">
        <div class="d-inline-block width-full d-md-flex" style="z-index: 1;">
          <button class="nav-mobile-burgerIcon float-right mt-1 border-0 d-md-none" type="button" aria-label="Toggle navigation">
            <!-- Hamburger icon added with css -->
          </button>
          <div style="z-index: 2;" class="nav-mobile-dropdown width-full">
            <div class="d-md-flex flex-justify-between flex-items-center">
              <div class="py-2 py-md-0 d-md-inline-block">
                <h4 class="text-mono f5 text-normal text-gray d-md-none">Explore by product</h4>
                <details class="dropdown-withArrow position-relative details details-reset d-md-none close-when-clicked-outside">
                  <summary class="nav-desktop-productDropdownButton text-blue-mktg py-2" role="button" aria-label="Toggle products list">
                    <div id="current-product" class="d-flex flex-items-center flex-justify-between" style="padding-top: 2px;">
                      <!-- Product switcher - GitHub.com, Enterprise Server, etc -->
                      <!-- 404 and 500 error layouts are not real pages so we need to hardcode the name for those -->
                      REST API
                      <svg class="arrow ml-md-1" width="14px" height="8px" viewBox="0 0 14 8" xml:space="preserve" fill="none" stroke="#1277eb"><path d="M1,1l6.2,6L13,1"></path></svg>
                    </div>
                  </summary>
                  <div id="homepages" class="position-md-absolute nav-desktop-productDropdown p-md-4 left-md-n4 top-md-6" style="z-index: 6;">
                    
                    <a href="/en/github"
                       class="d-block py-2 link-gray-dark no-underline">
                       GitHub.com
                       
                    </a>
                    
                    <a href="/en/enterprise/admin"
                       class="d-block py-2 link-gray-dark no-underline">
                       Enterprise Server
                       
                    </a>
                    
                    <a href="/en/actions"
                       class="d-block py-2 link-gray-dark no-underline">
                       GitHub Actions
                       
                    </a>
                    
                    <a href="/en/packages"
                       class="d-block py-2 link-gray-dark no-underline">
                       GitHub Packages
                       
                    </a>
                    
                    <a href="/en/developers"
                       class="d-block py-2 link-gray-dark no-underline">
                       Developers
                       
                    </a>
                    
                    <a href="/en/rest"
                       class="d-block py-2 text-blue-mktg text-underline active">
                       REST API
                       
                    </a>
                    
                    <a href="/en/graphql"
                       class="d-block py-2 link-gray-dark no-underline">
                       GraphQL API
                       
                    </a>
                    
                    <a href="/en/insights"
                       class="d-block py-2 link-gray-dark no-underline">
                       GitHub Insights
                       
                    </a>
                    
                    <a href="/en/desktop"
                       class="d-block py-2 link-gray-dark no-underline">
                       GitHub Desktop
                       
                    </a>
                    
                    <a href="https://atom.io/docs"
                       class="d-block py-2 link-gray-dark no-underline">
                       Atom
                       
                       <span class="ml-1"><svg width="9" height="10" viewBox="0 0 9 10" fill="none" xmlns="http://www.w3.org/2000/svg"><path stroke="#24292e" d="M.646 8.789l8-8M8.5 9V1M1 .643h8"/></svg></span>
                       
                    </a>
                    
                    <a href="https://electronjs.org/docs"
                       class="d-block py-2 link-gray-dark no-underline">
                       Electron
                       
                       <span class="ml-1"><svg width="9" height="10" viewBox="0 0 9 10" fill="none" xmlns="http://www.w3.org/2000/svg"><path stroke="#24292e" d="M.646 8.789l8-8M8.5 9V1M1 .643h8"/></svg></span>
                       
                    </a>
                    
                  </div>
                </details>
              </div>
              <div class="d-md-inline-block">

                
                  <div class="border-top border-md-top-0 py-2 py-md-0 d-md-inline-block">
                    <details class="dropdown-withArrow position-relative details details-reset mr-md-3 close-when-clicked-outside">
                      <summary class="py-2 text-gray-dark" role="button" aria-label="Toggle languages list">
                        <div class="d-flex flex-items-center flex-justify-between">
                          <!-- Language switcher - 'English', 'Japanese', etc -->
                          
                            English
                          
                          <svg class="arrow ml-md-1" width="14px" height="8px" viewBox="0 0 14 8" xml:space="preserve" fill="none" stroke="#1B1F23"><path d="M1,1l6.2,6L13,1"></path></svg>
                        </div>
                      </summary>
                      <div id="languages-selector" class="position-md-absolute nav-desktop-langDropdown p-md-4 right-md-n4 top-md-6" style="z-index: 6;">
                      
                        
                          <a
                            href="/en/free-pro-team@latest/rest/reference/activity"
                            class="d-block py-2 no-underline active link-gray"
                            style="white-space: nowrap"
                          >
                            
                              English
                            
                          </a>
                        
                      
                        
                          <a
                            href="/cn/rest/reference/activity"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              简体中文 (Simplified Chinese)
                            
                          </a>
                        
                      
                        
                          <a
                            href="/ja/rest/reference/activity"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              日本語 (Japanese)
                            
                          </a>
                        
                      
                        
                          <a
                            href="/es/rest/reference/activity"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              Español (Spanish)
                            
                          </a>
                        
                      
                        
                          <a
                            href="/pt/rest/reference/activity"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              Português do Brasil (Portuguese)
                            
                          </a>
                        
                      
                        
                      
                      </div>
                    </details>
                  </div>
                

                <!-- GitHub.com homepage and 404 page has a stylized search; Enterprise homepages do not -->
                
                <div class="pt-3 pt-md-0 d-md-inline-block ml-md-3 bord'er-top border-md-top-0">
                  <!--
  This form is used in two places:

  - On the homepage, front and center
  - On all other pages, in the header
 -->

<form class="mb-0">
  <div id="search-input-container" aria-hidden="true">
    <!-- Aloglia instantsearch.js will add a search input here -->
  </div>
</form>

                  <div id="search-results-container"></div>
                  <div class="search-overlay-desktop"></div>
                </div>
                

              </div>
            </div>
          </div>
        </div>
      </div>
    </header>
  </div>

      
        <main class="container-xl px-3 px-md-6 my-4 my-lg-4 d-lg-flex">
  <article class="markdown-body width-full">
    


    <div class="article-grid-container">
      <div class="article-grid-toc">
        
  <details id="article-versions" class="dropdown-withArrow d-inline-block details details-reset mb-4 mb-md-0 position-relative close-when-clicked-outside">
    <summary class="d-flex flex-items-center flex-justify-between f4 h5-mktg btn-outline-mktg btn-mktg p-2">
      <!-- GitHub.com, Enterprise Server 2.16, etc -->
      <span class="d-md-none d-xl-inline-block mr-1">Article version:</span> GitHub.com
      <svg class="arrow ml-1" width="14px" height="8px" viewBox="0 0 14 8" xml:space="preserve" fill="none" stroke="#1277eb"><path d="M1,1l6.2,6L13,1"></path></svg>
    </summary>

    <div class="nav-dropdown position-md-absolute bg-white rounded-1 px-4 py-3 top-7 box-shadow-large" style="z-index: 6; width: 210px;">
      
      <a
      href="/en/free-pro-team@latest/rest/reference/activity"
      class="d-block py-2 link-blue active"
      >GitHub.com</a>
      
      <a
      href="/en/enterprise/2.21/user/rest/reference/activity"
      class="d-block py-2 link-gray-dark no-underline"
      >Enterprise Server 2.21</a>
      
      <a
      href="/en/enterprise/2.20/user/rest/reference/activity"
      class="d-block py-2 link-gray-dark no-underline"
      >Enterprise Server 2.20</a>
      
      <a
      href="/en/enterprise/2.19/user/rest/reference/activity"
      class="d-block py-2 link-gray-dark no-underline"
      >Enterprise Server 2.19</a>
      
    </div>
  </details>


      </div>
      <div class="article-grid-body d-flex flex-items-center" style="height: 39px;">
        <nav class="breadcrumbs f5" aria-label="Breadcrumb">
  
  <a title="product: REST API" href="/en/rest" class="d-inline-block ">
    REST API</a>
  
  <a title="category: Reference" href="/en/free-pro-team@latest/rest/reference" class="d-inline-block ">
    Reference</a>
  
  <a title="article: Activity" href="/en/free-pro-team@latest/rest/reference/activity" class="d-inline-block text-gray-light">
    Activity</a>
  
</nav>

      </div>
    </div>

    <div class="mt-2 article-grid-container">

    <div class="article-grid-head">
      <div class="d-flex flex-items-baseline flex-justify-between mt-3">
        <h1 class="border-bottom-0">Activity</h1>
        <div class="d-none d-lg-block ml-2">
          <button class="btn-link link-gray js-print tooltipped tooltipped-n" aria-label="Print this article">
            <!-- From https://heroicons.dev/ -->
<svg fill="none" height="18" width="18" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" viewBox="0 0 24 24" stroke="currentColor"><path d="M17 17h2a2 2 0 002-2v-4a2 2 0 00-2-2H5a2 2 0 00-2 2v4a2 2 0 002 2h2m2 4h6a2 2 0 002-2v-4a2 2 0 00-2-2H9a2 2 0 00-2 2v4a2 2 0 002 2zm8-12V5a2 2 0 00-2-2H9a2 2 0 00-2 2v4h10z"></path></svg>
          </button>
        </div>
      </div>

      

      

      

      
    </div>
    <div class="article-grid-toc border-bottom border-xl-0 pb-4 mb-5 pb-xl-0 mb-xl-0">
      <div class="article-grid-toc-content">
        
        <h3 id="in-this-article" class="f5 mb-2"><a class="link-gray-dark" href="#in-this-article">In this article</a></h3>
        <ul class="list-style-none pl-0 f5 mb-0">
          
          <li class="ml-0  mb-2 lh-condensed"><a href="#events">Events</a></li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-public-events">List public events</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-public-events-for-a-network-of-repositories">List public events for a network of repositories</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-public-organization-events">List public organization events</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-repository-events">List repository events</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-events-for-the-authenticated-user">List events for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-organization-events-for-the-authenticated-user">List organization events for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-public-events-for-a-user">List public events for a user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-events-received-by-the-authenticated-user">List events received by the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-public-events-received-by-a-user">List public events received by a user</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed"><a href="#feeds">Feeds</a></li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#get-feeds">Get feeds</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed"><a href="#example-of-getting-an-atom-feed">Example of getting an Atom feed</a></li>
          
          <li class="ml-0  mb-2 lh-condensed"><a href="#notifications">Notifications</a></li>
          
          <li class="ml-3  mb-2 lh-condensed"><a href="#notification-reasons">Notification reasons</a></li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-notifications-for-the-authenticated-user">List notifications for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#mark-notifications-as-read">Mark notifications as read</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#get-a-thread">Get a thread</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#mark-a-thread-as-read">Mark a thread as read</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#get-a-thread-subscription-for-the-authenticated-user">Get a thread subscription for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#set-a-thread-subscription">Set a thread subscription</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#delete-a-thread-subscription">Delete a thread subscription</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-repository-notifications-for-the-authenticated-user">List repository notifications for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#mark-repository-notifications-as-read">Mark repository notifications as read</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed"><a href="#starring">Starring</a></li>
          
          <li class="ml-3  mb-2 lh-condensed"><a href="#starring-vs-watching">Starring vs. Watching</a></li>
          
          <li class="ml-3  mb-2 lh-condensed"><a href="#custom-media-types-for-starring">Custom media types for starring</a></li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-stargazers">List stargazers</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-repositories-starred-by-the-authenticated-user">List repositories starred by the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#check-if-a-repository-is-starred-by-the-authenticated-user">Check if a repository is starred by the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#star-a-repository-for-the-authenticated-user">Star a repository for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#unstar-a-repository-for-the-authenticated-user">Unstar a repository for the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-repositories-starred-by-a-user">List repositories starred by a user</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed"><a href="#watching">Watching</a></li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-watchers">List watchers</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#get-a-repository-subscription">Get a repository subscription</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#set-a-repository-subscription">Set a repository subscription</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#delete-a-repository-subscription">Delete a repository subscription</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-repositories-watched-by-the-authenticated-user">List repositories watched by the authenticated user</a>
    </li>
          
          <li class="ml-3  mb-2 lh-condensed">
      <a href="#list-repositories-watched-by-a-user">List repositories watched by a user</a>
    </li>
          
        </ul>
        
        <div class="d-none d-xl-block border-top border-gray-light mt-4">
          
          <form class="js-helpfulness mt-4 f5" id="helpfulness-xl">
  <h4
    data-help-start
    data-help-yes
    data-help-no
  >
    Did this doc help you?
  </h4>
  <p
    class="radio-group"
    data-help-start
    data-help-yes
    data-help-no
  >
    <input
      hidden
      id="helpfulness-yes-xl"
      type="radio"
      name="helpfulness-vote"
      value="Yes"
      aria-label="Yes"
    />
    <label class="btn x-radio-label" for="helpfulness-yes-xl">
      <svg version="1.1" width="24" height="24" viewBox="0 0 24 24" class="octicon octicon-thumbsup" aria-hidden="true"><path fill-rule="evenodd" d="M12.596 2.043c-1.301-.092-2.303.986-2.303 2.206v1.053c0 2.666-1.813 3.785-2.774 4.2a1.866 1.866 0 01-.523.131A1.75 1.75 0 005.25 8h-1.5A1.75 1.75 0 002 9.75v10.5c0 .967.784 1.75 1.75 1.75h1.5a1.75 1.75 0 001.742-1.58c.838.06 1.667.296 2.69.586l.602.17c1.464.406 3.213.824 5.544.824 2.188 0 3.693-.204 4.583-1.372.422-.554.65-1.255.816-2.05.148-.708.262-1.57.396-2.58l.051-.39c.319-2.386.328-4.18-.223-5.394-.293-.644-.743-1.125-1.355-1.431-.59-.296-1.284-.404-2.036-.404h-2.05l.056-.429c.025-.18.05-.372.076-.572.06-.483.117-1.006.117-1.438 0-1.245-.222-2.253-.92-2.941-.684-.675-1.668-.88-2.743-.956zM7 18.918c1.059.064 2.079.355 3.118.652l.568.16c1.406.39 3.006.77 5.142.77 2.277 0 3.004-.274 3.39-.781.216-.283.388-.718.54-1.448.136-.65.242-1.45.379-2.477l.05-.384c.32-2.4.253-3.795-.102-4.575-.16-.352-.375-.568-.66-.711-.305-.153-.74-.245-1.365-.245h-2.37c-.681 0-1.293-.57-1.211-1.328.026-.243.065-.537.105-.834l.07-.527c.06-.482.105-.921.105-1.25 0-1.125-.213-1.617-.473-1.873-.275-.27-.774-.455-1.795-.528-.351-.024-.698.274-.698.71v1.053c0 3.55-2.488 5.063-3.68 5.577-.372.16-.754.232-1.113.26v7.78zM3.75 20.5a.25.25 0 01-.25-.25V9.75a.25.25 0 01.25-.25h1.5a.25.25 0 01.25.25v10.5a.25.25 0 01-.25.25h-1.5z"></path></svg>
    </label>
    <input
      hidden
      id="helpfulness-no-xl"
      type="radio"
      name="helpfulness-vote"
      value="No"
      aria-label="No"
    />
    <label class="btn x-radio-label" for="helpfulness-no-xl">
      <svg version="1.1" width="24" height="24" viewBox="0 0 24 24" class="octicon octicon-thumbsdown" aria-hidden="true"><path fill-rule="evenodd" d="M12.596 21.957c-1.301.092-2.303-.986-2.303-2.206v-1.053c0-2.666-1.813-3.785-2.774-4.2a1.864 1.864 0 00-.523-.13A1.75 1.75 0 015.25 16h-1.5A1.75 1.75 0 012 14.25V3.75C2 2.784 2.784 2 3.75 2h1.5a1.75 1.75 0 011.742 1.58c.838-.06 1.667-.296 2.69-.586l.602-.17C11.748 2.419 13.497 2 15.828 2c2.188 0 3.693.204 4.583 1.372.422.554.65 1.255.816 2.05.148.708.262 1.57.396 2.58l.051.39c.319 2.386.328 4.18-.223 5.394-.293.644-.743 1.125-1.355 1.431-.59.296-1.284.404-2.036.404h-2.05l.056.429c.025.18.05.372.076.572.06.483.117 1.006.117 1.438 0 1.245-.222 2.253-.92 2.942-.684.674-1.668.879-2.743.955zM7 5.082c1.059-.064 2.079-.355 3.118-.651.188-.054.377-.108.568-.16 1.406-.392 3.006-.771 5.142-.771 2.277 0 3.004.274 3.39.781.216.283.388.718.54 1.448.136.65.242 1.45.379 2.477l.05.385c.32 2.398.253 3.794-.102 4.574-.16.352-.375.569-.66.711-.305.153-.74.245-1.365.245h-2.37c-.681 0-1.293.57-1.211 1.328.026.244.065.537.105.834l.07.527c.06.482.105.922.105 1.25 0 1.125-.213 1.617-.473 1.873-.275.27-.774.456-1.795.528-.351.024-.698-.274-.698-.71v-1.053c0-3.55-2.488-5.063-3.68-5.577A3.485 3.485 0 007 12.861V5.08zM3.75 3.5a.25.25 0 00-.25.25v10.5c0 .138.112.25.25.25h1.5a.25.25 0 00.25-.25V3.75a.25.25 0 00-.25-.25h-1.5z"></path></svg>
    </label>
  </p>
  <p class="text-gray f6" hidden data-help-yes>
    Want to learn about new docs features and updates? Sign up for updates!
  </p>
  <p class="text-gray f6" hidden data-help-no>
    We're continually improving our docs. We'd love to hear how we can do better.
  </p>
  <input
    type="text"
    class="d-none"
    name="helpfulness-token"
    aria-hidden="true"
  />
  <p hidden data-help-no>
    <label
      class="d-block mb-1 f6"
      for="helpfulness-category-xl"
    >
      What problem did you have?
      <span class="text-normal text-gray-light float-right ml-1">
        Required
      </span>
    </label>
    <select
      class="form-control select-sm width-full"
      name="helpfulness-category"
      id="helpfulness-category-xl"
    >
      <option value="">
        Choose an option
      </option>
      <option value="Unclear">
        Information was unclear
      </option>
      <option value="Confusing">
        The content was confusing
      </option>
      <option value="Unhelpful">
        The article didn't answer my question
      </option>
      <option value="Other">
        Other
      </option>
    </select>
  </p>
  <p hidden data-help-no>
    <label
      class="d-block mb-1 f6"
      for="helpfulness-comment-xl"
    >
      <span>Let us know what we can do better</span>
      <span class="text-normal text-gray-light float-right ml-1">
        Optional
      </span>
    </label>
    <textarea
      class="form-control input-sm width-full"
      name="helpfulness-comment"
      id="helpfulness-comment-xl"
    ></textarea>
  </p>
  <p>
    <label
      class="d-block mb-1 f6"
      for="helpfulness-email-xl"
      hidden
      data-help-no
    >
      Can we contact you if we have more questions?
      <span class="text-normal text-gray-light float-right ml-1">
        Optional
      </span>
    </label>
    <input
      type="email"
      class="form-control input-sm width-full"
      name="helpfulness-email"
      id="helpfulness-email-xl"
      placeholder="email@example.com"
      hidden
      data-help-yes
      data-help-no
    />
  </p>
  <p
    class="text-right"
    hidden
    data-help-yes
    data-help-no
  >
    <button type="submit" class="btn btn-blue btn-sm">
      Send
    </button>
  </p>
  <p class="text-gray f6" hidden data-help-end>
    Thank you! Your feedback has been submitted.
  </p>
</form>

        </div>
      </div>
    </div>
    <div id="article-contents" class="article-grid-body">
      
      <h2 id="events"><a href="#events">Events</a></h2>
<p>The Events API is a read-only API to the GitHub events. These events power the various activity streams on the site.</p>
<p>The Events API can return different types of events triggered by activity on GitHub. For more information about the specific events that you can receive from the Events API, see &quot;<a href="/en/developers/webhooks-and-events/github-event-types">GitHub Event types</a>.&quot; An events API for repository issues is also available. For more information, see the &quot;<a href="/en/free-pro-team@latest/rest/reference/issues#events">Issue Events API</a>.&quot;</p>
<p>Events are optimized for polling with the &quot;ETag&quot; header. If no new events have been triggered, you will see a &quot;304 Not Modified&quot; response, and your current rate limit will be untouched. There is also an &quot;X-Poll-Interval&quot; header that specifies how often (in seconds) you are allowed to poll. In times of high server load, the time may increase. Please obey the header.</p>
<pre><code class="hljs language-shell">$ curl -I https://api.github.com/users/tater/events
&gt; HTTP/1.1 200 OK
&gt; X-Poll-Interval: 60
&gt; ETag: &quot;a18c3bded88eb5dbb5c849a489412bf3&quot;

# The quotes around the ETag value are important
$ curl -I https://api.github.com/users/tater/events \
$    -H &apos;If-None-Match: &quot;a18c3bded88eb5dbb5c849a489412bf3&quot;&apos;
&gt; HTTP/1.1 304 Not Modified
&gt; X-Poll-Interval: 60</code></pre>
<p>Events support pagination, however the <code>per_page</code> option is unsupported. The fixed page size is 30 items. Fetching up to ten pages is supported, for a total of 300 events. For information, see &quot;<a href="/en/free-pro-team@latest/rest/guides/traversing-with-pagination">Traversing with pagination</a>.&quot;</p>
<p>Only events created within the past 90 days will be included in timelines. Events older than 90 days will not be included (even if the total number of events in the timeline is less than 300).</p>
  <div>
  <div>
    <h3 id="list-public-events" class="pt-3">
      <a href="#list-public-events">List public events</a>
    </h3>
    <p>We delay the public events feed by five minutes, which means the most recent event returned by the public events API actually occurred at least five minutes ago.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /events</code></pre>
  <div>
    
      <h4 id="list-public-events--parameters">
        <a href="#list-public-events--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-public-events--code-samples">
        <a href="#list-public-events--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/events
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /events&apos;</span>)
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-public-events-for-a-network-of-repositories" class="pt-3">
      <a href="#list-public-events-for-a-network-of-repositories">List public events for a network of repositories</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /networks/{owner}/{repo}/events</code></pre>
  <div>
    
      <h4 id="list-public-events-for-a-network-of-repositories--parameters">
        <a href="#list-public-events-for-a-network-of-repositories--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-public-events-for-a-network-of-repositories--code-samples">
        <a href="#list-public-events-for-a-network-of-repositories--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/networks/octocat/hello-world/events
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /networks/{owner}/{repo}/events&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-public-organization-events" class="pt-3">
      <a href="#list-public-organization-events">List public organization events</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /orgs/{org}/events</code></pre>
  <div>
    
      <h4 id="list-public-organization-events--parameters">
        <a href="#list-public-organization-events--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>org</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-public-organization-events--code-samples">
        <a href="#list-public-organization-events--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/orgs/ORG/events
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /orgs/{org}/events&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-repository-events" class="pt-3">
      <a href="#list-repository-events">List repository events</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/events</code></pre>
  <div>
    
      <h4 id="list-repository-events--parameters">
        <a href="#list-repository-events--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-repository-events--code-samples">
        <a href="#list-repository-events--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/events
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/events&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-events-for-the-authenticated-user" class="pt-3">
      <a href="#list-events-for-the-authenticated-user">List events for the authenticated user</a>
    </h3>
    <p>If you are authenticated as the given user, you will see your private events. Otherwise, you&apos;ll only see public events.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/events</code></pre>
  <div>
    
      <h4 id="list-events-for-the-authenticated-user--parameters">
        <a href="#list-events-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-events-for-the-authenticated-user--code-samples">
        <a href="#list-events-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/events
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/events&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-organization-events-for-the-authenticated-user" class="pt-3">
      <a href="#list-organization-events-for-the-authenticated-user">List organization events for the authenticated user</a>
    </h3>
    <p>This is the user&apos;s organization dashboard. You must be authenticated as the user to view this.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/events/orgs/{org}</code></pre>
  <div>
    
      <h4 id="list-organization-events-for-the-authenticated-user--parameters">
        <a href="#list-organization-events-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>org</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-organization-events-for-the-authenticated-user--code-samples">
        <a href="#list-organization-events-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/events/orgs/ORG
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/events/orgs/{org}&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>,
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-public-events-for-a-user" class="pt-3">
      <a href="#list-public-events-for-a-user">List public events for a user</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/events/public</code></pre>
  <div>
    
      <h4 id="list-public-events-for-a-user--parameters">
        <a href="#list-public-events-for-a-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-public-events-for-a-user--code-samples">
        <a href="#list-public-events-for-a-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/events/public
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/events/public&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-events-received-by-the-authenticated-user" class="pt-3">
      <a href="#list-events-received-by-the-authenticated-user">List events received by the authenticated user</a>
    </h3>
    <p>These are events that you&apos;ve received by watching repos and following users. If you are authenticated as the given user, you will see private events. Otherwise, you&apos;ll only see public events.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/received_events</code></pre>
  <div>
    
      <h4 id="list-events-received-by-the-authenticated-user--parameters">
        <a href="#list-events-received-by-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-events-received-by-the-authenticated-user--code-samples">
        <a href="#list-events-received-by-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/received_events
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/received_events&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-public-events-received-by-a-user" class="pt-3">
      <a href="#list-public-events-received-by-a-user">List public events received by a user</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/received_events/public</code></pre>
  <div>
    
      <h4 id="list-public-events-received-by-a-user--parameters">
        <a href="#list-public-events-received-by-a-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-public-events-received-by-a-user--code-samples">
        <a href="#list-public-events-received-by-a-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/received_events/public
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/received_events/public&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
<h2 id="feeds"><a href="#feeds">Feeds</a></h2>
  <div>
  <div>
    <h3 id="get-feeds" class="pt-3">
      <a href="#get-feeds">Get feeds</a>
    </h3>
    <p>GitHub provides several timeline resources in <a href="http://en.wikipedia.org/wiki/Atom_(standard)">Atom</a> format. The Feeds API lists all the feeds available to the authenticated user:</p>
<ul>
<li><strong>Timeline</strong>: The GitHub global public timeline</li>
<li><strong>User</strong>: The public timeline for any user, using <a href="https://developer.github.com/v3/#hypermedia">URI template</a></li>
<li><strong>Current user public</strong>: The public timeline for the authenticated user</li>
<li><strong>Current user</strong>: The private timeline for the authenticated user</li>
<li><strong>Current user actor</strong>: The private timeline for activity created by the authenticated user</li>
<li><strong>Current user organizations</strong>: The private timeline for the organizations the authenticated user is a member of.</li>
<li><strong>Security advisories</strong>: A collection of public announcements that provide information about security-related vulnerabilities in software on GitHub.</li>
</ul>
<p><strong>Note</strong>: Private feeds are only returned when <a href="https://developer.github.com/v3/#basic-authentication">authenticating via Basic Auth</a> since current feed URIs use the older, non revocable auth tokens.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /feeds</code></pre>
  <div>
    
      <h4 id="get-feeds--parameters">
        <a href="#get-feeds--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="get-feeds--code-samples">
        <a href="#get-feeds--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/feeds
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /feeds&apos;</span>)
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;timeline_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/timeline&quot;</span>,
  <span class="hljs-attr">&quot;user_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/{user}&quot;</span>,
  <span class="hljs-attr">&quot;current_user_public_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
  <span class="hljs-attr">&quot;current_user_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat.private?token=abc123&quot;</span>,
  <span class="hljs-attr">&quot;current_user_actor_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat.private.actor?token=abc123&quot;</span>,
  <span class="hljs-attr">&quot;current_user_organization_url&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
  <span class="hljs-attr">&quot;current_user_organization_urls&quot;</span>: [
    <span class="hljs-string">&quot;https://github.com/organizations/github/octocat.private.atom?token=abc123&quot;</span>
  ],
  <span class="hljs-attr">&quot;security_advisories_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/security-advisories&quot;</span>,
  <span class="hljs-attr">&quot;_links&quot;</span>: {
    <span class="hljs-attr">&quot;timeline&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/timeline&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
    },
    <span class="hljs-attr">&quot;user&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/{user}&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
    },
    <span class="hljs-attr">&quot;current_user_public&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
    },
    <span class="hljs-attr">&quot;current_user&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat.private?token=abc123&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
    },
    <span class="hljs-attr">&quot;current_user_actor&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat.private.actor?token=abc123&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
    },
    <span class="hljs-attr">&quot;current_user_organization&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;&quot;</span>
    },
    <span class="hljs-attr">&quot;current_user_organizations&quot;</span>: [
      {
        <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/organizations/github/octocat.private.atom?token=abc123&quot;</span>,
        <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
      }
    ],
    <span class="hljs-attr">&quot;security_advisories&quot;</span>: {
      <span class="hljs-attr">&quot;href&quot;</span>: <span class="hljs-string">&quot;https://github.com/security-advisories&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;application/atom+xml&quot;</span>
    }
  }
}
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
<h3 id="example-of-getting-an-atom-feed"><a href="#example-of-getting-an-atom-feed">Example of getting an Atom feed</a></h3>
<p>To get a feed in Atom format, you must specify the <code>application/atom+xml</code> type in the <code>Accept</code> header. For example, to get the Atom feed for GitHub security advisories:</p>
<pre><code>curl -H &quot;Accept: application/atom+xml&quot; https://github.com/security-advisories
</code></pre>
<h4 id="response"><a href="#response">Response</a></h4>
<pre><code class="hljs language-shell">Status: 200 OK</code></pre>
<pre><code class="hljs language-xml"><span class="hljs-meta">&lt;?xml version=&quot;1.0&quot; encoding=&quot;UTF-8&quot;?&gt;</span>
<span class="hljs-tag">&lt;<span class="hljs-name">feed</span> <span class="hljs-attr">xmlns</span>=<span class="hljs-string">&quot;http://www.w3.org/2005/Atom&quot;</span> <span class="hljs-attr">xmlns:media</span>=<span class="hljs-string">&quot;http://search.yahoo.com/mrss/&quot;</span> <span class="hljs-attr">xml:lang</span>=<span class="hljs-string">&quot;en-US&quot;</span>&gt;</span>
  <span class="hljs-tag">&lt;<span class="hljs-name">id</span>&gt;</span>tag:github.com,2008:/security-advisories<span class="hljs-tag">&lt;/<span class="hljs-name">id</span>&gt;</span>
  <span class="hljs-tag">&lt;<span class="hljs-name">link</span> <span class="hljs-attr">rel</span>=<span class="hljs-string">&quot;self&quot;</span> <span class="hljs-attr">type</span>=<span class="hljs-string">&quot;application/atom+xml&quot;</span> <span class="hljs-attr">href</span>=<span class="hljs-string">&quot;https://github.com/security-advisories.atom&quot;</span>/&gt;</span>
  <span class="hljs-tag">&lt;<span class="hljs-name">title</span>&gt;</span>GitHub Security Advisory Feed<span class="hljs-tag">&lt;/<span class="hljs-name">title</span>&gt;</span>
  <span class="hljs-tag">&lt;<span class="hljs-name">author</span>&gt;</span>
    <span class="hljs-tag">&lt;<span class="hljs-name">name</span>&gt;</span>GitHub<span class="hljs-tag">&lt;/<span class="hljs-name">name</span>&gt;</span>
  <span class="hljs-tag">&lt;/<span class="hljs-name">author</span>&gt;</span>
  <span class="hljs-tag">&lt;<span class="hljs-name">updated</span>&gt;</span>2019-01-14T19:34:52Z<span class="hljs-tag">&lt;/<span class="hljs-name">updated</span>&gt;</span>
     <span class="hljs-tag">&lt;<span class="hljs-name">entry</span>&gt;</span>
      <span class="hljs-tag">&lt;<span class="hljs-name">id</span>&gt;</span>tag:github.com,2008:GHSA-abcd-12ab-23cd<span class="hljs-tag">&lt;/<span class="hljs-name">id</span>&gt;</span>
      <span class="hljs-tag">&lt;<span class="hljs-name">published</span>&gt;</span>2018-07-26T15:14:52Z<span class="hljs-tag">&lt;/<span class="hljs-name">published</span>&gt;</span>
      <span class="hljs-tag">&lt;<span class="hljs-name">updated</span>&gt;</span>2019-01-14T19:34:52Z<span class="hljs-tag">&lt;/<span class="hljs-name">updated</span>&gt;</span>
      <span class="hljs-tag">&lt;<span class="hljs-name">title</span> <span class="hljs-attr">type</span>=<span class="hljs-string">&quot;html&quot;</span>&gt;</span>[GHSA-abcd-12ab-23cd] Moderate severity vulnerability that affects Octoapp<span class="hljs-tag">&lt;/<span class="hljs-name">title</span>&gt;</span>
        <span class="hljs-tag">&lt;<span class="hljs-name">category</span> <span class="hljs-attr">term</span>=<span class="hljs-string">&quot;NPM&quot;</span>/&gt;</span>
      <span class="hljs-tag">&lt;<span class="hljs-name">content</span> <span class="hljs-attr">type</span>=<span class="hljs-string">&quot;html&quot;</span>&gt;</span>
        <span class="hljs-symbol">&amp;lt;</span>p<span class="hljs-symbol">&amp;gt;</span>Octoapp node module before 4.17.5 suffers from a Modification of Assumed-Immutable Data (MAID) vulnerability via defaultsDeep, merge, and mergeWith functions, which allows a malicious user to modify the prototype of <span class="hljs-symbol">&amp;quot;</span>Object<span class="hljs-symbol">&amp;quot;</span> via <span class="hljs-symbol">&amp;lt;</span>strong<span class="hljs-symbol">&amp;gt;</span>proto<span class="hljs-symbol">&amp;lt;</span>/strong<span class="hljs-symbol">&amp;gt;</span>, causing the addition or modification of an existing property that will exist on all objects.<span class="hljs-symbol">&amp;lt;</span>/p<span class="hljs-symbol">&amp;gt;</span>
          <span class="hljs-symbol">&amp;lt;</span>p<span class="hljs-symbol">&amp;gt;</span><span class="hljs-symbol">&amp;lt;</span>strong<span class="hljs-symbol">&amp;gt;</span>Affected Packages<span class="hljs-symbol">&amp;lt;</span>/strong<span class="hljs-symbol">&amp;gt;</span><span class="hljs-symbol">&amp;lt;</span>/p<span class="hljs-symbol">&amp;gt;</span>

  <span class="hljs-symbol">&amp;lt;</span>dl<span class="hljs-symbol">&amp;gt;</span>
      <span class="hljs-symbol">&amp;lt;</span>dt<span class="hljs-symbol">&amp;gt;</span>Octoapp<span class="hljs-symbol">&amp;lt;</span>/dt<span class="hljs-symbol">&amp;gt;</span>
      <span class="hljs-symbol">&amp;lt;</span>dd<span class="hljs-symbol">&amp;gt;</span>Ecosystem: npm<span class="hljs-symbol">&amp;lt;</span>/dd<span class="hljs-symbol">&amp;gt;</span>
      <span class="hljs-symbol">&amp;lt;</span>dd<span class="hljs-symbol">&amp;gt;</span>Severity: moderate<span class="hljs-symbol">&amp;lt;</span>/dd<span class="hljs-symbol">&amp;gt;</span>
      <span class="hljs-symbol">&amp;lt;</span>dd<span class="hljs-symbol">&amp;gt;</span>Versions: <span class="hljs-symbol">&amp;amp;</span>lt; 4.17.5<span class="hljs-symbol">&amp;lt;</span>/dd<span class="hljs-symbol">&amp;gt;</span>
        <span class="hljs-symbol">&amp;lt;</span>dd<span class="hljs-symbol">&amp;gt;</span>Fixed in: 4.17.5<span class="hljs-symbol">&amp;lt;</span>/dd<span class="hljs-symbol">&amp;gt;</span>
  <span class="hljs-symbol">&amp;lt;</span>/dl<span class="hljs-symbol">&amp;gt;</span>

          <span class="hljs-symbol">&amp;lt;</span>p<span class="hljs-symbol">&amp;gt;</span><span class="hljs-symbol">&amp;lt;</span>strong<span class="hljs-symbol">&amp;gt;</span>References<span class="hljs-symbol">&amp;lt;</span>/strong<span class="hljs-symbol">&amp;gt;</span><span class="hljs-symbol">&amp;lt;</span>/p<span class="hljs-symbol">&amp;gt;</span>

  <span class="hljs-symbol">&amp;lt;</span>ul<span class="hljs-symbol">&amp;gt;</span>
      <span class="hljs-symbol">&amp;lt;</span>li<span class="hljs-symbol">&amp;gt;</span>https://nvd.nist.gov/vuln/detail/CVE-2018-123<span class="hljs-symbol">&amp;lt;</span>/li<span class="hljs-symbol">&amp;gt;</span>
  <span class="hljs-symbol">&amp;lt;</span>/ul<span class="hljs-symbol">&amp;gt;</span>

      <span class="hljs-tag">&lt;/<span class="hljs-name">content</span>&gt;</span>
    <span class="hljs-tag">&lt;/<span class="hljs-name">entry</span>&gt;</span>
<span class="hljs-tag">&lt;/<span class="hljs-name">feed</span>&gt;</span>
</code></pre>
<h2 id="notifications"><a href="#notifications">Notifications</a></h2>
<p>Users receive notifications for conversations in repositories they watch including:</p>
<ul>
<li>Issues and their comments</li>
<li>Pull Requests and their comments</li>
<li>Comments on any commits</li>
</ul>
<p>Notifications are also sent for conversations in unwatched repositories when the user is involved including:</p>
<ul>
<li><strong>@mentions</strong></li>
<li>Issue assignments</li>
<li>Commits the user authors or commits</li>
<li>Any discussion in which the user actively participates</li>
</ul>
<p>All Notification API calls require the <code>notifications</code> or <code>repo</code> API scopes.  Doing this will give read-only access to some issue and commit content. You will still need the <code>repo</code> scope to access issues and commits from their respective endpoints.</p>
<p>Notifications come back as &quot;threads&quot;.  A thread contains information about the current discussion of an issue, pull request, or commit.</p>
<p>Notifications are optimized for polling with the <code>Last-Modified</code> header.  If there are no new notifications, you will see a <code>304 Not Modified</code> response, leaving your current rate limit untouched.  There is an <code>X-Poll-Interval</code> header that specifies how often (in seconds) you are allowed to poll.  In times of high server load, the time may increase.  Please obey the header.</p>
<pre><code class="hljs language-shell"># Add authentication to your requests
$ curl -I https://api.github.com/notifications
HTTP/1.1 200 OK
Last-Modified: Thu, 25 Oct 2012 15:16:27 GMT
X-Poll-Interval: 60

# Pass the Last-Modified header exactly
$ curl -I https://api.github.com/notifications
$    -H &quot;If-Modified-Since: Thu, 25 Oct 2012 15:16:27 GMT&quot;
&gt; HTTP/1.1 304 Not Modified
&gt; X-Poll-Interval: 60</code></pre>
<h3 id="notification-reasons"><a href="#notification-reasons">Notification reasons</a></h3>
<p>When retrieving responses from the Notifications API, each payload has a key titled <code>reason</code>. These correspond to events that trigger a notification.</p>
<p>Here&apos;s a list of potential <code>reason</code>s for receiving a notification:</p>





















































<table><thead><tr><th>Reason Name</th><th>Description</th></tr></thead><tbody><tr><td><code>assign</code></td><td>You were assigned to the issue.</td></tr><tr><td><code>author</code></td><td>You created the thread.</td></tr><tr><td><code>comment</code></td><td>You commented on the thread.</td></tr><tr><td><code>invitation</code></td><td>You accepted an invitation to contribute to the repository.</td></tr><tr><td><code>manual</code></td><td>You subscribed to the thread (via an issue or pull request).</td></tr><tr><td><code>mention</code></td><td>You were specifically <strong>@mentioned</strong> in the content.</td></tr><tr><td><code>review_requested</code></td><td>You, or a team you&apos;re a member of, were requested to review a pull request.</td></tr><tr><td><code>security_alert</code></td><td>GitHub discovered a <a href="/en/github/managing-security-vulnerabilities/about-alerts-for-vulnerable-dependencies">security vulnerability</a> in your repository.</td></tr><tr><td><code>state_change</code></td><td>You changed the thread state (for example, closing an issue or merging a pull request).</td></tr><tr><td><code>subscribed</code></td><td>You&apos;re watching the repository.</td></tr><tr><td><code>team_mention</code></td><td>You were on a team that was mentioned.</td></tr></tbody></table>
<p>Note that the <code>reason</code> is modified on a per-thread basis, and can change, if the <code>reason</code> on a later notification is different.</p>
<p>For example, if you are the author of an issue, subsequent notifications on that issue will have a <code>reason</code> of <code>author</code>. If you&apos;re then  <strong>@mentioned</strong> on the same issue, the notifications you fetch thereafter will have a <code>reason</code> of <code>mention</code>. The <code>reason</code> remains as <code>mention</code>, regardless of whether you&apos;re ever mentioned again.</p>
  <div>
  <div>
    <h3 id="list-notifications-for-the-authenticated-user" class="pt-3">
      <a href="#list-notifications-for-the-authenticated-user">List notifications for the authenticated user</a>
    </h3>
    <p>List all notifications for the current user, sorted by most recently updated.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /notifications</code></pre>
  <div>
    
      <h4 id="list-notifications-for-the-authenticated-user--parameters">
        <a href="#list-notifications-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>all</code></td>
              <td class="opacity-70">boolean</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>If <code>true</code>, show notifications marked as read.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>participating</code></td>
              <td class="opacity-70">boolean</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>If <code>true</code>, only shows notifications in which the user is directly participating or mentioned.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>since</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Only show notifications updated after the given time. This is a timestamp in <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> format: <code>YYYY-MM-DDTHH:MM:SSZ</code>.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>before</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Only show notifications updated before the given time. This is a timestamp in <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> format: <code>YYYY-MM-DDTHH:MM:SSZ</code>.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-notifications-for-the-authenticated-user--code-samples">
        <a href="#list-notifications-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /notifications&apos;</span>)
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-string">&quot;1&quot;</span>,
    <span class="hljs-attr">&quot;repository&quot;</span>: {
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
      <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;owner&quot;</span>: {
        <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
        <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
        <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
        <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
        <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
        <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
        <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
        <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
        <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
        <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
        <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
        <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
        <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
        <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
        <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
        <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
        <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
        <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
      },
      <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
      <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
      <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
      <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
      <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
      <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
      <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
      <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
      <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
      <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
      <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
      <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
      <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
      <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
      <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
      <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
      <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
      <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
      <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
      <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
      <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
      <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
      <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
      <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
      <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
      <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
      <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
      <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
      <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
      <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
      <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
      <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>
    },
    <span class="hljs-attr">&quot;subject&quot;</span>: {
      <span class="hljs-attr">&quot;title&quot;</span>: <span class="hljs-string">&quot;Greetings&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octokit/octokit.rb/issues/123&quot;</span>,
      <span class="hljs-attr">&quot;latest_comment_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octokit/octokit.rb/issues/comments/123&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;Issue&quot;</span>
    },
    <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-string">&quot;subscribed&quot;</span>,
    <span class="hljs-attr">&quot;unread&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2014-11-07T22:01:45Z&quot;</span>,
    <span class="hljs-attr">&quot;last_read_at&quot;</span>: <span class="hljs-string">&quot;2014-11-07T22:01:45Z&quot;</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1&quot;</span>
  }
]
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="mark-notifications-as-read" class="pt-3">
      <a href="#mark-notifications-as-read">Mark notifications as read</a>
    </h3>
    <p>Marks all notifications as &quot;read&quot; removes it from the <a href="https://github.com/notifications">default view on GitHub</a>. If the number of notifications is too large to complete in one request, you will receive a <code>202 Accepted</code> status and GitHub will run an asynchronous process to mark notifications as &quot;read.&quot; To check whether any &quot;unread&quot; notifications remain, you can use the <a href="https://developer.github.com/v3/activity/notifications/#list-notifications-for-the-authenticated-user">List notifications for the authenticated user</a> endpoint and pass the query parameter <code>all=false</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">put</span> /notifications</code></pre>
  <div>
    
      <h4 id="mark-notifications-as-read--parameters">
        <a href="#mark-notifications-as-read--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>last_read_at</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p>Describes the last point that notifications were checked. Anything updated since this time will not be marked as read. If you omit this parameter, all notifications are marked as read. This is a timestamp in <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> format: <code>YYYY-MM-DDTHH:MM:SSZ</code>. Default: The current timestamp.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="mark-notifications-as-read--code-samples">
        <a href="#mark-notifications-as-read--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X PUT \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications \
  -d &apos;{&quot;last_read_at&quot;:&quot;last_read_at&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;PUT /notifications&apos;</span>, {
  <span class="hljs-attr">last_read_at</span>: <span class="hljs-string">&apos;last_read_at&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 205 Reset Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="get-a-thread" class="pt-3">
      <a href="#get-a-thread">Get a thread</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /notifications/threads/{thread_id}</code></pre>
  <div>
    
      <h4 id="get-a-thread--parameters">
        <a href="#get-a-thread--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>thread_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="get-a-thread--code-samples">
        <a href="#get-a-thread--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications/threads/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /notifications/threads/{thread_id}&apos;</span>, {
  <span class="hljs-attr">thread_id</span>: <span class="hljs-number">42</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-string">&quot;1&quot;</span>,
  <span class="hljs-attr">&quot;repository&quot;</span>: {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
    <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;owner&quot;</span>: {
      <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
      <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
      <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
      <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
      <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
      <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
      <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
      <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
      <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
      <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
      <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
      <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
    },
    <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
    <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
    <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
    <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
    <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
    <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
    <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
    <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
    <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
    <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
    <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
    <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
    <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
    <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
    <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
    <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
    <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
    <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
    <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
    <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
    <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
    <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
    <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
    <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
    <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
    <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
    <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>
  },
  <span class="hljs-attr">&quot;subject&quot;</span>: {
    <span class="hljs-attr">&quot;title&quot;</span>: <span class="hljs-string">&quot;Greetings&quot;</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octokit/octokit.rb/issues/123&quot;</span>,
    <span class="hljs-attr">&quot;latest_comment_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octokit/octokit.rb/issues/comments/123&quot;</span>,
    <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;Issue&quot;</span>
  },
  <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-string">&quot;subscribed&quot;</span>,
  <span class="hljs-attr">&quot;unread&quot;</span>: <span class="hljs-literal">true</span>,
  <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2014-11-07T22:01:45Z&quot;</span>,
  <span class="hljs-attr">&quot;last_read_at&quot;</span>: <span class="hljs-string">&quot;2014-11-07T22:01:45Z&quot;</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1&quot;</span>
}
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="mark-a-thread-as-read" class="pt-3">
      <a href="#mark-a-thread-as-read">Mark a thread as read</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">patch</span> /notifications/threads/{thread_id}</code></pre>
  <div>
    
      <h4 id="mark-a-thread-as-read--parameters">
        <a href="#mark-a-thread-as-read--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>thread_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="mark-a-thread-as-read--code-samples">
        <a href="#mark-a-thread-as-read--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X PATCH \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications/threads/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;PATCH /notifications/threads/{thread_id}&apos;</span>, {
  <span class="hljs-attr">thread_id</span>: <span class="hljs-number">42</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 205 Reset Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="get-a-thread-subscription-for-the-authenticated-user" class="pt-3">
      <a href="#get-a-thread-subscription-for-the-authenticated-user">Get a thread subscription for the authenticated user</a>
    </h3>
    <p>This checks to see if the current user is subscribed to a thread. You can also <a href="https://developer.github.com/v3/activity/watching/#get-a-repository-subscription">get a repository subscription</a>.</p>
<p>Note that subscriptions are only generated if a user is participating in a conversation--for example, they&apos;ve replied to the thread, were <strong>@mentioned</strong>, or manually subscribe to a thread.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /notifications/threads/{thread_id}/subscription</code></pre>
  <div>
    
      <h4 id="get-a-thread-subscription-for-the-authenticated-user--parameters">
        <a href="#get-a-thread-subscription-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>thread_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="get-a-thread-subscription-for-the-authenticated-user--code-samples">
        <a href="#get-a-thread-subscription-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications/threads/42/subscription
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /notifications/threads/{thread_id}/subscription&apos;</span>, {
  <span class="hljs-attr">thread_id</span>: <span class="hljs-number">42</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;subscribed&quot;</span>: <span class="hljs-literal">true</span>,
  <span class="hljs-attr">&quot;ignored&quot;</span>: <span class="hljs-literal">false</span>,
  <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-literal">null</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2012-10-06T21:34:12Z&quot;</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1/subscription&quot;</span>,
  <span class="hljs-attr">&quot;thread_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1&quot;</span>
}
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="set-a-thread-subscription" class="pt-3">
      <a href="#set-a-thread-subscription">Set a thread subscription</a>
    </h3>
    <p>If you are watching a repository, you receive notifications for all threads by default. Use this endpoint to ignore future notifications for threads until you comment on the thread or get an <strong>@mention</strong>.</p>
<p>You can also use this endpoint to subscribe to threads that you are currently not receiving notifications for or to subscribed to threads that you have previously ignored.</p>
<p>Unsubscribing from a conversation in a repository that you are not watching is functionally equivalent to the <a href="https://developer.github.com/v3/activity/notifications/#delete-a-thread-subscription">Delete a thread subscription</a> endpoint.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">put</span> /notifications/threads/{thread_id}/subscription</code></pre>
  <div>
    
      <h4 id="set-a-thread-subscription--parameters">
        <a href="#set-a-thread-subscription--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>thread_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>ignored</code></td>
            <td class="opacity-70">boolean</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p>Unsubscribes and subscribes you to a conversation. Set <code>ignored</code> to <code>true</code> to block all notifications from this thread.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="set-a-thread-subscription--code-samples">
        <a href="#set-a-thread-subscription--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X PUT \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications/threads/42/subscription \
  -d &apos;{&quot;ignored&quot;:true}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;PUT /notifications/threads/{thread_id}/subscription&apos;</span>, {
  <span class="hljs-attr">thread_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">ignored</span>: <span class="hljs-literal">true</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;subscribed&quot;</span>: <span class="hljs-literal">true</span>,
  <span class="hljs-attr">&quot;ignored&quot;</span>: <span class="hljs-literal">false</span>,
  <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-literal">null</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2012-10-06T21:34:12Z&quot;</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1/subscription&quot;</span>,
  <span class="hljs-attr">&quot;thread_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1&quot;</span>
}
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-a-thread-subscription" class="pt-3">
      <a href="#delete-a-thread-subscription">Delete a thread subscription</a>
    </h3>
    <p>Mutes all future notifications for a conversation until you comment on the thread or get an <strong>@mention</strong>. If you are watching the repository of the thread, you will still receive notifications. To ignore future notifications for a repository you are watching, use the <a href="https://developer.github.com/v3/activity/notifications/#set-a-thread-subscription">Set a thread subscription</a> endpoint and set <code>ignore</code> to <code>true</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /notifications/threads/{thread_id}/subscription</code></pre>
  <div>
    
      <h4 id="delete-a-thread-subscription--parameters">
        <a href="#delete-a-thread-subscription--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>thread_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-a-thread-subscription--code-samples">
        <a href="#delete-a-thread-subscription--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/notifications/threads/42/subscription
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /notifications/threads/{thread_id}/subscription&apos;</span>, {
  <span class="hljs-attr">thread_id</span>: <span class="hljs-number">42</span>
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-repository-notifications-for-the-authenticated-user" class="pt-3">
      <a href="#list-repository-notifications-for-the-authenticated-user">List repository notifications for the authenticated user</a>
    </h3>
    <p>List all notifications for the current user.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/notifications</code></pre>
  <div>
    
      <h4 id="list-repository-notifications-for-the-authenticated-user--parameters">
        <a href="#list-repository-notifications-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>all</code></td>
              <td class="opacity-70">boolean</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>If <code>true</code>, show notifications marked as read.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>participating</code></td>
              <td class="opacity-70">boolean</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>If <code>true</code>, only shows notifications in which the user is directly participating or mentioned.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>since</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Only show notifications updated after the given time. This is a timestamp in <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> format: <code>YYYY-MM-DDTHH:MM:SSZ</code>.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>before</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Only show notifications updated before the given time. This is a timestamp in <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> format: <code>YYYY-MM-DDTHH:MM:SSZ</code>.</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-repository-notifications-for-the-authenticated-user--code-samples">
        <a href="#list-repository-notifications-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/notifications
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/notifications&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-string">&quot;1&quot;</span>,
    <span class="hljs-attr">&quot;repository&quot;</span>: {
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
      <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;owner&quot;</span>: {
        <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
        <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
        <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
        <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
        <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
        <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
        <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
        <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
        <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
        <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
        <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
        <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
        <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
        <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
        <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
        <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
        <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
        <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
      },
      <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
      <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
      <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
      <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
      <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
      <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
      <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
      <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
      <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
      <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
      <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
      <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
      <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
      <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
      <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
      <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
      <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
      <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
      <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
      <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
      <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
      <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
      <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
      <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
      <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
      <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
      <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
      <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
      <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
      <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
      <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
      <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
      <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
      <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>
    },
    <span class="hljs-attr">&quot;subject&quot;</span>: {
      <span class="hljs-attr">&quot;title&quot;</span>: <span class="hljs-string">&quot;Greetings&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octokit/octokit.rb/issues/123&quot;</span>,
      <span class="hljs-attr">&quot;latest_comment_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octokit/octokit.rb/issues/comments/123&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;Issue&quot;</span>
    },
    <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-string">&quot;subscribed&quot;</span>,
    <span class="hljs-attr">&quot;unread&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2014-11-07T22:01:45Z&quot;</span>,
    <span class="hljs-attr">&quot;last_read_at&quot;</span>: <span class="hljs-string">&quot;2014-11-07T22:01:45Z&quot;</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/notifications/threads/1&quot;</span>
  }
]
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="mark-repository-notifications-as-read" class="pt-3">
      <a href="#mark-repository-notifications-as-read">Mark repository notifications as read</a>
    </h3>
    <p>Marks all notifications in a repository as &quot;read&quot; removes them from the <a href="https://github.com/notifications">default view on GitHub</a>. If the number of notifications is too large to complete in one request, you will receive a <code>202 Accepted</code> status and GitHub will run an asynchronous process to mark notifications as &quot;read.&quot; To check whether any &quot;unread&quot; notifications remain, you can use the <a href="https://developer.github.com/v3/activity/notifications/#list-repository-notifications-for-the-authenticated-user">List repository notifications for the authenticated user</a> endpoint and pass the query parameter <code>all=false</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">put</span> /repos/{owner}/{repo}/notifications</code></pre>
  <div>
    
      <h4 id="mark-repository-notifications-as-read--parameters">
        <a href="#mark-repository-notifications-as-read--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>last_read_at</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p>Describes the last point that notifications were checked. Anything updated since this time will not be marked as read. If you omit this parameter, all notifications are marked as read. This is a timestamp in <a href="https://en.wikipedia.org/wiki/ISO_8601">ISO 8601</a> format: <code>YYYY-MM-DDTHH:MM:SSZ</code>. Default: The current timestamp.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="mark-repository-notifications-as-read--code-samples">
        <a href="#mark-repository-notifications-as-read--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X PUT \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/notifications \
  -d &apos;{&quot;last_read_at&quot;:&quot;last_read_at&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;PUT /repos/{owner}/{repo}/notifications&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">last_read_at</span>: <span class="hljs-string">&apos;last_read_at&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response</h4>
      <pre><code>Status: 205 Reset Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
<h2 id="starring"><a href="#starring">Starring</a></h2>
<p>Repository starring is a feature that lets users bookmark repositories. Stars are shown next to repositories to show an approximate level of interest. Stars have no effect on notifications or the activity feed.</p>
<h3 id="starring-vs-watching"><a href="#starring-vs-watching">Starring vs. Watching</a></h3>
<p>In August 2012, we <a href="https://github.com/blog/1204-notifications-stars">changed the way watching
works</a> on GitHub. Many API
client applications may be using the original &quot;watcher&quot; endpoints for accessing
this data. You can now start using the &quot;star&quot; endpoints instead (described
below). For more information, see the <a href="https://developer.github.com/changes/2012-09-05-watcher-api/">Watcher API Change post</a> and the &quot;<a href="/en/free-pro-team@latest/rest/reference/activity#watching">Repository Watching API</a>.&quot;</p>
<h3 id="custom-media-types-for-starring"><a href="#custom-media-types-for-starring">Custom media types for starring</a></h3>
<p>There is one supported custom media type for the Starring REST API. When you use this custom media type, you will receive a response with the <code>starred_at</code> timestamp property that indicates the time the star was created. The response also has a second property that includes the resource that is returned when the custom media type is not included. The property that contains the resource will be either <code>user</code> or <code>repo</code>.</p>
<pre><code>application/vnd.github.v3.star+json
</code></pre>
<p>For more information about media types, see &quot;<a href="/en/free-pro-team@latest/rest/overview/media-types">Custom media types</a>.&quot;</p>
  <div>
  <div>
    <h3 id="list-stargazers" class="pt-3">
      <a href="#list-stargazers">List stargazers</a>
    </h3>
    <p>Lists the people that have starred the repository.</p>
<p>You can also find out <em>when</em> stars were created by passing the following custom <a href="https://developer.github.com/v3/media/">media type</a> via the <code>Accept</code> header:</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/stargazers</code></pre>
  <div>
    
      <h4 id="list-stargazers--parameters">
        <a href="#list-stargazers--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-stargazers--code-samples">
        <a href="#list-stargazers--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/stargazers
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/stargazers&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
    <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
    <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
    <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
    <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
    <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
    <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
    <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
    <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
    <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
    <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
    <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
    <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-repositories-starred-by-the-authenticated-user" class="pt-3">
      <a href="#list-repositories-starred-by-the-authenticated-user">List repositories starred by the authenticated user</a>
    </h3>
    <p>Lists repositories the authenticated user has starred.</p>
<p>You can also find out <em>when</em> stars were created by passing the following custom <a href="https://developer.github.com/v3/media/">media type</a> via the <code>Accept</code> header:</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /user/starred</code></pre>
  <div>
    
      <h4 id="list-repositories-starred-by-the-authenticated-user--parameters">
        <a href="#list-repositories-starred-by-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>sort</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>One of <code>created</code> (when the repository was starred) or <code>updated</code> (when it was last pushed to).</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>direction</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>One of <code>asc</code> (ascending) or <code>desc</code> (descending).</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-repositories-starred-by-the-authenticated-user--code-samples">
        <a href="#list-repositories-starred-by-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/user/starred
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /user/starred&apos;</span>)
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
    <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;owner&quot;</span>: {
      <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
      <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
      <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
      <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
      <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
      <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
      <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
      <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
      <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
      <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
      <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
      <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
    },
    <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
    <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
    <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
    <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
    <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
    <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
    <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
    <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
    <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
    <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
    <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
    <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
    <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
    <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
    <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
    <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
    <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
    <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
    <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
    <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
    <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
    <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
    <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
    <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
    <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
    <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
    <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;clone_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;mirror_url&quot;</span>: <span class="hljs-string">&quot;git:git.example.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;hooks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/hooks&quot;</span>,
    <span class="hljs-attr">&quot;svn_url&quot;</span>: <span class="hljs-string">&quot;https://svn.github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;homepage&quot;</span>: <span class="hljs-string">&quot;https://github.com&quot;</span>,
    <span class="hljs-attr">&quot;language&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;forks_count&quot;</span>: <span class="hljs-number">9</span>,
    <span class="hljs-attr">&quot;stargazers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;watchers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;size&quot;</span>: <span class="hljs-number">108</span>,
    <span class="hljs-attr">&quot;default_branch&quot;</span>: <span class="hljs-string">&quot;master&quot;</span>,
    <span class="hljs-attr">&quot;open_issues_count&quot;</span>: <span class="hljs-number">0</span>,
    <span class="hljs-attr">&quot;is_template&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;topics&quot;</span>: [
      <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-string">&quot;atom&quot;</span>,
      <span class="hljs-string">&quot;electron&quot;</span>,
      <span class="hljs-string">&quot;api&quot;</span>
    ],
    <span class="hljs-attr">&quot;has_issues&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_projects&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_wiki&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_pages&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;has_downloads&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;archived&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;disabled&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;visibility&quot;</span>: <span class="hljs-string">&quot;public&quot;</span>,
    <span class="hljs-attr">&quot;pushed_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:06:43Z&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:01:12Z&quot;</span>,
    <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:14:43Z&quot;</span>,
    <span class="hljs-attr">&quot;permissions&quot;</span>: {
      <span class="hljs-attr">&quot;admin&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;push&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;pull&quot;</span>: <span class="hljs-literal">true</span>
    },
    <span class="hljs-attr">&quot;allow_rebase_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;template_repository&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;temp_clone_token&quot;</span>: <span class="hljs-string">&quot;ABTLWHOULUVAXGTRYU7OC2876QJ2O&quot;</span>,
    <span class="hljs-attr">&quot;allow_squash_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;delete_branch_on_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;allow_merge_commit&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;subscribers_count&quot;</span>: <span class="hljs-number">42</span>,
    <span class="hljs-attr">&quot;network_count&quot;</span>: <span class="hljs-number">0</span>
  }
]
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="check-if-a-repository-is-starred-by-the-authenticated-user" class="pt-3">
      <a href="#check-if-a-repository-is-starred-by-the-authenticated-user">Check if a repository is starred by the authenticated user</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /user/starred/{owner}/{repo}</code></pre>
  <div>
    
      <h4 id="check-if-a-repository-is-starred-by-the-authenticated-user--parameters">
        <a href="#check-if-a-repository-is-starred-by-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="check-if-a-repository-is-starred-by-the-authenticated-user--code-samples">
        <a href="#check-if-a-repository-is-starred-by-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/user/starred/octocat/hello-world
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /user/starred/{owner}/{repo}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response if this repository is starred by you</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
      <h4>Response if this repository is not starred by you</h4>
      <pre><code>Status: 404 Not Found</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="star-a-repository-for-the-authenticated-user" class="pt-3">
      <a href="#star-a-repository-for-the-authenticated-user">Star a repository for the authenticated user</a>
    </h3>
    <p>Note that you&apos;ll need to set <code>Content-Length</code> to zero when calling out to this endpoint. For more information, see &quot;<a href="https://developer.github.com/v3/#http-verbs">HTTP verbs</a>.&quot;</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">put</span> /user/starred/{owner}/{repo}</code></pre>
  <div>
    
      <h4 id="star-a-repository-for-the-authenticated-user--parameters">
        <a href="#star-a-repository-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="star-a-repository-for-the-authenticated-user--code-samples">
        <a href="#star-a-repository-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X PUT \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/user/starred/octocat/hello-world
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;PUT /user/starred/{owner}/{repo}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="unstar-a-repository-for-the-authenticated-user" class="pt-3">
      <a href="#unstar-a-repository-for-the-authenticated-user">Unstar a repository for the authenticated user</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /user/starred/{owner}/{repo}</code></pre>
  <div>
    
      <h4 id="unstar-a-repository-for-the-authenticated-user--parameters">
        <a href="#unstar-a-repository-for-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="unstar-a-repository-for-the-authenticated-user--code-samples">
        <a href="#unstar-a-repository-for-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/user/starred/octocat/hello-world
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /user/starred/{owner}/{repo}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-repositories-starred-by-a-user" class="pt-3">
      <a href="#list-repositories-starred-by-a-user">List repositories starred by a user</a>
    </h3>
    <p>Lists repositories a user has starred.</p>
<p>You can also find out <em>when</em> stars were created by passing the following custom <a href="https://developer.github.com/v3/media/">media type</a> via the <code>Accept</code> header:</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/starred</code></pre>
  <div>
    
      <h4 id="list-repositories-starred-by-a-user--parameters">
        <a href="#list-repositories-starred-by-a-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>sort</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>One of <code>created</code> (when the repository was starred) or <code>updated</code> (when it was last pushed to).</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>direction</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>One of <code>asc</code> (ascending) or <code>desc</code> (descending).</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-repositories-starred-by-a-user--code-samples">
        <a href="#list-repositories-starred-by-a-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/starred
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/starred&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
    <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;owner&quot;</span>: {
      <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
      <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
      <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
      <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
      <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
      <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
      <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
      <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
      <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
      <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
      <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
      <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
    },
    <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
    <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
    <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
    <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
    <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
    <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
    <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
    <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
    <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
    <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
    <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
    <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
    <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
    <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
    <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
    <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
    <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
    <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
    <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
    <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
    <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
    <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
    <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
    <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
    <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
    <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
    <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;clone_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;mirror_url&quot;</span>: <span class="hljs-string">&quot;git:git.example.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;hooks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/hooks&quot;</span>,
    <span class="hljs-attr">&quot;svn_url&quot;</span>: <span class="hljs-string">&quot;https://svn.github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;homepage&quot;</span>: <span class="hljs-string">&quot;https://github.com&quot;</span>,
    <span class="hljs-attr">&quot;language&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;forks_count&quot;</span>: <span class="hljs-number">9</span>,
    <span class="hljs-attr">&quot;stargazers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;watchers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;size&quot;</span>: <span class="hljs-number">108</span>,
    <span class="hljs-attr">&quot;default_branch&quot;</span>: <span class="hljs-string">&quot;master&quot;</span>,
    <span class="hljs-attr">&quot;open_issues_count&quot;</span>: <span class="hljs-number">0</span>,
    <span class="hljs-attr">&quot;is_template&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;topics&quot;</span>: [
      <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-string">&quot;atom&quot;</span>,
      <span class="hljs-string">&quot;electron&quot;</span>,
      <span class="hljs-string">&quot;api&quot;</span>
    ],
    <span class="hljs-attr">&quot;has_issues&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_projects&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_wiki&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_pages&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;has_downloads&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;archived&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;disabled&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;visibility&quot;</span>: <span class="hljs-string">&quot;public&quot;</span>,
    <span class="hljs-attr">&quot;pushed_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:06:43Z&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:01:12Z&quot;</span>,
    <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:14:43Z&quot;</span>,
    <span class="hljs-attr">&quot;permissions&quot;</span>: {
      <span class="hljs-attr">&quot;admin&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;push&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;pull&quot;</span>: <span class="hljs-literal">true</span>
    },
    <span class="hljs-attr">&quot;allow_rebase_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;template_repository&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;temp_clone_token&quot;</span>: <span class="hljs-string">&quot;ABTLWHOULUVAXGTRYU7OC2876QJ2O&quot;</span>,
    <span class="hljs-attr">&quot;allow_squash_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;delete_branch_on_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;allow_merge_commit&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;subscribers_count&quot;</span>: <span class="hljs-number">42</span>,
    <span class="hljs-attr">&quot;network_count&quot;</span>: <span class="hljs-number">0</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
<h2 id="watching"><a href="#watching">Watching</a></h2>
<p>Watching a repository registers the user to receive notifications on new discussions, as well as events in the user&apos;s activity feed. For simple repository bookmarks, see &quot;<a href="/en/free-pro-team@latest/rest/reference/activity#starring">Repository starring</a>.&quot;</p>
  <div>
  <div>
    <h3 id="list-watchers" class="pt-3">
      <a href="#list-watchers">List watchers</a>
    </h3>
    <p>Lists the people watching the specified repository.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/subscribers</code></pre>
  <div>
    
      <h4 id="list-watchers--parameters">
        <a href="#list-watchers--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-watchers--code-samples">
        <a href="#list-watchers--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/subscribers
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/subscribers&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
    <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
    <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
    <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
    <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
    <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
    <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
    <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
    <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
    <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
    <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
    <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
    <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="get-a-repository-subscription" class="pt-3">
      <a href="#get-a-repository-subscription">Get a repository subscription</a>
    </h3>
    
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/subscription</code></pre>
  <div>
    
      <h4 id="get-a-repository-subscription--parameters">
        <a href="#get-a-repository-subscription--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="get-a-repository-subscription--code-samples">
        <a href="#get-a-repository-subscription--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/subscription
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/subscription&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Response if you subscribe to the repository</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;subscribed&quot;</span>: <span class="hljs-literal">true</span>,
  <span class="hljs-attr">&quot;ignored&quot;</span>: <span class="hljs-literal">false</span>,
  <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-literal">null</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2012-10-06T21:34:12Z&quot;</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/example/subscription&quot;</span>,
  <span class="hljs-attr">&quot;repository_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/example&quot;</span>
}
</code></pre></div>
    
      <h4>Response if you don t subscribe to the repository</h4>
      <pre><code>Status: 404 Not Found</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="set-a-repository-subscription" class="pt-3">
      <a href="#set-a-repository-subscription">Set a repository subscription</a>
    </h3>
    <p>If you would like to watch a repository, set <code>subscribed</code> to <code>true</code>. If you would like to ignore notifications made within a repository, set <code>ignored</code> to <code>true</code>. If you would like to stop watching a repository, <a href="https://developer.github.com/v3/activity/watching/#delete-a-repository-subscription">delete the repository&apos;s subscription</a> completely.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">put</span> /repos/{owner}/{repo}/subscription</code></pre>
  <div>
    
      <h4 id="set-a-repository-subscription--parameters">
        <a href="#set-a-repository-subscription--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>subscribed</code></td>
            <td class="opacity-70">boolean</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p>Determines if notifications should be received from this repository.</p>
              
            </td>
          </tr>
          
          
          <tr>
            <td><code>ignored</code></td>
            <td class="opacity-70">boolean</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p>Determines if all notifications should be blocked from this repository.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="set-a-repository-subscription--code-samples">
        <a href="#set-a-repository-subscription--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X PUT \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/subscription \
  -d &apos;{&quot;subscribed&quot;:true}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;PUT /repos/{owner}/{repo}/subscription&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">subscribed</span>: <span class="hljs-literal">true</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;subscribed&quot;</span>: <span class="hljs-literal">true</span>,
  <span class="hljs-attr">&quot;ignored&quot;</span>: <span class="hljs-literal">false</span>,
  <span class="hljs-attr">&quot;reason&quot;</span>: <span class="hljs-literal">null</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2012-10-06T21:34:12Z&quot;</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/example/subscription&quot;</span>,
  <span class="hljs-attr">&quot;repository_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/example&quot;</span>
}
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-a-repository-subscription" class="pt-3">
      <a href="#delete-a-repository-subscription">Delete a repository subscription</a>
    </h3>
    <p>This endpoint should only be used to stop watching a repository. To control whether or not you wish to receive notifications from a repository, <a href="https://developer.github.com/v3/activity/watching/#set-a-repository-subscription">set the repository&apos;s subscription manually</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /repos/{owner}/{repo}/subscription</code></pre>
  <div>
    
      <h4 id="delete-a-repository-subscription--parameters">
        <a href="#delete-a-repository-subscription--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>owner</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>repo</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-a-repository-subscription--code-samples">
        <a href="#delete-a-repository-subscription--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/repos/octocat/hello-world/subscription
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /repos/{owner}/{repo}/subscription&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-repositories-watched-by-the-authenticated-user" class="pt-3">
      <a href="#list-repositories-watched-by-the-authenticated-user">List repositories watched by the authenticated user</a>
    </h3>
    <p>Lists repositories the authenticated user is watching.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /user/subscriptions</code></pre>
  <div>
    
      <h4 id="list-repositories-watched-by-the-authenticated-user--parameters">
        <a href="#list-repositories-watched-by-the-authenticated-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-repositories-watched-by-the-authenticated-user--code-samples">
        <a href="#list-repositories-watched-by-the-authenticated-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/user/subscriptions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /user/subscriptions&apos;</span>)
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
    <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;owner&quot;</span>: {
      <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
      <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
      <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
      <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
      <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
      <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
      <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
      <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
      <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
      <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
      <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
      <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
    },
    <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
    <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
    <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
    <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
    <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
    <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
    <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
    <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
    <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
    <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
    <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
    <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
    <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
    <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
    <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
    <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
    <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
    <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
    <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
    <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
    <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
    <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
    <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
    <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
    <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
    <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
    <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;clone_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;mirror_url&quot;</span>: <span class="hljs-string">&quot;git:git.example.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;hooks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/hooks&quot;</span>,
    <span class="hljs-attr">&quot;svn_url&quot;</span>: <span class="hljs-string">&quot;https://svn.github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;homepage&quot;</span>: <span class="hljs-string">&quot;https://github.com&quot;</span>,
    <span class="hljs-attr">&quot;language&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;forks_count&quot;</span>: <span class="hljs-number">9</span>,
    <span class="hljs-attr">&quot;stargazers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;watchers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;size&quot;</span>: <span class="hljs-number">108</span>,
    <span class="hljs-attr">&quot;default_branch&quot;</span>: <span class="hljs-string">&quot;master&quot;</span>,
    <span class="hljs-attr">&quot;open_issues_count&quot;</span>: <span class="hljs-number">0</span>,
    <span class="hljs-attr">&quot;is_template&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;topics&quot;</span>: [
      <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-string">&quot;atom&quot;</span>,
      <span class="hljs-string">&quot;electron&quot;</span>,
      <span class="hljs-string">&quot;api&quot;</span>
    ],
    <span class="hljs-attr">&quot;has_issues&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_projects&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_wiki&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_pages&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;has_downloads&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;archived&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;disabled&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;visibility&quot;</span>: <span class="hljs-string">&quot;public&quot;</span>,
    <span class="hljs-attr">&quot;pushed_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:06:43Z&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:01:12Z&quot;</span>,
    <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:14:43Z&quot;</span>,
    <span class="hljs-attr">&quot;permissions&quot;</span>: {
      <span class="hljs-attr">&quot;admin&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;push&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;pull&quot;</span>: <span class="hljs-literal">true</span>
    },
    <span class="hljs-attr">&quot;template_repository&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;temp_clone_token&quot;</span>: <span class="hljs-string">&quot;ABTLWHOULUVAXGTRYU7OC2876QJ2O&quot;</span>,
    <span class="hljs-attr">&quot;delete_branch_on_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;subscribers_count&quot;</span>: <span class="hljs-number">42</span>,
    <span class="hljs-attr">&quot;network_count&quot;</span>: <span class="hljs-number">0</span>,
    <span class="hljs-attr">&quot;license&quot;</span>: {
      <span class="hljs-attr">&quot;key&quot;</span>: <span class="hljs-string">&quot;mit&quot;</span>,
      <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;MIT License&quot;</span>,
      <span class="hljs-attr">&quot;spdx_id&quot;</span>: <span class="hljs-string">&quot;MIT&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/licenses/mit&quot;</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDc6TGljZW5zZW1pdA==&quot;</span>
    }
  }
]
</code></pre></div>
    
    
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-repositories-watched-by-a-user" class="pt-3">
      <a href="#list-repositories-watched-by-a-user">List repositories watched by a user</a>
    </h3>
    <p>Lists repositories a user is watching.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /users/{username}/subscriptions</code></pre>
  <div>
    
      <h4 id="list-repositories-watched-by-a-user--parameters">
        <a href="#list-repositories-watched-by-a-user--parameters">Parameters</a>
      </h4>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>In</th>
            <th>Description</th>
          </tr>
        </thead>
        <tbody>
          
            <tr>
              <td><code>accept</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">header</td>
              <td class="opacity-70">
                <p>Setting to <code>application/vnd.github.v3+json</code> is recommended</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>username</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>per_page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Results per page (max 100)</p>
                
              </td>
            </tr>
          
            <tr>
              <td><code>page</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Page number of the results to fetch.</p>
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="list-repositories-watched-by-a-user--code-samples">
        <a href="#list-repositories-watched-by-a-user--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.v3+json&quot; \
  https://api.github.com/users/USERNAME/subscriptions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /users/{username}/subscriptions&apos;</span>, {
  <span class="hljs-attr">username</span>: <span class="hljs-string">&apos;username&apos;</span>
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1296269</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDEwOlJlcG9zaXRvcnkxMjk2MjY5&quot;</span>,
    <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;full_name&quot;</span>: <span class="hljs-string">&quot;octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;owner&quot;</span>: {
      <span class="hljs-attr">&quot;login&quot;</span>: <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDQ6VXNlcjE=&quot;</span>,
      <span class="hljs-attr">&quot;avatar_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/images/error/octocat_happy.gif&quot;</span>,
      <span class="hljs-attr">&quot;gravatar_id&quot;</span>: <span class="hljs-string">&quot;&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat&quot;</span>,
      <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat&quot;</span>,
      <span class="hljs-attr">&quot;followers_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/followers&quot;</span>,
      <span class="hljs-attr">&quot;following_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/following{/other_user}&quot;</span>,
      <span class="hljs-attr">&quot;gists_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/gists{/gist_id}&quot;</span>,
      <span class="hljs-attr">&quot;starred_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/starred{/owner}{/repo}&quot;</span>,
      <span class="hljs-attr">&quot;subscriptions_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/subscriptions&quot;</span>,
      <span class="hljs-attr">&quot;organizations_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/orgs&quot;</span>,
      <span class="hljs-attr">&quot;repos_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/repos&quot;</span>,
      <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/events{/privacy}&quot;</span>,
      <span class="hljs-attr">&quot;received_events_url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/users/octocat/received_events&quot;</span>,
      <span class="hljs-attr">&quot;type&quot;</span>: <span class="hljs-string">&quot;User&quot;</span>,
      <span class="hljs-attr">&quot;site_admin&quot;</span>: <span class="hljs-literal">false</span>
    },
    <span class="hljs-attr">&quot;private&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;html_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;description&quot;</span>: <span class="hljs-string">&quot;This your first repo!&quot;</span>,
    <span class="hljs-attr">&quot;fork&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;archive_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/{archive_format}{/ref}&quot;</span>,
    <span class="hljs-attr">&quot;assignees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/assignees{/user}&quot;</span>,
    <span class="hljs-attr">&quot;blobs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/blobs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;branches_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/branches{/branch}&quot;</span>,
    <span class="hljs-attr">&quot;collaborators_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/collaborators{/collaborator}&quot;</span>,
    <span class="hljs-attr">&quot;comments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;compare_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/compare/{base}...{head}&quot;</span>,
    <span class="hljs-attr">&quot;contents_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contents/{+path}&quot;</span>,
    <span class="hljs-attr">&quot;contributors_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/contributors&quot;</span>,
    <span class="hljs-attr">&quot;deployments_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/deployments&quot;</span>,
    <span class="hljs-attr">&quot;downloads_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/downloads&quot;</span>,
    <span class="hljs-attr">&quot;events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/events&quot;</span>,
    <span class="hljs-attr">&quot;forks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/forks&quot;</span>,
    <span class="hljs-attr">&quot;git_commits_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/commits{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_refs_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/refs{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/tags{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;git_url&quot;</span>: <span class="hljs-string">&quot;git:github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;issue_comment_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/comments{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issue_events_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues/events{/number}&quot;</span>,
    <span class="hljs-attr">&quot;issues_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/issues{/number}&quot;</span>,
    <span class="hljs-attr">&quot;keys_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/keys{/key_id}&quot;</span>,
    <span class="hljs-attr">&quot;labels_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/labels{/name}&quot;</span>,
    <span class="hljs-attr">&quot;languages_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/languages&quot;</span>,
    <span class="hljs-attr">&quot;merges_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/merges&quot;</span>,
    <span class="hljs-attr">&quot;milestones_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/milestones{/number}&quot;</span>,
    <span class="hljs-attr">&quot;notifications_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/notifications{?since,all,participating}&quot;</span>,
    <span class="hljs-attr">&quot;pulls_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/pulls{/number}&quot;</span>,
    <span class="hljs-attr">&quot;releases_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/releases{/id}&quot;</span>,
    <span class="hljs-attr">&quot;ssh_url&quot;</span>: <span class="hljs-string">&quot;git@github.com:octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;stargazers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/stargazers&quot;</span>,
    <span class="hljs-attr">&quot;statuses_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/statuses/{sha}&quot;</span>,
    <span class="hljs-attr">&quot;subscribers_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscribers&quot;</span>,
    <span class="hljs-attr">&quot;subscription_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/subscription&quot;</span>,
    <span class="hljs-attr">&quot;tags_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/tags&quot;</span>,
    <span class="hljs-attr">&quot;teams_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/teams&quot;</span>,
    <span class="hljs-attr">&quot;trees_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/git/trees{/sha}&quot;</span>,
    <span class="hljs-attr">&quot;clone_url&quot;</span>: <span class="hljs-string">&quot;https://github.com/octocat/Hello-World.git&quot;</span>,
    <span class="hljs-attr">&quot;mirror_url&quot;</span>: <span class="hljs-string">&quot;git:git.example.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;hooks_url&quot;</span>: <span class="hljs-string">&quot;http://api.github.com/repos/octocat/Hello-World/hooks&quot;</span>,
    <span class="hljs-attr">&quot;svn_url&quot;</span>: <span class="hljs-string">&quot;https://svn.github.com/octocat/Hello-World&quot;</span>,
    <span class="hljs-attr">&quot;homepage&quot;</span>: <span class="hljs-string">&quot;https://github.com&quot;</span>,
    <span class="hljs-attr">&quot;language&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;forks_count&quot;</span>: <span class="hljs-number">9</span>,
    <span class="hljs-attr">&quot;stargazers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;watchers_count&quot;</span>: <span class="hljs-number">80</span>,
    <span class="hljs-attr">&quot;size&quot;</span>: <span class="hljs-number">108</span>,
    <span class="hljs-attr">&quot;default_branch&quot;</span>: <span class="hljs-string">&quot;master&quot;</span>,
    <span class="hljs-attr">&quot;open_issues_count&quot;</span>: <span class="hljs-number">0</span>,
    <span class="hljs-attr">&quot;is_template&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;topics&quot;</span>: [
      <span class="hljs-string">&quot;octocat&quot;</span>,
      <span class="hljs-string">&quot;atom&quot;</span>,
      <span class="hljs-string">&quot;electron&quot;</span>,
      <span class="hljs-string">&quot;api&quot;</span>
    ],
    <span class="hljs-attr">&quot;has_issues&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_projects&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_wiki&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;has_pages&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;has_downloads&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;archived&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;disabled&quot;</span>: <span class="hljs-literal">false</span>,
    <span class="hljs-attr">&quot;visibility&quot;</span>: <span class="hljs-string">&quot;public&quot;</span>,
    <span class="hljs-attr">&quot;pushed_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:06:43Z&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:01:12Z&quot;</span>,
    <span class="hljs-attr">&quot;updated_at&quot;</span>: <span class="hljs-string">&quot;2011-01-26T19:14:43Z&quot;</span>,
    <span class="hljs-attr">&quot;permissions&quot;</span>: {
      <span class="hljs-attr">&quot;admin&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;push&quot;</span>: <span class="hljs-literal">false</span>,
      <span class="hljs-attr">&quot;pull&quot;</span>: <span class="hljs-literal">true</span>
    },
    <span class="hljs-attr">&quot;template_repository&quot;</span>: <span class="hljs-literal">null</span>,
    <span class="hljs-attr">&quot;temp_clone_token&quot;</span>: <span class="hljs-string">&quot;ABTLWHOULUVAXGTRYU7OC2876QJ2O&quot;</span>,
    <span class="hljs-attr">&quot;delete_branch_on_merge&quot;</span>: <span class="hljs-literal">true</span>,
    <span class="hljs-attr">&quot;subscribers_count&quot;</span>: <span class="hljs-number">42</span>,
    <span class="hljs-attr">&quot;network_count&quot;</span>: <span class="hljs-number">0</span>,
    <span class="hljs-attr">&quot;license&quot;</span>: {
      <span class="hljs-attr">&quot;key&quot;</span>: <span class="hljs-string">&quot;mit&quot;</span>,
      <span class="hljs-attr">&quot;name&quot;</span>: <span class="hljs-string">&quot;MIT License&quot;</span>,
      <span class="hljs-attr">&quot;spdx_id&quot;</span>: <span class="hljs-string">&quot;MIT&quot;</span>,
      <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/licenses/mit&quot;</span>,
      <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDc6TGljZW5zZW1pdA==&quot;</span>
    }
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
  </div>
  <hr>
</div>
    </div>
    </div>
  </article>
  <div class="d-block d-xl-none border-top border-gray-light mt-4 markdown-body">
    
    <form class="js-helpfulness mt-4 f5" id="helpfulness-sm">
  <h4
    data-help-start
    data-help-yes
    data-help-no
  >
    Did this doc help you?
  </h4>
  <p
    class="radio-group"
    data-help-start
    data-help-yes
    data-help-no
  >
    <input
      hidden
      id="helpfulness-yes-sm"
      type="radio"
      name="helpfulness-vote"
      value="Yes"
      aria-label="Yes"
    />
    <label class="btn x-radio-label" for="helpfulness-yes-sm">
      <svg version="1.1" width="24" height="24" viewBox="0 0 24 24" class="octicon octicon-thumbsup" aria-hidden="true"><path fill-rule="evenodd" d="M12.596 2.043c-1.301-.092-2.303.986-2.303 2.206v1.053c0 2.666-1.813 3.785-2.774 4.2a1.866 1.866 0 01-.523.131A1.75 1.75 0 005.25 8h-1.5A1.75 1.75 0 002 9.75v10.5c0 .967.784 1.75 1.75 1.75h1.5a1.75 1.75 0 001.742-1.58c.838.06 1.667.296 2.69.586l.602.17c1.464.406 3.213.824 5.544.824 2.188 0 3.693-.204 4.583-1.372.422-.554.65-1.255.816-2.05.148-.708.262-1.57.396-2.58l.051-.39c.319-2.386.328-4.18-.223-5.394-.293-.644-.743-1.125-1.355-1.431-.59-.296-1.284-.404-2.036-.404h-2.05l.056-.429c.025-.18.05-.372.076-.572.06-.483.117-1.006.117-1.438 0-1.245-.222-2.253-.92-2.941-.684-.675-1.668-.88-2.743-.956zM7 18.918c1.059.064 2.079.355 3.118.652l.568.16c1.406.39 3.006.77 5.142.77 2.277 0 3.004-.274 3.39-.781.216-.283.388-.718.54-1.448.136-.65.242-1.45.379-2.477l.05-.384c.32-2.4.253-3.795-.102-4.575-.16-.352-.375-.568-.66-.711-.305-.153-.74-.245-1.365-.245h-2.37c-.681 0-1.293-.57-1.211-1.328.026-.243.065-.537.105-.834l.07-.527c.06-.482.105-.921.105-1.25 0-1.125-.213-1.617-.473-1.873-.275-.27-.774-.455-1.795-.528-.351-.024-.698.274-.698.71v1.053c0 3.55-2.488 5.063-3.68 5.577-.372.16-.754.232-1.113.26v7.78zM3.75 20.5a.25.25 0 01-.25-.25V9.75a.25.25 0 01.25-.25h1.5a.25.25 0 01.25.25v10.5a.25.25 0 01-.25.25h-1.5z"></path></svg>
    </label>
    <input
      hidden
      id="helpfulness-no-sm"
      type="radio"
      name="helpfulness-vote"
      value="No"
      aria-label="No"
    />
    <label class="btn x-radio-label" for="helpfulness-no-sm">
      <svg version="1.1" width="24" height="24" viewBox="0 0 24 24" class="octicon octicon-thumbsdown" aria-hidden="true"><path fill-rule="evenodd" d="M12.596 21.957c-1.301.092-2.303-.986-2.303-2.206v-1.053c0-2.666-1.813-3.785-2.774-4.2a1.864 1.864 0 00-.523-.13A1.75 1.75 0 015.25 16h-1.5A1.75 1.75 0 012 14.25V3.75C2 2.784 2.784 2 3.75 2h1.5a1.75 1.75 0 011.742 1.58c.838-.06 1.667-.296 2.69-.586l.602-.17C11.748 2.419 13.497 2 15.828 2c2.188 0 3.693.204 4.583 1.372.422.554.65 1.255.816 2.05.148.708.262 1.57.396 2.58l.051.39c.319 2.386.328 4.18-.223 5.394-.293.644-.743 1.125-1.355 1.431-.59.296-1.284.404-2.036.404h-2.05l.056.429c.025.18.05.372.076.572.06.483.117 1.006.117 1.438 0 1.245-.222 2.253-.92 2.942-.684.674-1.668.879-2.743.955zM7 5.082c1.059-.064 2.079-.355 3.118-.651.188-.054.377-.108.568-.16 1.406-.392 3.006-.771 5.142-.771 2.277 0 3.004.274 3.39.781.216.283.388.718.54 1.448.136.65.242 1.45.379 2.477l.05.385c.32 2.398.253 3.794-.102 4.574-.16.352-.375.569-.66.711-.305.153-.74.245-1.365.245h-2.37c-.681 0-1.293.57-1.211 1.328.026.244.065.537.105.834l.07.527c.06.482.105.922.105 1.25 0 1.125-.213 1.617-.473 1.873-.275.27-.774.456-1.795.528-.351.024-.698-.274-.698-.71v-1.053c0-3.55-2.488-5.063-3.68-5.577A3.485 3.485 0 007 12.861V5.08zM3.75 3.5a.25.25 0 00-.25.25v10.5c0 .138.112.25.25.25h1.5a.25.25 0 00.25-.25V3.75a.25.25 0 00-.25-.25h-1.5z"></path></svg>
    </label>
  </p>
  <p class="text-gray f6" hidden data-help-yes>
    Want to learn about new docs features and updates? Sign up for updates!
  </p>
  <p class="text-gray f6" hidden data-help-no>
    We're continually improving our docs. We'd love to hear how we can do better.
  </p>
  <input
    type="text"
    class="d-none"
    name="helpfulness-token"
    aria-hidden="true"
  />
  <p hidden data-help-no>
    <label
      class="d-block mb-1 f6"
      for="helpfulness-category-sm"
    >
      What problem did you have?
      <span class="text-normal text-gray-light float-right ml-1">
        Required
      </span>
    </label>
    <select
      class="form-control select-sm width-full"
      name="helpfulness-category"
      id="helpfulness-category-sm"
    >
      <option value="">
        Choose an option
      </option>
      <option value="Unclear">
        Information was unclear
      </option>
      <option value="Confusing">
        The content was confusing
      </option>
      <option value="Unhelpful">
        The article didn't answer my question
      </option>
      <option value="Other">
        Other
      </option>
    </select>
  </p>
  <p hidden data-help-no>
    <label
      class="d-block mb-1 f6"
      for="helpfulness-comment-sm"
    >
      <span>Let us know what we can do better</span>
      <span class="text-normal text-gray-light float-right ml-1">
        Optional
      </span>
    </label>
    <textarea
      class="form-control input-sm width-full"
      name="helpfulness-comment"
      id="helpfulness-comment-sm"
    ></textarea>
  </p>
  <p>
    <label
      class="d-block mb-1 f6"
      for="helpfulness-email-sm"
      hidden
      data-help-no
    >
      Can we contact you if we have more questions?
      <span class="text-normal text-gray-light float-right ml-1">
        Optional
      </span>
    </label>
    <input
      type="email"
      class="form-control input-sm width-full"
      name="helpfulness-email"
      id="helpfulness-email-sm"
      placeholder="email@example.com"
      hidden
      data-help-yes
      data-help-no
    />
  </p>
  <p
    class="text-right"
    hidden
    data-help-yes
    data-help-no
  >
    <button type="submit" class="btn btn-blue btn-sm">
      Send
    </button>
  </p>
  <p class="text-gray f6" hidden data-help-end>
    Thank you! Your feedback has been submitted.
  </p>
</form>

  </div>
</main>

      
      <!-- Contact support banner -->
<section class="mt-lg-9 py-7 no-print" style="background-color: #fafbfc;">
  <div class="container-xl px-3 px-md-6">
    <div class="gutter gutter-xl-spacious d-md-flex">
      <div class="col-12 col-md-5">
        <h3 class="text-mono f5 text-normal text-gray mb-2">Ask a human</h3>
        <h4 class="h2-mktg mb-3">Can't find what you're looking for?</h4>
        <a id="contact-us" class="btn-mktg" href="https://support.github.com/contact">Contact us</a>
      </div>
      <div class="col-12 col-md-3 ml-md-6 ml-lg-0 position-relative">
        <svg class="position-relative position-md-absolute right-n8 right-lg-8 top-lg-3" width="240px" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 518.6 406.73"><g data-name="Layer 2"><g id="Layer_5" data-name="Layer 5"><ellipse cx="383.17" cy="369.46" rx="135.42" ry="16.07" style="fill:#cacfd6"/><path d="M37.2,399.86c0,2.37-5.37,3.88-11.75,4.2-.8,1.61-8,2.67-11,2.67C4.83,406.73,1,405,1,401.56s7.82-6.2,17.46-6.2S37.2,396.44,37.2,399.86Z" style="fill:#cacfd6"/><path d="M143.92,281.81c1.81,9.57,6.21,49.92,6.47,56.9s8.53,9.05,14.74,7.76,12-8.33,12.4-11.38-14.89-45.84-15.83-51.72S143.92,281.81,143.92,281.81Z" style="fill:#ffd33d"/><path d="M161.7,283.37a2.85,2.85,0,0,0-1.25-1.89c-3.72,7.66,3.41,23.7,4.1,30.94.5,5.19,3.64,7.34,4.91,11.94,1,3.66-.55,6.28.94,10a31.71,31.71,0,0,0,3.88,6.26c1.86-2.07,3.09-4.19,3.25-5.54C177.89,332,162.64,289.25,161.7,283.37Z" style="fill:#f9c513"/><path d="M153.38,344.41a11.86,11.86,0,0,0,5,2.12c-.48-4.14-1-8.27-1.35-12.43-.64-7.93-2.45-14.05-4.37-21.63-2.28-9.06,1.28-22.79-5.76-29.84a2.45,2.45,0,0,1-.72-1.33l-.92.2C149.47,302.17,150.89,323.38,153.38,344.41Z" style="fill:#ffdf5d"/><path d="M159,293a19.71,19.71,0,0,0,4.63-2.76,69.71,69.71,0,0,1-1.91-6.91c-.95-5.89-17.78-1.56-17.78-1.56.45,2.37,1.06,6.62,1.72,11.77C150,293.59,154.58,294.86,159,293Z" style="fill:#dbab09"/><path d="M83.21,191.25c-1.41.23-10.64-3.83-14.89,12.37s19.51,51.52,43.93,66.81c24.66,15.43,40.79,19.51,46.48,15.73,5.1-8.31,4.88-14,4.88-14s-34-14.65-51.68-30.48C93.52,225.21,83.21,191.25,83.21,191.25Z" style="fill:#dbab09;stroke:#ffea7f;stroke-miterlimit:10;stroke-width:2.5px"/><path d="M110.23,258.74c14.48,11.6,35.06,22.76,53.38,22.74,3.62-6.92,0-9.34,0-9.34s-34-14.65-51.68-30.48c-18.41-16.45-28.72-50.41-28.72-50.41-1,.17-6.12-1.9-10.47,3C70.82,220.26,91.37,243.62,110.23,258.74Z" style="fill:#b08800"/><path d="M192.9,23.26s-57.09,34.68-50.3,91.31c6,49.64,36.15,66.44,46.89,77.58,3.08,6.49-.7,19.08,9.17,26.63s32.14,13.23,61.56.51,56.06-29.82,67.6-54.47" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M202,52.3s-10.83-9.67-8.84-37.39a2.22,2.22,0,0,1,1.1-1.76c23.2-13.26,49-10.74,70.16,3.37" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M236.06,5.22C262.59-4,309.23,1.88,337.36,46.34c24.65,39,14.43,77.55,8.32,98.32" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M203.93,19.65s16.43,0,26.13,6.68c11.74,8.09,16.16,20.8,16.16,20.8" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M148.76,74.46s-26.39,9.14-32.15,27.43c7,11.53,21.41,22,27.49,22.16" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M142.48,113.44s3.61,19,22.59,17.05c28.31-7.16,29.24-57.72,68.06-56,37.09,1.62,42.8,51.75,47.11,65.77,3,9.78,16.49,8,22.51,11.73,10.11,6.35,10.11,29.3-8.74,49.35" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M332.09,156.9s15,82.09,14.35,93.44" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M370.63,245.84s-29.95-4.67-38.54,22.2,15.19,42.07,27.64,48.29c6.55,3.28,25.37,10.51,28.26,7.79,5.43-7.16-3.68-24.71-.59-42.84.78-4.54,6-13.45,6-17-13.88-42.95-40.28-125.12-59.87-129.63-7.52-1.73-5.66,16.81-5.66,16.81" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M317,181.44s3.88,25.34,13.06,42.32,16.43,26.58,16.43,26.58" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M321.17,176.08s7.46,25.2,13.42,34.37a145.35,145.35,0,0,1,9.29,16.36" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M320.14,196.63s-1.83,42.27,2.64,49.62" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M329.55,246.25s-26.16,1.83-27.53,26.61S317,301.31,317,301.31s-2.57,12.39,4.33,13.31,5.8-16.72,6.54-20.77a8.56,8.56,0,0,1,3.88-5.43" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M371.09,199.22s29.15,15.41,56,23.24,28.14,23.45,28.78,31.15-4.43,27.15-44.56,50.17" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M327.61,308s4.5,17.47,8,22.9" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M359.73,316.33s5,21.14,9.64,27" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M346.8,329.59a41.34,41.34,0,0,0-14.71,2.83s-16.35,6.6-19.23,27.95" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M313.56,356.51s-13.67-2-14.45,3.86.47,8.93,22.17,10.1,28.29,0,28.29,0" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M403.67,341.35a57.23,57.23,0,0,0-19.94-.39c-34.16,5.05-34.16,27.56-34.16,27.56s-5.69,7.77-.77,12.43a159,159,0,0,0,61.33,3.88c33-4.66,38-9.45,37.65-13.85s-7.93-5.9-7.93-5.9-2.4-10.22-19.23-40.36-11.41-44.27-11.41-44.27" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M353.51,382.15s2.8-12.65,11-12.65,29.87,17.38,40.49,15.95" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M309.49,189.18s-30.69,37.39-47.69,50.94c-15.05,12-60.2,33.28-84.27,32" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M187.77,285.77s.78-8.41-10.24-13.69" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M191.91,315.31c7.18-4.25,15.13-11.53,13.18-18.83-3.16-11.8-23.24-13.83-30.65-10.71-9.22,3.89-10.83,10.72-28.8,7.81" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M127.16,280.45s-9.9,30.5,5.92,45.45c16.14,15.27,61.21,20.63,61.2-27.57" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M175.32,198.57c5.76-.65,15.5-.65,15.5-.65" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><line x1="180.45" y1="208.08" x2="190.82" y2="202.95" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M295.63,139.3c7.55-12.7,22.25-31.34,32-37.48" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M305.37,142.27c5.29-4.87,15.8-14.22,35.78-22.06" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M186.69,325.8c51.66-20.24,113.43-76.49,134.25-123.21" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><ellipse cx="237.8" cy="275.53" rx="27.12" ry="8.66" transform="translate(-117.06 194.69) rotate(-36.37)" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><ellipse cx="287" cy="234.24" rx="22.62" ry="5.51" transform="translate(-76.62 305.08) rotate(-50.24)" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><ellipse cx="211.18" cy="196.36" rx="6.49" ry="3.48" transform="translate(-26.64 33.29) rotate(-8.48)" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M81.8,191.48S77.29,216.79,106.89,247c17.77,18.13,38.66,26.73,55.18,28.42,2.36-10.8,8.36-32.57-41.26-70C98.2,188.35,81.8,191.48,81.8,191.48Z" style="fill:#79b8ff"/><path d="M113.34,233.44a126.89,126.89,0,0,1-17.09-39.58,50.66,50.66,0,0,0-5.58-1.08c1,25,18.7,45.33,32.52,66.28,2.2,1.39,4.4,2.63,6.61,3.8A243.42,243.42,0,0,0,113.34,233.44Z" style="fill:#c8e1ff"/><path d="M87.23,191.32a24.34,24.34,0,0,0-5.43.16s-4.51,25.3,25.09,55.51c1.29,1.32,2.61,2.57,3.93,3.79C99.22,232.1,87.16,213.47,87.23,191.32Z" style="fill:#2188ff"/><path d="M153.93,254.73c-5-8.36-5.32-18.19-10.2-26.44-7.84-13.25-22.34-20-29.67-33.46-1.32-.49-2.61-.93-3.83-1.28,6.64,16,24.93,23.35,31.59,39.39,3.86,9.29,5.26,19,10.11,28a119.75,119.75,0,0,0,7.1,11.4c.9.29,1.78.54,2.67.8A87.18,87.18,0,0,0,153.93,254.73Z" style="fill:#daedff"/><path d="M68.32,203.62l2.8-13.2L96.3,170.55s36.38-3.91,65.6,30.76c25.47,30.22,24.58,44.25,25.64,58,0,0-18.72,24.65-29.89,28.26,7.4-22,10.3-34.8-22.44-66.09S77.83,185.18,68.32,203.62Z" style="fill:#ffea7f"/><path d="M178.75,225.08c.47,10.69,2.66,21.7-2.37,31.37-3.59,6.91-9.84,11.23-12.22,19-1.08,3.53-1.47,7.08-2.82,10.32,11.17-6.71,26.2-26.5,26.2-26.5C186.81,249.78,187,240.13,178.75,225.08Z" style="fill:#ffdf5d"/><path d="M389.9,253.67s4.18-5.22,9.62-7.42" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:3px"/><path d="M159.4,221.29c-7.51-13-18.33-25.17-31.76-32.3s-29.35-9.27-44.5-8.05l-5.23,4.13a51.11,51.11,0,0,1,17.74-.36c9.76,1.31,20.25,4.38,28.34,10.19,11.72,8.42,20.29,20.47,29.33,31.5,8.06,9.83,14.91,21,16.74,33.83a74.77,74.77,0,0,1,0,18.67c2-1.84,3.91-3.81,5.74-5.78C173.39,255,168.53,237.16,159.4,221.29Z" style="fill:#fff5b1"/><path d="M320.08,108a48.4,48.4,0,0,1,7.53-6.17" style="fill:none;stroke:#0366d6;stroke-linecap:round;stroke-miterlimit:10;stroke-width:3px"/><path d="M330.79,124.79c3.38-1.68,6.85-3.2,10.36-4.58" style="fill:none;stroke:#0366d6;stroke-linecap:round;stroke-miterlimit:10;stroke-width:3px"/><path d="M175.32,198.57c1.86-.2,3.72-.32,5.59-.41" style="fill:none;stroke:#0366d6;stroke-linecap:round;stroke-miterlimit:10;stroke-width:3px"/><path d="M180.45,208.08l3.65-1.8" style="fill:none;stroke:#0366d6;stroke-linecap:round;stroke-miterlimit:10;stroke-width:3px"/><ellipse cx="18.39" cy="393.7" rx="11.22" ry="6.17" style="fill:#0366d6"/><path d="M1,401.38c.45-3.84,5.43-5.54,9-5.53" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:2px"/><path d="M21.25,398.3s4.54.9,4.2,7" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:2px"/><path d="M26.67,395.36a10.33,10.33,0,0,1,5.14,1.7,7.12,7.12,0,0,1,3,4.74" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:2px"/><path d="M28.26,392.65s1.13-1.69,5.47-1.41a7.41,7.41,0,0,1,6.05,4.12" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:2px"/><path d="M2.93,393.7a9.52,9.52,0,0,1,4.24-3,13.51,13.51,0,0,1,5.8.2" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:2px"/><path d="M18.27,388.53s-1.53-2.08-3.58-1.7c-1.5.28-1.72,4-1.72,4" style="fill:none;stroke:#0366d6;stroke-miterlimit:10;stroke-width:2px"/></g></g></svg>
      </div>
    </div>
  </div>
</section>

      <div class="footer mt-6 no-print">
  <div class="container-xl px-3 px-md-6">
    <div class="d-flex flex-wrap py-5 mb-5">
      <div class="col-12 col-lg-4 mb-5">
        <a href="https://github.com/" data-ga-click="Footer, go to home, text:home" class="text-gray-dark" aria-label="Go to GitHub homepage">
          <svg version="1.1" width="84.375" height="30" viewBox="0 0 45 16" class="octicon octicon-logo-github" aria-hidden="true"><path fill-rule="evenodd" d="M18.53 12.03h-.02c.009 0 .015.01.024.011h.006l-.01-.01zm.004.011c-.093.001-.327.05-.574.05-.78 0-1.05-.36-1.05-.83V8.13h1.59c.09 0 .16-.08.16-.19v-1.7c0-.09-.08-.17-.16-.17h-1.59V3.96c0-.08-.05-.13-.14-.13h-2.16c-.09 0-.14.05-.14.13v2.17s-1.09.27-1.16.28c-.08.02-.13.09-.13.17v1.36c0 .11.08.19.17.19h1.11v3.28c0 2.44 1.7 2.69 2.86 2.69.53 0 1.17-.17 1.27-.22.06-.02.09-.09.09-.16v-1.5a.177.177 0 00-.146-.18zM42.23 9.84c0-1.81-.73-2.05-1.5-1.97-.6.04-1.08.34-1.08.34v3.52s.49.34 1.22.36c1.03.03 1.36-.34 1.36-2.25zm2.43-.16c0 3.43-1.11 4.41-3.05 4.41-1.64 0-2.52-.83-2.52-.83s-.04.46-.09.52c-.03.06-.08.08-.14.08h-1.48c-.1 0-.19-.08-.19-.17l.02-11.11c0-.09.08-.17.17-.17h2.13c.09 0 .17.08.17.17v3.77s.82-.53 2.02-.53l-.01-.02c1.2 0 2.97.45 2.97 3.88zm-8.72-3.61h-2.1c-.11 0-.17.08-.17.19v5.44s-.55.39-1.3.39-.97-.34-.97-1.09V6.25c0-.09-.08-.17-.17-.17h-2.14c-.09 0-.17.08-.17.17v5.11c0 2.2 1.23 2.75 2.92 2.75 1.39 0 2.52-.77 2.52-.77s.05.39.08.45c.02.05.09.09.16.09h1.34c.11 0 .17-.08.17-.17l.02-7.47c0-.09-.08-.17-.19-.17zm-23.7-.01h-2.13c-.09 0-.17.09-.17.2v7.34c0 .2.13.27.3.27h1.92c.2 0 .25-.09.25-.27V6.23c0-.09-.08-.17-.17-.17zm-1.05-3.38c-.77 0-1.38.61-1.38 1.38 0 .77.61 1.38 1.38 1.38.75 0 1.36-.61 1.36-1.38 0-.77-.61-1.38-1.36-1.38zm16.49-.25h-2.11c-.09 0-.17.08-.17.17v4.09h-3.31V2.6c0-.09-.08-.17-.17-.17h-2.13c-.09 0-.17.08-.17.17v11.11c0 .09.09.17.17.17h2.13c.09 0 .17-.08.17-.17V8.96h3.31l-.02 4.75c0 .09.08.17.17.17h2.13c.09 0 .17-.08.17-.17V2.6c0-.09-.08-.17-.17-.17zM8.81 7.35v5.74c0 .04-.01.11-.06.13 0 0-1.25.89-3.31.89-2.49 0-5.44-.78-5.44-5.92S2.58 1.99 5.1 2c2.18 0 3.06.49 3.2.58.04.05.06.09.06.14L7.94 4.5c0 .09-.09.2-.2.17-.36-.11-.9-.33-2.17-.33-1.47 0-3.05.42-3.05 3.73s1.5 3.7 2.58 3.7c.92 0 1.25-.11 1.25-.11v-2.3H4.88c-.11 0-.19-.08-.19-.17V7.35c0-.09.08-.17.19-.17h3.74c.11 0 .19.08.19.17z"></path></svg>
        </a>
      </div>
      <div class="col-6 col-sm-3 col-lg-2 mb-6 mb-md-2 pr-3 pr-lg-0 pl-lg-4">
        <h4 class="mb-3 text-mono text-gray-light text-normal">Product</h4>
        <ul class="list-style-none text-gray f5">
          <li class="lh-condensed mb-3"><a href="https://github.com/features" data-ga-click="Footer, go to features, text:features" class="link-gray">Features</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.com/security" data-ga-click="Footer, go to security, text:security" class="link-gray">Security</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.com/enterprise" data-ga-click="Footer, go to enterprise, text:enterprise" class="link-gray">Enterprise</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.com/case-studies?type=customers" data-ga-click="Footer, go to case studies, text:case studies" class="link-gray">Case Studies</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.com/pricing" data-ga-click="Footer, go to pricing, text:pricing" class="link-gray">Pricing</a></li>
          <li class="lh-condensed mb-3"><a href="https://resources.github.com" data-ga-click="Footer, go to resources, text:resources" class="link-gray">Resources</a></li>
        </ul>
      </div>
      <div class="col-6 col-sm-3 col-lg-2 mb-6 mb-md-2 pr-3 pr-md-0 pl-md-4">
        <h4 class="mb-3 text-mono text-gray-light text-normal">Platform</h4>
        <ul class="list-style-none f5">
          <li class="lh-condensed mb-3"><a href="https://developer.github.com/" data-ga-click="Footer, go to api, text:api" class="link-gray">Developer API</a></li>
          <li class="lh-condensed mb-3"><a href="http://partner.github.com/" data-ga-click="Footer, go to partner, text:partner" class="link-gray">Partners</a></li>
          <li class="lh-condensed mb-3"><a href="https://atom.io" data-ga-click="Footer, go to atom, text:atom" class="link-gray">Atom</a></li>
          <li class="lh-condensed mb-3"><a href="http://electron.atom.io/" data-ga-click="Footer, go to electron, text:electron" class="link-gray">Electron</a></li>
          <li class="lh-condensed mb-3"><a href="https://desktop.github.com/" data-ga-click="Footer, go to desktop, text:desktop" class="link-gray">GitHub Desktop</a></li>
        </ul>
      </div>
      <div class="col-6 col-sm-3 col-lg-2 mb-6 mb-md-2 pr-3 pr-md-0 pl-md-4">
        <h4 class="mb-3 text-mono text-gray-light text-normal">Support</h4>
        <ul class="list-style-none f5">
          <li class="lh-condensed mb-3"><a href="/" class="link-gray">Help</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.community" class="link-gray">Community Forum</a></li>
          <li class="lh-condensed mb-3"><a href="https://services.github.com/" class="link-gray">Training</a></li>
          <li class="lh-condensed mb-3"><a href="https://githubstatus.com/" class="link-gray">Status</a></li>
          <li class="lh-condensed mb-3"><a href="https://support.github.com/contact" class="link-gray">Contact GitHub</a></li>
        </ul>
      </div>
      <div class="col-6 col-sm-3 col-lg-2 mb-6 mb-md-2 pr-3 pr-md-0 pl-md-4">
        <h4 class="mb-3 text-mono text-gray-light text-normal">Company</h4>
        <ul class="list-style-none f5">
          <li class="lh-condensed mb-3"><a href="https://github.com/about" class="link-gray">About</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.blog/" class="link-gray">Blog</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.com/about/careers" class="link-gray">Careers</a></li>
          <li class="lh-condensed mb-3"><a href="https://github.com/about/press" class="link-gray">Press</a></li>
          <li class="lh-condensed mb-3"><a href="https://shop.github.com" class="link-gray">Shop</a></li>
        </ul>
      </div>
    </div>
  </div>
  <div class="bg-gray-light">
    <div class="container-xl px-3 px-md-6 f6 py-4 d-sm-flex flex-justify-between flex-row-reverse flex-items-center">
      <ul class="list-style-none d-flex flex-items-center mb-3 mb-sm-0 lh-condensed-ultra">
        <li class="mr-3">
          <a href="https://twitter.com/github" title="GitHub on Twitter" style="color: #959da5;">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 273.5 222.3" class="d-block" height="18"><path d="M273.5 26.3a109.77 109.77 0 0 1-32.2 8.8 56.07 56.07 0 0 0 24.7-31 113.39 113.39 0 0 1-35.7 13.6 56.1 56.1 0 0 0-97 38.4 54 54 0 0 0 1.5 12.8A159.68 159.68 0 0 1 19.1 10.3a56.12 56.12 0 0 0 17.4 74.9 56.06 56.06 0 0 1-25.4-7v.7a56.11 56.11 0 0 0 45 55 55.65 55.65 0 0 1-14.8 2 62.39 62.39 0 0 1-10.6-1 56.24 56.24 0 0 0 52.4 39 112.87 112.87 0 0 1-69.7 24 119 119 0 0 1-13.4-.8 158.83 158.83 0 0 0 86 25.2c103.2 0 159.6-85.5 159.6-159.6 0-2.4-.1-4.9-.2-7.3a114.25 114.25 0 0 0 28.1-29.1" fill="currentColor"></path></svg>
          </a>
        </li>
        <li class="mr-3">
          <a href="https://www.facebook.com/GitHub" title="GitHub on Facebook" style="color: #959da5;">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 15.3 15.4" class="d-block" height="18"><path d="M14.5 0H.8a.88.88 0 0 0-.8.9v13.6a.88.88 0 0 0 .8.9h7.3v-6h-2V7.1h2V5.4a2.87 2.87 0 0 1 2.5-3.1h.5a10.87 10.87 0 0 1 1.8.1v2.1h-1.3c-1 0-1.1.5-1.1 1.1v1.5h2.3l-.3 2.3h-2v5.9h3.9a.88.88 0 0 0 .9-.8V.8a.86.86 0 0 0-.8-.8z" fill="currentColor"></path></svg>
          </a>
        </li>
        <li class="mr-3">
          <a href="https://www.youtube.com/github" title="GitHub on YouTube" style="color: #959da5;">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 19.17 13.6" class="d-block" height="16"><path d="M18.77 2.13A2.4 2.4 0 0 0 17.09.42C15.59 0 9.58 0 9.58 0a57.55 57.55 0 0 0-7.5.4A2.49 2.49 0 0 0 .39 2.13 26.27 26.27 0 0 0 0 6.8a26.15 26.15 0 0 0 .39 4.67 2.43 2.43 0 0 0 1.69 1.71c1.52.42 7.5.42 7.5.42a57.69 57.69 0 0 0 7.51-.4 2.4 2.4 0 0 0 1.68-1.71 25.63 25.63 0 0 0 .4-4.67 24 24 0 0 0-.4-4.69zM7.67 9.71V3.89l5 2.91z" fill="currentColor"></path></svg>
          </a>
        </li>
        <li class="mr-3 flex-self-start">
          <a href="https://www.linkedin.com/company/github" title="GitHub on Linkedin" style="color: #959da5;">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 19 18" class="d-block" height="18"><path d="M3.94 2A2 2 0 1 1 2 0a2 2 0 0 1 1.94 2zM4 5.48H0V18h4zm6.32 0H6.34V18h3.94v-6.57c0-3.66 4.77-4 4.77 0V18H19v-7.93c0-6.17-7.06-5.94-8.72-2.91z" fill="currentColor"></path></svg>
          </a>
        </li>
        <li>
          <a href="https://github.com/github" title="GitHub's organization" style="color: #959da5;">
            <svg version="1.1" width="20" height="20" viewBox="0 0 16 16" class="octicon octicon-mark-github" aria-hidden="true"><path fill-rule="evenodd" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path></svg>
          </a>
        </li>
      </ul>
      <ul class="list-style-none d-flex text-gray">
        <li class="mr-3">&copy; 2020 GitHub, Inc.</li>
        <li class="mr-3"><a href="/articles/github-terms-of-service/" class="link-gray">Terms </a></li>
        <li><a href="/articles/github-privacy-statement/" class="link-gray">Privacy </a></li>
      </ul>
    </div>
  </div>
</div>

<script src="/dist/index.js"></script>

    </main>
  </body>
</html>
`

var activityEventsGoFileOriginal = `// Copyright 2013 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
)

// ListEvents drinks from the firehose of all public events across GitHub.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events
func (s *ActivityService) ListEvents(ctx context.Context, opts *ListOptions) ([]*Event, *Response, error) {
	u, err := addOptions("events", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListRepositoryEvents lists events for a repository.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-repository-events
func (s *ActivityService) ListRepositoryEvents(ctx context.Context, owner, repo string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/events", owner, repo)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// Note that ActivityService.ListIssueEventsForRepository was moved to:
// IssuesService.ListRepositoryEvents.

// ListEventsForRepoNetwork lists public events for a network of repositories.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-for-a-network-of-repositories
func (s *ActivityService) ListEventsForRepoNetwork(ctx context.Context, owner, repo string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("networks/%v/%v/events", owner, repo)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListEventsForOrganization lists public events for an organization.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-for-an-organization
func (s *ActivityService) ListEventsForOrganization(ctx context.Context, org string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("orgs/%v/events", org)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListEventsPerformedByUser lists the events performed by a user. If publicOnly is
// true, only public events will be returned.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-events-for-the-authenticated-user
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-for-a-user
func (s *ActivityService) ListEventsPerformedByUser(ctx context.Context, user string, publicOnly bool, opts *ListOptions) ([]*Event, *Response, error) {
	var u string
	if publicOnly {
		u = fmt.Sprintf("users/%v/events/public", user)
	} else {
		u = fmt.Sprintf("users/%v/events", user)
	}
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListEventsReceivedByUser lists the events received by a user. If publicOnly is
// true, only public events will be returned.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-events-received-by-the-authenticated-user
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-received-by-a-user
func (s *ActivityService) ListEventsReceivedByUser(ctx context.Context, user string, publicOnly bool, opts *ListOptions) ([]*Event, *Response, error) {
	var u string
	if publicOnly {
		u = fmt.Sprintf("users/%v/received_events/public", user)
	} else {
		u = fmt.Sprintf("users/%v/received_events", user)
	}
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListUserEventsForOrganization provides the user’s organization dashboard. You
// must be authenticated as the user to view this.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-events-for-an-organization
func (s *ActivityService) ListUserEventsForOrganization(ctx context.Context, org, user string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("users/%v/events/orgs/%v", user, org)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}
`

var activityEventsGoFileWant = `// Copyright 2013 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
)

// ListEvents drinks from the firehose of all public events across GitHub.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events
func (s *ActivityService) ListEvents(ctx context.Context, opts *ListOptions) ([]*Event, *Response, error) {
	u, err := addOptions("events", opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListRepositoryEvents lists events for a repository.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-repository-events
func (s *ActivityService) ListRepositoryEvents(ctx context.Context, owner, repo string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/events", owner, repo)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// Note that ActivityService.ListIssueEventsForRepository was moved to:
// IssuesService.ListRepositoryEvents.

// ListEventsForRepoNetwork lists public events for a network of repositories.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-for-a-network-of-repositories
func (s *ActivityService) ListEventsForRepoNetwork(ctx context.Context, owner, repo string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("networks/%v/%v/events", owner, repo)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListEventsForOrganization lists public events for an organization.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-organization-events
func (s *ActivityService) ListEventsForOrganization(ctx context.Context, org string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("orgs/%v/events", org)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListEventsPerformedByUser lists the events performed by a user. If publicOnly is
// true, only public events will be returned.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-events-for-the-authenticated-user
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-for-a-user
func (s *ActivityService) ListEventsPerformedByUser(ctx context.Context, user string, publicOnly bool, opts *ListOptions) ([]*Event, *Response, error) {
	var u string
	if publicOnly {
		u = fmt.Sprintf("users/%v/events/public", user)
	} else {
		u = fmt.Sprintf("users/%v/events", user)
	}
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListEventsReceivedByUser lists the events received by a user. If publicOnly is
// true, only public events will be returned.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-events-received-by-the-authenticated-user
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-public-events-received-by-a-user
func (s *ActivityService) ListEventsReceivedByUser(ctx context.Context, user string, publicOnly bool, opts *ListOptions) ([]*Event, *Response, error) {
	var u string
	if publicOnly {
		u = fmt.Sprintf("users/%v/received_events/public", user)
	} else {
		u = fmt.Sprintf("users/%v/received_events", user)
	}
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}

// ListUserEventsForOrganization provides the user’s organization dashboard. You
// must be authenticated as the user to view this.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/activity/events/#list-organization-events-for-the-authenticated-user
func (s *ActivityService) ListUserEventsForOrganization(ctx context.Context, org, user string, opts *ListOptions) ([]*Event, *Response, error) {
	u := fmt.Sprintf("users/%v/events/orgs/%v", user, org)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var events []*Event
	resp, err := s.client.Do(ctx, req, &events)
	if err != nil {
		return nil, resp, err
	}

	return events, resp, nil
}
`
