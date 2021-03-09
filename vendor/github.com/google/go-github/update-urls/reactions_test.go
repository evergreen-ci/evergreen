// Copyright 2020 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"testing"
)

func newReactionsPipeline() *pipelineSetup {
	return &pipelineSetup{
		baseURL:              "https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/",
		endpointsFromWebsite: reactionsWant,
		filename:             "reactions.go",
		serviceName:          "ReactionsService",
		originalGoSource:     reactionsGoFileOriginal,
		wantGoSource:         reactionsGoFileWant,
		wantNumEndpoints:     25,
	}
}

func TestPipeline_Reactions(t *testing.T) {
	ps := newReactionsPipeline()
	ps.setup(t, false, false)
	ps.validate(t)
}

func TestPipeline_Reactions_FirstStripAllURLs(t *testing.T) {
	ps := newReactionsPipeline()
	ps.setup(t, true, false)
	ps.validate(t)
}

func TestPipeline_Reactions_FirstDestroyReceivers(t *testing.T) {
	ps := newReactionsPipeline()
	ps.setup(t, false, true)
	ps.validate(t)
}

func TestPipeline_Reactions_FirstStripAllURLsAndDestroyReceivers(t *testing.T) {
	ps := newReactionsPipeline()
	ps.setup(t, true, true)
	ps.validate(t)
}

func TestParseWebPageEndpoints_Reactions(t *testing.T) {
	got, want := parseWebPageEndpoints(reactionsTestWebPage), reactionsWant
	testWebPageHelper(t, got, want)
}

var reactionsWant = endpointsByFragmentID{
	"list-reactions-for-a-commit-comment": []*Endpoint{{urlFormats: []string{"repos/%v/%v/comments/%v/reactions"}, httpMethod: "GET"}},

	"delete-a-commit-comment-reaction": []*Endpoint{
		{urlFormats: []string{"repositories/%v/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
		{urlFormats: []string{"repos/%v/%v/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
	},

	"create-reaction-for-an-issue": []*Endpoint{{urlFormats: []string{"repos/%v/%v/issues/%v/reactions"}, httpMethod: "POST"}},

	"delete-an-issue-reaction": []*Endpoint{
		{urlFormats: []string{"repositories/%v/issues/%v/reactions/%v"}, httpMethod: "DELETE"},
		{urlFormats: []string{"repos/%v/%v/issues/%v/reactions/%v"}, httpMethod: "DELETE"},
	},

	"create-reaction-for-a-pull-request-review-comment": []*Endpoint{{urlFormats: []string{"repos/%v/%v/pulls/comments/%v/reactions"}, httpMethod: "POST"}},

	"list-reactions-for-a-team-discussion": []*Endpoint{
		{urlFormats: []string{"organizations/%v/team/%v/discussions/%v/reactions"}, httpMethod: "GET"},
		{urlFormats: []string{"orgs/%v/teams/%v/discussions/%v/reactions"}, httpMethod: "GET"},
	},

	"delete-a-reaction-legacy": []*Endpoint{{urlFormats: []string{"reactions/%v"}, httpMethod: "DELETE"}},

	"list-reactions-for-a-team-discussion-comment-legacy": []*Endpoint{{urlFormats: []string{"teams/%v/discussions/%v/comments/%v/reactions"}, httpMethod: "GET"}},

	"delete-an-issue-comment-reaction": []*Endpoint{
		{urlFormats: []string{"repositories/%v/issues/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
		{urlFormats: []string{"repos/%v/%v/issues/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
	},

	"list-reactions-for-a-pull-request-review-comment": []*Endpoint{{urlFormats: []string{"repos/%v/%v/pulls/comments/%v/reactions"}, httpMethod: "GET"}},

	"create-reaction-for-a-team-discussion-legacy": []*Endpoint{{urlFormats: []string{"teams/%v/discussions/%v/reactions"}, httpMethod: "POST"}},

	"create-reaction-for-a-team-discussion-comment-legacy": []*Endpoint{{urlFormats: []string{"teams/%v/discussions/%v/comments/%v/reactions"}, httpMethod: "POST"}},

	"create-reaction-for-a-commit-comment": []*Endpoint{{urlFormats: []string{"repos/%v/%v/comments/%v/reactions"}, httpMethod: "POST"}},

	"list-reactions-for-an-issue": []*Endpoint{{urlFormats: []string{"repos/%v/%v/issues/%v/reactions"}, httpMethod: "GET"}},

	"create-reaction-for-an-issue-comment": []*Endpoint{{urlFormats: []string{"repos/%v/%v/issues/comments/%v/reactions"}, httpMethod: "POST"}},

	"create-reaction-for-a-team-discussion": []*Endpoint{
		{urlFormats: []string{"organizations/%v/team/%v/discussions/%v/reactions"}, httpMethod: "POST"},
		{urlFormats: []string{"orgs/%v/teams/%v/discussions/%v/reactions"}, httpMethod: "POST"},
	},

	"delete-team-discussion-reaction": []*Endpoint{
		{urlFormats: []string{"organizations/%v/team/%v/discussions/%v/reactions/%v"}, httpMethod: "DELETE"},
		{urlFormats: []string{"orgs/%v/teams/%v/discussions/%v/reactions/%v"}, httpMethod: "DELETE"},
	},

	"create-reaction-for-a-team-discussion-comment": []*Endpoint{
		{urlFormats: []string{"organizations/%v/team/%v/discussions/%v/comments/%v/reactions"}, httpMethod: "POST"},
		{urlFormats: []string{"orgs/%v/teams/%v/discussions/%v/comments/%v/reactions"}, httpMethod: "POST"},
	},

	"list-reactions-for-an-issue-comment": []*Endpoint{{urlFormats: []string{"repos/%v/%v/issues/comments/%v/reactions"}, httpMethod: "GET"}},

	"delete-a-pull-request-comment-reaction": []*Endpoint{
		{urlFormats: []string{"repositories/%v/pulls/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
		{urlFormats: []string{"repos/%v/%v/pulls/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
	},

	"list-reactions-for-a-team-discussion-comment": []*Endpoint{
		{urlFormats: []string{"organizations/%v/team/%v/discussions/%v/comments/%v/reactions"}, httpMethod: "GET"},
		{urlFormats: []string{"orgs/%v/teams/%v/discussions/%v/comments/%v/reactions"}, httpMethod: "GET"},
	},

	"delete-team-discussion-comment-reaction": []*Endpoint{
		{urlFormats: []string{"organizations/%v/team/%v/discussions/%v/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
		{urlFormats: []string{"orgs/%v/teams/%v/discussions/%v/comments/%v/reactions/%v"}, httpMethod: "DELETE"},
	},

	"list-reactions-for-a-team-discussion-legacy": []*Endpoint{{urlFormats: []string{"teams/%v/discussions/%v/reactions"}, httpMethod: "GET"}},
}

var reactionsTestWebPage = `
<html lang="en">
  <head>
  <title>Reactions - GitHub Docs</title>
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
      href="https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions"
    />
  
    <link
      rel="alternate"
      hreflang="zh-Hans"
      href="https://docs.github.com/cn/rest/reference/reactions"
    />
  
    <link
      rel="alternate"
      hreflang="ja"
      href="https://docs.github.com/ja/rest/reference/reactions"
    />
  
    <link
      rel="alternate"
      hreflang="es"
      href="https://docs.github.com/es/rest/reference/reactions"
    />
  
    <link
      rel="alternate"
      hreflang="pt"
      href="https://docs.github.com/pt/rest/reference/reactions"
    />
  
    <link
      rel="alternate"
      hreflang="de"
      href="https://docs.github.com/de/rest/reference/reactions"
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
        
        
        <li class="sidebar-article ">
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
        
        
        <li class="sidebar-article active is-current-page">
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
                            href="/en/free-pro-team@latest/rest/reference/reactions"
                            class="d-block py-2 no-underline active link-gray"
                            style="white-space: nowrap"
                          >
                            
                              English
                            
                          </a>
                        
                      
                        
                          <a
                            href="/cn/rest/reference/reactions"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              简体中文 (Simplified Chinese)
                            
                          </a>
                        
                      
                        
                          <a
                            href="/ja/rest/reference/reactions"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              日本語 (Japanese)
                            
                          </a>
                        
                      
                        
                          <a
                            href="/es/rest/reference/reactions"
                            class="d-block py-2 no-underline link-gray-dark"
                            style="white-space: nowrap"
                          >
                            
                              Español (Spanish)
                            
                          </a>
                        
                      
                        
                          <a
                            href="/pt/rest/reference/reactions"
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
      href="/en/free-pro-team@latest/rest/reference/reactions"
      class="d-block py-2 link-blue active"
      >GitHub.com</a>
      
      <a
      href="/en/enterprise/2.21/user/rest/reference/reactions"
      class="d-block py-2 link-gray-dark no-underline"
      >Enterprise Server 2.21</a>
      
      <a
      href="/en/enterprise/2.20/user/rest/reference/reactions"
      class="d-block py-2 link-gray-dark no-underline"
      >Enterprise Server 2.20</a>
      
      <a
      href="/en/enterprise/2.19/user/rest/reference/reactions"
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
  
  <a title="article: Reactions" href="/en/free-pro-team@latest/rest/reference/reactions" class="d-inline-block text-gray-light">
    Reactions</a>
  
</nav>

      </div>
    </div>

    <div class="mt-2 article-grid-container">

    <div class="article-grid-head">
      <div class="d-flex flex-items-baseline flex-justify-between mt-3">
        <h1 class="border-bottom-0">Reactions</h1>
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
          
          <li class="ml-0  mb-2 lh-condensed"><a href="#reaction-types">Reaction types</a></li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-a-team-discussion-comment">List reactions for a team discussion comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-a-team-discussion-comment">Create reaction for a team discussion comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-team-discussion-comment-reaction">Delete team discussion comment reaction</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-a-team-discussion">List reactions for a team discussion</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-a-team-discussion">Create reaction for a team discussion</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-team-discussion-reaction">Delete team discussion reaction</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-a-reaction-legacy">Delete a reaction (Legacy)</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-a-commit-comment">List reactions for a commit comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-a-commit-comment">Create reaction for a commit comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-a-commit-comment-reaction">Delete a commit comment reaction</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-an-issue-comment">List reactions for an issue comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-an-issue-comment">Create reaction for an issue comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-an-issue-comment-reaction">Delete an issue comment reaction</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-an-issue">List reactions for an issue</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-an-issue">Create reaction for an issue</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-an-issue-reaction">Delete an issue reaction</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-a-pull-request-review-comment">List reactions for a pull request review comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-a-pull-request-review-comment">Create reaction for a pull request review comment</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#delete-a-pull-request-comment-reaction">Delete a pull request comment reaction</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-a-team-discussion-comment-legacy">List reactions for a team discussion comment (Legacy)</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-a-team-discussion-comment-legacy">Create reaction for a team discussion comment (Legacy)</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#list-reactions-for-a-team-discussion-legacy">List reactions for a team discussion (Legacy)</a>
    </li>
          
          <li class="ml-0  mb-2 lh-condensed">
      <a href="#create-reaction-for-a-team-discussion-legacy">Create reaction for a team discussion (Legacy)</a>
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
      
      <h3 id="reaction-types"><a href="#reaction-types">Reaction types</a></h3>
<p>When creating a reaction, the allowed values for the <code>content</code> parameter are as follows (with the corresponding emoji for reference):</p>









































<table><thead><tr><th>content</th><th>emoji</th></tr></thead><tbody><tr><td><code>+1</code></td><td>&#x1F44D;</td></tr><tr><td><code>-1</code></td><td>&#x1F44E;</td></tr><tr><td><code>laugh</code></td><td>&#x1F604;</td></tr><tr><td><code>confused</code></td><td>&#x1F615;</td></tr><tr><td><code>heart</code></td><td>&#x2764;&#xFE0F;</td></tr><tr><td><code>hooray</code></td><td>&#x1F389;</td></tr><tr><td><code>rocket</code></td><td>&#x1F680;</td></tr><tr><td><code>eyes</code></td><td>&#x1F440;</td></tr></tbody></table>
  <div>
  <div>
    <h3 id="list-reactions-for-a-team-discussion-comment" class="pt-3">
      <a href="#list-reactions-for-a-team-discussion-comment">List reactions for a team discussion comment</a>
    </h3>
    <p>List the reactions to a <a href="https://developer.github.com/v3/teams/discussion_comments/">team discussion comment</a>. OAuth access tokens require the <code>read:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>.</p>
<p><strong>Note:</strong> You can also specify a team by <code>org_id</code> and <code>team_id</code> using the route <code>GET /organizations/:org_id/team/:team_id/discussions/:discussion_number/comments/:comment_number/reactions</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/comments/{comment_number}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-a-team-discussion-comment--parameters">
        <a href="#list-reactions-for-a-team-discussion-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-a-team-discussion-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>team_slug</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>comment_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to a team discussion comment.</p>
                
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
    
    
      <h4 id="list-reactions-for-a-team-discussion-comment--code-samples">
        <a href="#list-reactions-for-a-team-discussion-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/orgs/ORG/teams/TEAM_SLUG/discussions/42/comments/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/comments/{comment_number}/reactions&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>,
  <span class="hljs-attr">team_slug</span>: <span class="hljs-string">&apos;team_slug&apos;</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">comment_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-a-team-discussion-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-a-team-discussion-comment" class="pt-3">
      <a href="#create-reaction-for-a-team-discussion-comment">Create reaction for a team discussion comment</a>
    </h3>
    <p>Create a reaction to a <a href="https://developer.github.com/v3/teams/discussion_comments/">team discussion comment</a>. OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this team discussion comment.</p>
<p><strong>Note:</strong> You can also specify a team by <code>org_id</code> and <code>team_id</code> using the route <code>POST /organizations/:org_id/team/:team_id/discussions/:discussion_number/comments/:comment_number/reactions</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/comments/{comment_number}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-a-team-discussion-comment--parameters">
        <a href="#create-reaction-for-a-team-discussion-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-a-team-discussion-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>team_slug</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>comment_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the team discussion comment.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-a-team-discussion-comment--code-samples">
        <a href="#create-reaction-for-a-team-discussion-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/orgs/ORG/teams/TEAM_SLUG/discussions/42/comments/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/comments/{comment_number}/reactions&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>,
  <span class="hljs-attr">team_slug</span>: <span class="hljs-string">&apos;team_slug&apos;</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">comment_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="create-reaction-for-a-team-discussion-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-team-discussion-comment-reaction" class="pt-3">
      <a href="#delete-team-discussion-comment-reaction">Delete team discussion comment reaction</a>
    </h3>
    <p><strong>Note:</strong> You can also specify a team or organization with <code>team_id</code> and <code>org_id</code> using the route <code>DELETE /organizations/:org_id/team/:team_id/discussions/:discussion_number/comments/:comment_number/reactions/:reaction_id</code>.</p>
<p>Delete a reaction to a <a href="https://developer.github.com/v3/teams/discussion_comments/">team discussion comment</a>. OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/comments/{comment_number}/reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-team-discussion-comment-reaction--parameters">
        <a href="#delete-team-discussion-comment-reaction--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-team-discussion-comment-reaction-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>team_slug</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>comment_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-team-discussion-comment-reaction--code-samples">
        <a href="#delete-team-discussion-comment-reaction--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/orgs/ORG/teams/TEAM_SLUG/discussions/42/comments/42/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/comments/{comment_number}/reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>,
  <span class="hljs-attr">team_slug</span>: <span class="hljs-string">&apos;team_slug&apos;</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">comment_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-team-discussion-comment-reaction-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-a-team-discussion" class="pt-3">
      <a href="#list-reactions-for-a-team-discussion">List reactions for a team discussion</a>
    </h3>
    <p>List the reactions to a <a href="https://developer.github.com/v3/teams/discussions/">team discussion</a>. OAuth access tokens require the <code>read:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>.</p>
<p><strong>Note:</strong> You can also specify a team by <code>org_id</code> and <code>team_id</code> using the route <code>GET /organizations/:org_id/team/:team_id/discussions/:discussion_number/reactions</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-a-team-discussion--parameters">
        <a href="#list-reactions-for-a-team-discussion--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-a-team-discussion-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>team_slug</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to a team discussion.</p>
                
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
    
    
      <h4 id="list-reactions-for-a-team-discussion--code-samples">
        <a href="#list-reactions-for-a-team-discussion--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/orgs/ORG/teams/TEAM_SLUG/discussions/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/reactions&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>,
  <span class="hljs-attr">team_slug</span>: <span class="hljs-string">&apos;team_slug&apos;</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-a-team-discussion-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-a-team-discussion" class="pt-3">
      <a href="#create-reaction-for-a-team-discussion">Create reaction for a team discussion</a>
    </h3>
    <p>Create a reaction to a <a href="https://developer.github.com/v3/teams/discussions/">team discussion</a>. OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this team discussion.</p>
<p><strong>Note:</strong> You can also specify a team by <code>org_id</code> and <code>team_id</code> using the route <code>POST /organizations/:org_id/team/:team_id/discussions/:discussion_number/reactions</code>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-a-team-discussion--parameters">
        <a href="#create-reaction-for-a-team-discussion--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-a-team-discussion-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>team_slug</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the team discussion.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-a-team-discussion--code-samples">
        <a href="#create-reaction-for-a-team-discussion--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/orgs/ORG/teams/TEAM_SLUG/discussions/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/reactions&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>,
  <span class="hljs-attr">team_slug</span>: <span class="hljs-string">&apos;team_slug&apos;</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
    
      <h4 id="create-reaction-for-a-team-discussion-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-team-discussion-reaction" class="pt-3">
      <a href="#delete-team-discussion-reaction">Delete team discussion reaction</a>
    </h3>
    <p><strong>Note:</strong> You can also specify a team or organization with <code>team_id</code> and <code>org_id</code> using the route <code>DELETE /organizations/:org_id/team/:team_id/discussions/:discussion_number/reactions/:reaction_id</code>.</p>
<p>Delete a reaction to a <a href="https://developer.github.com/v3/teams/discussions/">team discussion</a>. OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-team-discussion-reaction--parameters">
        <a href="#delete-team-discussion-reaction--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-team-discussion-reaction-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>team_slug</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-team-discussion-reaction--code-samples">
        <a href="#delete-team-discussion-reaction--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/orgs/ORG/teams/TEAM_SLUG/discussions/42/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /orgs/{org}/teams/{team_slug}/discussions/{discussion_number}/reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">org</span>: <span class="hljs-string">&apos;org&apos;</span>,
  <span class="hljs-attr">team_slug</span>: <span class="hljs-string">&apos;team_slug&apos;</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-team-discussion-reaction-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-a-reaction-legacy" class="pt-3">
      <a href="#delete-a-reaction-legacy">Delete a reaction (Legacy)</a>
    </h3>
    <p><strong>Deprecation Notice:</strong> This endpoint route is deprecated and will be removed from the Reactions API. We recommend migrating your existing code to use the new delete reactions endpoints. For more information, see this <a href="https://developer.github.com/changes/2020-02-26-new-delete-reactions-endpoints/">blog post</a>.</p>
<p>OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>, when deleting a <a href="https://developer.github.com/v3/teams/discussions/">team discussion</a> or <a href="https://developer.github.com/v3/teams/discussion_comments/">team discussion comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-a-reaction-legacy--parameters">
        <a href="#delete-a-reaction-legacy--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-a-reaction-legacy-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-a-reaction-legacy--code-samples">
        <a href="#delete-a-reaction-legacy--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-a-reaction-legacy-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-a-commit-comment" class="pt-3">
      <a href="#list-reactions-for-a-commit-comment">List reactions for a commit comment</a>
    </h3>
    <p>List the reactions to a <a href="https://developer.github.com/v3/repos/comments/">commit comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/comments/{comment_id}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-a-commit-comment--parameters">
        <a href="#list-reactions-for-a-commit-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-a-commit-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to a commit comment.</p>
                
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
    
    
      <h4 id="list-reactions-for-a-commit-comment--code-samples">
        <a href="#list-reactions-for-a-commit-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/comments/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/comments/{comment_id}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-a-commit-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-a-commit-comment" class="pt-3">
      <a href="#create-reaction-for-a-commit-comment">Create reaction for a commit comment</a>
    </h3>
    <p>Create a reaction to a <a href="https://developer.github.com/v3/repos/comments/">commit comment</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this commit comment.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /repos/{owner}/{repo}/comments/{comment_id}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-a-commit-comment--parameters">
        <a href="#create-reaction-for-a-commit-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-a-commit-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the commit comment.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-a-commit-comment--code-samples">
        <a href="#create-reaction-for-a-commit-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/comments/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /repos/{owner}/{repo}/comments/{comment_id}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="create-reaction-for-a-commit-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-a-commit-comment-reaction" class="pt-3">
      <a href="#delete-a-commit-comment-reaction">Delete a commit comment reaction</a>
    </h3>
    <p><strong>Note:</strong> You can also specify a repository by <code>repository_id</code> using the route <code>DELETE /repositories/:repository_id/comments/:comment_id/reactions/:reaction_id</code>.</p>
<p>Delete a reaction to a <a href="https://developer.github.com/v3/repos/comments/">commit comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /repos/{owner}/{repo}/comments/{comment_id}/reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-a-commit-comment-reaction--parameters">
        <a href="#delete-a-commit-comment-reaction--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-a-commit-comment-reaction-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-a-commit-comment-reaction--code-samples">
        <a href="#delete-a-commit-comment-reaction--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/comments/42/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /repos/{owner}/{repo}/comments/{comment_id}/reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-a-commit-comment-reaction-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-an-issue-comment" class="pt-3">
      <a href="#list-reactions-for-an-issue-comment">List reactions for an issue comment</a>
    </h3>
    <p>List the reactions to an <a href="https://developer.github.com/v3/issues/comments/">issue comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-an-issue-comment--parameters">
        <a href="#list-reactions-for-an-issue-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-an-issue-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to an issue comment.</p>
                
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
    
    
      <h4 id="list-reactions-for-an-issue-comment--code-samples">
        <a href="#list-reactions-for-an-issue-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/issues/comments/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-an-issue-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-an-issue-comment" class="pt-3">
      <a href="#create-reaction-for-an-issue-comment">Create reaction for an issue comment</a>
    </h3>
    <p>Create a reaction to an <a href="https://developer.github.com/v3/issues/comments/">issue comment</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this issue comment.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-an-issue-comment--parameters">
        <a href="#create-reaction-for-an-issue-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-an-issue-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the issue comment.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-an-issue-comment--code-samples">
        <a href="#create-reaction-for-an-issue-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/issues/comments/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="create-reaction-for-an-issue-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-an-issue-comment-reaction" class="pt-3">
      <a href="#delete-an-issue-comment-reaction">Delete an issue comment reaction</a>
    </h3>
    <p><strong>Note:</strong> You can also specify a repository by <code>repository_id</code> using the route <code>DELETE delete /repositories/:repository_id/issues/comments/:comment_id/reactions/:reaction_id</code>.</p>
<p>Delete a reaction to an <a href="https://developer.github.com/v3/issues/comments/">issue comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-an-issue-comment-reaction--parameters">
        <a href="#delete-an-issue-comment-reaction--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-an-issue-comment-reaction-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-an-issue-comment-reaction--code-samples">
        <a href="#delete-an-issue-comment-reaction--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/issues/comments/42/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-an-issue-comment-reaction-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-an-issue" class="pt-3">
      <a href="#list-reactions-for-an-issue">List reactions for an issue</a>
    </h3>
    <p>List the reactions to an <a href="https://developer.github.com/v3/issues/">issue</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/issues/{issue_number}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-an-issue--parameters">
        <a href="#list-reactions-for-an-issue--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-an-issue-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>issue_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to an issue.</p>
                
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
    
    
      <h4 id="list-reactions-for-an-issue--code-samples">
        <a href="#list-reactions-for-an-issue--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/issues/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/issues/{issue_number}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">issue_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-an-issue-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-an-issue" class="pt-3">
      <a href="#create-reaction-for-an-issue">Create reaction for an issue</a>
    </h3>
    <p>Create a reaction to an <a href="https://developer.github.com/v3/issues/">issue</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this issue.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /repos/{owner}/{repo}/issues/{issue_number}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-an-issue--parameters">
        <a href="#create-reaction-for-an-issue--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-an-issue-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>issue_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the issue.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-an-issue--code-samples">
        <a href="#create-reaction-for-an-issue--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/issues/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /repos/{owner}/{repo}/issues/{issue_number}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">issue_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
    
      <h4 id="create-reaction-for-an-issue-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-an-issue-reaction" class="pt-3">
      <a href="#delete-an-issue-reaction">Delete an issue reaction</a>
    </h3>
    <p><strong>Note:</strong> You can also specify a repository by <code>repository_id</code> using the route <code>DELETE /repositories/:repository_id/issues/:issue_number/reactions/:reaction_id</code>.</p>
<p>Delete a reaction to an <a href="https://developer.github.com/v3/issues/">issue</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /repos/{owner}/{repo}/issues/{issue_number}/reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-an-issue-reaction--parameters">
        <a href="#delete-an-issue-reaction--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-an-issue-reaction-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>issue_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-an-issue-reaction--code-samples">
        <a href="#delete-an-issue-reaction--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/issues/42/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /repos/{owner}/{repo}/issues/{issue_number}/reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">issue_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-an-issue-reaction-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-a-pull-request-review-comment" class="pt-3">
      <a href="#list-reactions-for-a-pull-request-review-comment">List reactions for a pull request review comment</a>
    </h3>
    <p>List the reactions to a <a href="https://developer.github.com/v3/pulls/comments/">pull request review comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-a-pull-request-review-comment--parameters">
        <a href="#list-reactions-for-a-pull-request-review-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-a-pull-request-review-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to a pull request review comment.</p>
                
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
    
    
      <h4 id="list-reactions-for-a-pull-request-review-comment--code-samples">
        <a href="#list-reactions-for-a-pull-request-review-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/pulls/comments/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-a-pull-request-review-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-a-pull-request-review-comment" class="pt-3">
      <a href="#create-reaction-for-a-pull-request-review-comment">Create reaction for a pull request review comment</a>
    </h3>
    <p>Create a reaction to a <a href="https://developer.github.com/v3/pulls/comments/">pull request review comment</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this pull request review comment.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-a-pull-request-review-comment--parameters">
        <a href="#create-reaction-for-a-pull-request-review-comment--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-a-pull-request-review-comment-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the pull request review comment.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-a-pull-request-review-comment--code-samples">
        <a href="#create-reaction-for-a-pull-request-review-comment--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/pulls/comments/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="create-reaction-for-a-pull-request-review-comment-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="delete-a-pull-request-comment-reaction" class="pt-3">
      <a href="#delete-a-pull-request-comment-reaction">Delete a pull request comment reaction</a>
    </h3>
    <p><strong>Note:</strong> You can also specify a repository by <code>repository_id</code> using the route <code>DELETE /repositories/:repository_id/pulls/comments/:comment_id/reactions/:reaction_id.</code></p>
<p>Delete a reaction to a <a href="https://developer.github.com/v3/pulls/comments/">pull request review comment</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">delete</span> /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions/{reaction_id}</code></pre>
  <div>
    
      <h4 id="delete-a-pull-request-comment-reaction--parameters">
        <a href="#delete-a-pull-request-comment-reaction--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#delete-a-pull-request-comment-reaction-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
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
              <td><code>comment_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>reaction_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="delete-a-pull-request-comment-reaction--code-samples">
        <a href="#delete-a-pull-request-comment-reaction--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X DELETE \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/repos/octocat/hello-world/pulls/comments/42/reactions/42
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;DELETE /repos/{owner}/{repo}/pulls/comments/{comment_id}/reactions/{reaction_id}&apos;</span>, {
  <span class="hljs-attr">owner</span>: <span class="hljs-string">&apos;octocat&apos;</span>,
  <span class="hljs-attr">repo</span>: <span class="hljs-string">&apos;hello-world&apos;</span>,
  <span class="hljs-attr">comment_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">reaction_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default Response</h4>
      <pre><code>Status: 204 No Content</code></pre>
      <div class="height-constrained-code-block"></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="delete-a-pull-request-comment-reaction-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-a-team-discussion-comment-legacy" class="pt-3">
      <a href="#list-reactions-for-a-team-discussion-comment-legacy">List reactions for a team discussion comment (Legacy)</a>
    </h3>
    <p><strong>Deprecation Notice:</strong> This endpoint route is deprecated and will be removed from the Teams API. We recommend migrating your existing code to use the new <a href="https://developer.github.com/v3/reactions/#list-reactions-for-a-team-discussion-comment"><code>List reactions for a team discussion comment</code></a> endpoint.</p>
<p>List the reactions to a <a href="https://developer.github.com/v3/teams/discussion_comments/">team discussion comment</a>. OAuth access tokens require the <code>read:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /teams/{team_id}/discussions/{discussion_number}/comments/{comment_number}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-a-team-discussion-comment-legacy--parameters">
        <a href="#list-reactions-for-a-team-discussion-comment-legacy--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-a-team-discussion-comment-legacy-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
              </td>
            </tr>
          
            <tr>
              <td><code>team_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>comment_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to a team discussion comment.</p>
                
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
    
    
      <h4 id="list-reactions-for-a-team-discussion-comment-legacy--code-samples">
        <a href="#list-reactions-for-a-team-discussion-comment-legacy--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/teams/42/discussions/42/comments/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /teams/{team_id}/discussions/{discussion_number}/comments/{comment_number}/reactions&apos;</span>, {
  <span class="hljs-attr">team_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">comment_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-a-team-discussion-comment-legacy-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-a-team-discussion-comment-legacy" class="pt-3">
      <a href="#create-reaction-for-a-team-discussion-comment-legacy">Create reaction for a team discussion comment (Legacy)</a>
    </h3>
    <p><strong>Deprecation Notice:</strong> This endpoint route is deprecated and will be removed from the Teams API. We recommend migrating your existing code to use the new <a href="https://developer.github.com/v3/reactions/#create-reaction-for-a-team-discussion-comment"><code>Create reaction for a team discussion comment</code></a> endpoint.</p>
<p>Create a reaction to a <a href="https://developer.github.com/v3/teams/discussion_comments/">team discussion comment</a>. OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this team discussion comment.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /teams/{team_id}/discussions/{discussion_number}/comments/{comment_number}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-a-team-discussion-comment-legacy--parameters">
        <a href="#create-reaction-for-a-team-discussion-comment-legacy--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-a-team-discussion-comment-legacy-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
              </td>
            </tr>
          
            <tr>
              <td><code>team_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>comment_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the team discussion comment.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-a-team-discussion-comment-legacy--code-samples">
        <a href="#create-reaction-for-a-team-discussion-comment-legacy--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/teams/42/discussions/42/comments/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /teams/{team_id}/discussions/{discussion_number}/comments/{comment_number}/reactions&apos;</span>, {
  <span class="hljs-attr">team_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">comment_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="create-reaction-for-a-team-discussion-comment-legacy-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="list-reactions-for-a-team-discussion-legacy" class="pt-3">
      <a href="#list-reactions-for-a-team-discussion-legacy">List reactions for a team discussion (Legacy)</a>
    </h3>
    <p><strong>Deprecation Notice:</strong> This endpoint route is deprecated and will be removed from the Teams API. We recommend migrating your existing code to use the new <a href="https://developer.github.com/v3/reactions/#list-reactions-for-a-team-discussion"><code>List reactions for a team discussion</code></a> endpoint.</p>
<p>List the reactions to a <a href="https://developer.github.com/v3/teams/discussions/">team discussion</a>. OAuth access tokens require the <code>read:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">get</span> /teams/{team_id}/discussions/{discussion_number}/reactions</code></pre>
  <div>
    
      <h4 id="list-reactions-for-a-team-discussion-legacy--parameters">
        <a href="#list-reactions-for-a-team-discussion-legacy--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#list-reactions-for-a-team-discussion-legacy-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
              </td>
            </tr>
          
            <tr>
              <td><code>team_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>content</code></td>
              <td class="opacity-70">string</td>
              <td class="opacity-70">query</td>
              <td class="opacity-70">
                <p>Returns a single <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a>. Omit this parameter to list all reactions to a team discussion.</p>
                
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
    
    
      <h4 id="list-reactions-for-a-team-discussion-legacy--code-samples">
        <a href="#list-reactions-for-a-team-discussion-legacy--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/teams/42/discussions/42/reactions
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;GET /teams/{team_id}/discussions/{discussion_number}/reactions&apos;</span>, {
  <span class="hljs-attr">team_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 200 OK</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">[
  {
    <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
    <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
    <span class="hljs-attr">&quot;user&quot;</span>: {
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
    <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
    <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
  }
]
</code></pre></div>
    
    
      <h4>Notes</h4>
      <ul class="mt-2">
      
        <li><a href="/en/developers/apps">Works with GitHub Apps</a></li>
      
      </ul>
    
    
      <h4 id="list-reactions-for-a-team-discussion-legacy-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
  </div>
  <hr>
</div>
  <div>
  <div>
    <h3 id="create-reaction-for-a-team-discussion-legacy" class="pt-3">
      <a href="#create-reaction-for-a-team-discussion-legacy">Create reaction for a team discussion (Legacy)</a>
    </h3>
    <p><strong>Deprecation Notice:</strong> This endpoint route is deprecated and will be removed from the Teams API. We recommend migrating your existing code to use the new <a href="https://developer.github.com/v3/reactions/#create-reaction-for-a-team-discussion"><code>Create reaction for a team discussion</code></a> endpoint.</p>
<p>Create a reaction to a <a href="https://developer.github.com/v3/teams/discussions/">team discussion</a>. OAuth access tokens require the <code>write:discussion</code> <a href="https://developer.github.com/apps/building-oauth-apps/understanding-scopes-for-oauth-apps/">scope</a>. A response with a <code>Status: 200 OK</code> means that you already added the reaction type to this team discussion.</p>
  </div>
  <pre><code><span class="bg-blue text-white rounded-1 px-2 py-1" style="text-transform: uppercase">post</span> /teams/{team_id}/discussions/{discussion_number}/reactions</code></pre>
  <div>
    
      <h4 id="create-reaction-for-a-team-discussion-legacy--parameters">
        <a href="#create-reaction-for-a-team-discussion-legacy--parameters">Parameters</a>
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
                <p>This API is under preview and subject to change.</p>
                
                  <a href="#create-reaction-for-a-team-discussion-legacy-preview-notices">
                    
                      See preview notice.
                    
                  </a>                
              </td>
            </tr>
          
            <tr>
              <td><code>team_id</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
            <tr>
              <td><code>discussion_number</code></td>
              <td class="opacity-70">integer</td>
              <td class="opacity-70">path</td>
              <td class="opacity-70">
                
                
              </td>
            </tr>
          
          
          <tr>
            <td><code>content</code></td>
            <td class="opacity-70">string</td>
            <td class="opacity-70">body</td>
            <td class="opacity-70">
              <p><strong>Required</strong>. The <a href="https://developer.github.com/v3/reactions/#reaction-types">reaction type</a> to add to the team discussion.</p>
              
            </td>
          </tr>
          
          
        </tbody>
      </table>
    
    
      <h4 id="create-reaction-for-a-team-discussion-legacy--code-samples">
        <a href="#create-reaction-for-a-team-discussion-legacy--code-samples">Code samples</a>
      </h4>
      
        
          <h5>
            
              Shell
            
          </h5>
          <pre><code class="hljs language-shell">curl \
  -X POST \
  -H &quot;Accept: application/vnd.github.squirrel-girl-preview+json&quot; \
  https://api.github.com/teams/42/discussions/42/reactions \
  -d &apos;{&quot;content&quot;:&quot;content&quot;}&apos;
</code></pre>
        
      
        
          <h5>
            
              JavaScript (<a href="https://github.com/octokit/core.js#readme">@octokit/core.js</a>)
            
          </h5>
          <pre><code class="hljs language-javascript"><span class="hljs-keyword">await</span> octokit.request(<span class="hljs-string">&apos;POST /teams/{team_id}/discussions/{discussion_number}/reactions&apos;</span>, {
  <span class="hljs-attr">team_id</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">discussion_number</span>: <span class="hljs-number">42</span>,
  <span class="hljs-attr">content</span>: <span class="hljs-string">&apos;content&apos;</span>,
  <span class="hljs-attr">mediaType</span>: {
    <span class="hljs-attr">previews</span>: [
      <span class="hljs-string">&apos;squirrel-girl&apos;</span>
    ]
  }
})
</code></pre>
        
      
    
    
      <h4>Default response</h4>
      <pre><code>Status: 201 Created</code></pre>
      <div class="height-constrained-code-block"><pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;id&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;node_id&quot;</span>: <span class="hljs-string">&quot;MDg6UmVhY3Rpb24x&quot;</span>,
  <span class="hljs-attr">&quot;user&quot;</span>: {
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
  <span class="hljs-attr">&quot;content&quot;</span>: <span class="hljs-string">&quot;heart&quot;</span>,
  <span class="hljs-attr">&quot;created_at&quot;</span>: <span class="hljs-string">&quot;2016-05-20T20:09:31Z&quot;</span>
}
</code></pre></div>
    
    
    
      <h4 id="create-reaction-for-a-team-discussion-legacy-preview-notices">
        
          Preview notice
        
      </h4>
      
        <div class="extended-markdown note border rounded-1 mb-4 p-3 border-blue bg-blue-light f5">
          <p>An additional <code>reactions</code> object in the issue comment payload is currently available for developers to preview. During
the preview period, the APIs may change without advance notice. Please see the <a href="https://developer.github.com/changes/2016-05-12-reactions-api-preview">blog
post</a> for full details.</p>
<p>To access the API you must provide a custom <a href="https://developer.github.com/v3/media">media type</a> in the <code>Accept</code> header:</p>
<pre><code class="hljs language-plaintext">application/vnd.github.squirrel-girl-preview
</code></pre>
<p>The <code>reactions</code> key will have the following payload where <code>url</code> can be used to construct the API location for <a href="https://developer.github.com/v3/reactions">listing
and creating</a> reactions.</p>
<pre><code class="hljs language-json">{
  <span class="hljs-attr">&quot;total_count&quot;</span>: <span class="hljs-number">5</span>,
  <span class="hljs-attr">&quot;+1&quot;</span>: <span class="hljs-number">3</span>,
  <span class="hljs-attr">&quot;-1&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;laugh&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;confused&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;heart&quot;</span>: <span class="hljs-number">1</span>,
  <span class="hljs-attr">&quot;hooray&quot;</span>: <span class="hljs-number">0</span>,
  <span class="hljs-attr">&quot;url&quot;</span>: <span class="hljs-string">&quot;https://api.github.com/repos/octocat/Hello-World/issues/1347/reactions&quot;</span>
}
</code></pre>
          &#x261D;&#xFE0F; This header is <strong>required</strong>.
        </div>
      
    
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

var reactionsGoFileOriginal = `// Copyright 2016 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
	"net/http"
)

// ReactionsService provides access to the reactions-related functions in the
// GitHub API.
type ReactionsService service

// Reaction represents a GitHub reaction.
type Reaction struct {
	// ID is the Reaction ID.
	ID     *int64  ` + "`" + `json:"id,omitempty"` + "`" + `
	User   *User   ` + "`" + `json:"user,omitempty"` + "`" + `
	NodeID *string ` + "`" + `json:"node_id,omitempty"` + "`" + `
	// Content is the type of reaction.
	// Possible values are:
	//     "+1", "-1", "laugh", "confused", "heart", "hooray".
	Content *string ` + "`" + `json:"content,omitempty"` + "`" + `
}

// Reactions represents a summary of GitHub reactions.
type Reactions struct {
	TotalCount *int    ` + "`" + `json:"total_count,omitempty"` + "`" + `
	PlusOne    *int    ` + "`" + `json:"+1,omitempty"` + "`" + `
	MinusOne   *int    ` + "`" + `json:"-1,omitempty"` + "`" + `
	Laugh      *int    ` + "`" + `json:"laugh,omitempty"` + "`" + `
	Confused   *int    ` + "`" + `json:"confused,omitempty"` + "`" + `
	Heart      *int    ` + "`" + `json:"heart,omitempty"` + "`" + `
	Hooray     *int    ` + "`" + `json:"hooray,omitempty"` + "`" + `
	URL        *string ` + "`" + `json:"url,omitempty"` + "`" + `
}

func (r Reaction) String() string {
	return Stringify(r)
}

// ListCommentReactionOptions specifies the optional parameters to the
// ReactionsService.ListCommentReactions method.
type ListCommentReactionOptions struct {
	// Content restricts the returned comment reactions to only those with the given type.
	// Omit this parameter to list all reactions to a commit comment.
	// Possible values are: "+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", or "eyes".
	Content string ` + "`" + `url:"content,omitempty"` + "`" + `

	ListOptions
}

// ListCommentReactions lists the reactions for a commit comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-commit-comment
func (s *ReactionsService) ListCommentReactions(ctx context.Context, owner, repo string, id int64, opts *ListCommentReactionOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/comments/%v/reactions", owner, repo, id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateCommentReaction creates a reaction for a commit comment.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-commit-comment
func (s *ReactionsService) CreateCommentReaction(ctx context.Context, owner, repo string, id int64, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/comments/%v/reactions", owner, repo, id)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteCommentReaction deletes the reaction for a commit comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-commit-comment-reaction
func (s *ReactionsService) DeleteCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) (*Response, error) {
	u := fmt.Sprintf("repos/%v/%v/comments/%v/reactions/%v", owner, repo, commentID, reactionID)

	return s.deleteReaction(ctx, u)
}

// DeleteCommentReactionByID deletes the reaction for a commit comment by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-commit-comment-reaction
func (s *ReactionsService) DeleteCommentReactionByID(ctx context.Context, repoID, commentID, reactionID int64) (*Response, error) {
	u := fmt.Sprintf("repositories/%v/comments/%v/reactions/%v", repoID, commentID, reactionID)

	return s.deleteReaction(ctx, u)
}

// ListIssueReactions lists the reactions for an issue.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-an-issue
func (s *ReactionsService) ListIssueReactions(ctx context.Context, owner, repo string, number int, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/%v/reactions", owner, repo, number)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateIssueReaction creates a reaction for an issue.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-an-issue
func (s *ReactionsService) CreateIssueReaction(ctx context.Context, owner, repo string, number int, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/%v/reactions", owner, repo, number)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteIssueReaction deletes the reaction to an issue.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-reaction
func (s *ReactionsService) DeleteIssueReaction(ctx context.Context, owner, repo string, issueNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repos/%v/%v/issues/%v/reactions/%v", owner, repo, issueNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteIssueReactionByID deletes the reaction to an issue by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-reaction
func (s *ReactionsService) DeleteIssueReactionByID(ctx context.Context, repoID, issueNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repositories/%v/issues/%v/reactions/%v", repoID, issueNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListIssueCommentReactions lists the reactions for an issue comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-an-issue-comment
func (s *ReactionsService) ListIssueCommentReactions(ctx context.Context, owner, repo string, id int64, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/comments/%v/reactions", owner, repo, id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateIssueCommentReaction creates a reaction for an issue comment.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-an-issue-comment
func (s *ReactionsService) CreateIssueCommentReaction(ctx context.Context, owner, repo string, id int64, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/comments/%v/reactions", owner, repo, id)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteIssueCommentReaction deletes the reaction to an issue comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-comment-reaction
func (s *ReactionsService) DeleteIssueCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repos/%v/%v/issues/comments/%v/reactions/%v", owner, repo, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteIssueCommentReactionByID deletes the reaction to an issue comment by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-comment-reaction
func (s *ReactionsService) DeleteIssueCommentReactionByID(ctx context.Context, repoID, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repositories/%v/issues/comments/%v/reactions/%v", repoID, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListPullRequestCommentReactions lists the reactions for a pull request review comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-an-issue-comment
func (s *ReactionsService) ListPullRequestCommentReactions(ctx context.Context, owner, repo string, id int64, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/pulls/comments/%v/reactions", owner, repo, id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreatePullRequestCommentReaction creates a reaction for a pull request review comment.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-an-issue-comment
func (s *ReactionsService) CreatePullRequestCommentReaction(ctx context.Context, owner, repo string, id int64, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/pulls/comments/%v/reactions", owner, repo, id)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeletePullRequestCommentReaction deletes the reaction to a pull request review comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-pull-request-comment-reaction
func (s *ReactionsService) DeletePullRequestCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repos/%v/%v/pulls/comments/%v/reactions/%v", owner, repo, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeletePullRequestCommentReactionByID deletes the reaction to a pull request review comment by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-pull-request-comment-reaction
func (s *ReactionsService) DeletePullRequestCommentReactionByID(ctx context.Context, repoID, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repositories/%v/pulls/comments/%v/reactions/%v", repoID, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListTeamDiscussionReactions lists the reactions for a team discussion.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-team-discussion
func (s *ReactionsService) ListTeamDiscussionReactions(ctx context.Context, teamID int64, discussionNumber int, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/reactions", teamID, discussionNumber)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateTeamDiscussionReaction creates a reaction for a team discussion.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-team-discussion
func (s *ReactionsService) CreateTeamDiscussionReaction(ctx context.Context, teamID int64, discussionNumber int, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/reactions", teamID, discussionNumber)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteTeamDiscussionReaction deletes the reaction to a team discussion.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-reaction
func (s *ReactionsService) DeleteTeamDiscussionReaction(ctx context.Context, org, teamSlug string, discussionNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("orgs/%v/teams/%v/discussions/%v/reactions/%v", org, teamSlug, discussionNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteTeamDiscussionReactionByOrgIDAndTeamID deletes the reaction to a team discussion by organization ID and team ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-reaction
func (s *ReactionsService) DeleteTeamDiscussionReactionByOrgIDAndTeamID(ctx context.Context, orgID, teamID, discussionNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("organizations/%v/team/%v/discussions/%v/reactions/%v", orgID, teamID, discussionNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListTeamDiscussionCommentReactions lists the reactions for a team discussion comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-team-discussion-comment
func (s *ReactionsService) ListTeamDiscussionCommentReactions(ctx context.Context, teamID int64, discussionNumber, commentNumber int, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/comments/%v/reactions", teamID, discussionNumber, commentNumber)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, nil, err
	}
	return m, resp, nil
}

// CreateTeamDiscussionCommentReaction creates a reaction for a team discussion comment.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-team-discussion-comment
func (s *ReactionsService) CreateTeamDiscussionCommentReaction(ctx context.Context, teamID int64, discussionNumber, commentNumber int, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/comments/%v/reactions", teamID, discussionNumber, commentNumber)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteTeamDiscussionCommentReaction deletes the reaction to a team discussion comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-comment-reaction
func (s *ReactionsService) DeleteTeamDiscussionCommentReaction(ctx context.Context, org, teamSlug string, discussionNumber, commentNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("orgs/%v/teams/%v/discussions/%v/comments/%v/reactions/%v", org, teamSlug, discussionNumber, commentNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteTeamDiscussionCommentReactionByOrgIDAndTeamID deletes the reaction to a team discussion comment by organization ID and team ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-comment-reaction
func (s *ReactionsService) DeleteTeamDiscussionCommentReactionByOrgIDAndTeamID(ctx context.Context, orgID, teamID, discussionNumber, commentNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("organizations/%v/team/%v/discussions/%v/comments/%v/reactions/%v", orgID, teamID, discussionNumber, commentNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

func (s *ReactionsService) deleteReaction(ctx context.Context, url string) (*Response, error) {
	req, err := s.client.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	return s.client.Do(ctx, req, nil)
}
`

var reactionsGoFileWant = `// Copyright 2016 The go-github AUTHORS. All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

import (
	"context"
	"fmt"
	"net/http"
)

// ReactionsService provides access to the reactions-related functions in the
// GitHub API.
type ReactionsService service

// Reaction represents a GitHub reaction.
type Reaction struct {
	// ID is the Reaction ID.
	ID     *int64  ` + "`" + `json:"id,omitempty"` + "`" + `
	User   *User   ` + "`" + `json:"user,omitempty"` + "`" + `
	NodeID *string ` + "`" + `json:"node_id,omitempty"` + "`" + `
	// Content is the type of reaction.
	// Possible values are:
	//     "+1", "-1", "laugh", "confused", "heart", "hooray".
	Content *string ` + "`" + `json:"content,omitempty"` + "`" + `
}

// Reactions represents a summary of GitHub reactions.
type Reactions struct {
	TotalCount *int    ` + "`" + `json:"total_count,omitempty"` + "`" + `
	PlusOne    *int    ` + "`" + `json:"+1,omitempty"` + "`" + `
	MinusOne   *int    ` + "`" + `json:"-1,omitempty"` + "`" + `
	Laugh      *int    ` + "`" + `json:"laugh,omitempty"` + "`" + `
	Confused   *int    ` + "`" + `json:"confused,omitempty"` + "`" + `
	Heart      *int    ` + "`" + `json:"heart,omitempty"` + "`" + `
	Hooray     *int    ` + "`" + `json:"hooray,omitempty"` + "`" + `
	URL        *string ` + "`" + `json:"url,omitempty"` + "`" + `
}

func (r Reaction) String() string {
	return Stringify(r)
}

// ListCommentReactionOptions specifies the optional parameters to the
// ReactionsService.ListCommentReactions method.
type ListCommentReactionOptions struct {
	// Content restricts the returned comment reactions to only those with the given type.
	// Omit this parameter to list all reactions to a commit comment.
	// Possible values are: "+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", or "eyes".
	Content string ` + "`" + `url:"content,omitempty"` + "`" + `

	ListOptions
}

// ListCommentReactions lists the reactions for a commit comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-commit-comment
func (s *ReactionsService) ListCommentReactions(ctx context.Context, owner, repo string, id int64, opts *ListCommentReactionOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/comments/%v/reactions", owner, repo, id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateCommentReaction creates a reaction for a commit comment.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-commit-comment
func (s *ReactionsService) CreateCommentReaction(ctx context.Context, owner, repo string, id int64, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/comments/%v/reactions", owner, repo, id)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteCommentReaction deletes the reaction for a commit comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-commit-comment-reaction
func (s *ReactionsService) DeleteCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) (*Response, error) {
	u := fmt.Sprintf("repos/%v/%v/comments/%v/reactions/%v", owner, repo, commentID, reactionID)

	return s.deleteReaction(ctx, u)
}

// DeleteCommentReactionByID deletes the reaction for a commit comment by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-commit-comment-reaction
func (s *ReactionsService) DeleteCommentReactionByID(ctx context.Context, repoID, commentID, reactionID int64) (*Response, error) {
	u := fmt.Sprintf("repositories/%v/comments/%v/reactions/%v", repoID, commentID, reactionID)

	return s.deleteReaction(ctx, u)
}

// ListIssueReactions lists the reactions for an issue.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-an-issue
func (s *ReactionsService) ListIssueReactions(ctx context.Context, owner, repo string, number int, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/%v/reactions", owner, repo, number)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateIssueReaction creates a reaction for an issue.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-an-issue
func (s *ReactionsService) CreateIssueReaction(ctx context.Context, owner, repo string, number int, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/%v/reactions", owner, repo, number)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteIssueReaction deletes the reaction to an issue.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-reaction
func (s *ReactionsService) DeleteIssueReaction(ctx context.Context, owner, repo string, issueNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repos/%v/%v/issues/%v/reactions/%v", owner, repo, issueNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteIssueReactionByID deletes the reaction to an issue by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-reaction
func (s *ReactionsService) DeleteIssueReactionByID(ctx context.Context, repoID, issueNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repositories/%v/issues/%v/reactions/%v", repoID, issueNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListIssueCommentReactions lists the reactions for an issue comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-an-issue-comment
func (s *ReactionsService) ListIssueCommentReactions(ctx context.Context, owner, repo string, id int64, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/comments/%v/reactions", owner, repo, id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateIssueCommentReaction creates a reaction for an issue comment.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-an-issue-comment
func (s *ReactionsService) CreateIssueCommentReaction(ctx context.Context, owner, repo string, id int64, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/issues/comments/%v/reactions", owner, repo, id)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteIssueCommentReaction deletes the reaction to an issue comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-comment-reaction
func (s *ReactionsService) DeleteIssueCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repos/%v/%v/issues/comments/%v/reactions/%v", owner, repo, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteIssueCommentReactionByID deletes the reaction to an issue comment by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-an-issue-comment-reaction
func (s *ReactionsService) DeleteIssueCommentReactionByID(ctx context.Context, repoID, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repositories/%v/issues/comments/%v/reactions/%v", repoID, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListPullRequestCommentReactions lists the reactions for a pull request review comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-pull-request-review-comment
func (s *ReactionsService) ListPullRequestCommentReactions(ctx context.Context, owner, repo string, id int64, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/pulls/comments/%v/reactions", owner, repo, id)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreatePullRequestCommentReaction creates a reaction for a pull request review comment.
// Note that if you have already created a reaction of type content, the
// previously created reaction will be returned with Status: 200 OK.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-pull-request-review-comment
func (s *ReactionsService) CreatePullRequestCommentReaction(ctx context.Context, owner, repo string, id int64, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("repos/%v/%v/pulls/comments/%v/reactions", owner, repo, id)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeletePullRequestCommentReaction deletes the reaction to a pull request review comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-pull-request-comment-reaction
func (s *ReactionsService) DeletePullRequestCommentReaction(ctx context.Context, owner, repo string, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repos/%v/%v/pulls/comments/%v/reactions/%v", owner, repo, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeletePullRequestCommentReactionByID deletes the reaction to a pull request review comment by repository ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-a-pull-request-comment-reaction
func (s *ReactionsService) DeletePullRequestCommentReactionByID(ctx context.Context, repoID, commentID, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("repositories/%v/pulls/comments/%v/reactions/%v", repoID, commentID, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListTeamDiscussionReactions lists the reactions for a team discussion.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-team-discussion-legacy
func (s *ReactionsService) ListTeamDiscussionReactions(ctx context.Context, teamID int64, discussionNumber int, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/reactions", teamID, discussionNumber)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// CreateTeamDiscussionReaction creates a reaction for a team discussion.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-team-discussion-legacy
func (s *ReactionsService) CreateTeamDiscussionReaction(ctx context.Context, teamID int64, discussionNumber int, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/reactions", teamID, discussionNumber)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteTeamDiscussionReaction deletes the reaction to a team discussion.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-reaction
func (s *ReactionsService) DeleteTeamDiscussionReaction(ctx context.Context, org, teamSlug string, discussionNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("orgs/%v/teams/%v/discussions/%v/reactions/%v", org, teamSlug, discussionNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteTeamDiscussionReactionByOrgIDAndTeamID deletes the reaction to a team discussion by organization ID and team ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-reaction
func (s *ReactionsService) DeleteTeamDiscussionReactionByOrgIDAndTeamID(ctx context.Context, orgID, teamID, discussionNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("organizations/%v/team/%v/discussions/%v/reactions/%v", orgID, teamID, discussionNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// ListTeamDiscussionCommentReactions lists the reactions for a team discussion comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#list-reactions-for-a-team-discussion-comment-legacy
func (s *ReactionsService) ListTeamDiscussionCommentReactions(ctx context.Context, teamID int64, discussionNumber, commentNumber int, opts *ListOptions) ([]*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/comments/%v/reactions", teamID, discussionNumber, commentNumber)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	var m []*Reaction
	resp, err := s.client.Do(ctx, req, &m)
	if err != nil {
		return nil, nil, err
	}
	return m, resp, nil
}

// CreateTeamDiscussionCommentReaction creates a reaction for a team discussion comment.
// The content should have one of the following values: "+1", "-1", "laugh", "confused", "heart", "hooray".
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#create-reaction-for-a-team-discussion-comment-legacy
func (s *ReactionsService) CreateTeamDiscussionCommentReaction(ctx context.Context, teamID int64, discussionNumber, commentNumber int, content string) (*Reaction, *Response, error) {
	u := fmt.Sprintf("teams/%v/discussions/%v/comments/%v/reactions", teamID, discussionNumber, commentNumber)

	body := &Reaction{Content: String(content)}
	req, err := s.client.NewRequest("POST", u, body)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", mediaTypeReactionsPreview)

	m := &Reaction{}
	resp, err := s.client.Do(ctx, req, m)
	if err != nil {
		return nil, resp, err
	}

	return m, resp, nil
}

// DeleteTeamDiscussionCommentReaction deletes the reaction to a team discussion comment.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-comment-reaction
func (s *ReactionsService) DeleteTeamDiscussionCommentReaction(ctx context.Context, org, teamSlug string, discussionNumber, commentNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("orgs/%v/teams/%v/discussions/%v/comments/%v/reactions/%v", org, teamSlug, discussionNumber, commentNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

// DeleteTeamDiscussionCommentReactionByOrgIDAndTeamID deletes the reaction to a team discussion comment by organization ID and team ID.
//
// GitHub API docs: https://docs.github.com/en/free-pro-team@latest/rest/reference/reactions/#delete-team-discussion-comment-reaction
func (s *ReactionsService) DeleteTeamDiscussionCommentReactionByOrgIDAndTeamID(ctx context.Context, orgID, teamID, discussionNumber, commentNumber int, reactionID int64) (*Response, error) {
	url := fmt.Sprintf("organizations/%v/team/%v/discussions/%v/comments/%v/reactions/%v", orgID, teamID, discussionNumber, commentNumber, reactionID)

	return s.deleteReaction(ctx, url)
}

func (s *ReactionsService) deleteReaction(ctx context.Context, url string) (*Response, error) {
	req, err := s.client.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	// TODO: remove custom Accept headers when APIs fully launch.
	req.Header.Set("Accept", mediaTypeReactionsPreview)

	return s.client.Do(ctx, req, nil)
}
`
