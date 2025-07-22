# 2023-03-24 REST route authentication exception

- status: accepted
- date: 2023-03-24
- authors: Malik Hadjri

## Context and Problem Statement

Ticket: EVG-19068

Recently all REST endpoints in Evergreen have been changed to require authentication across the board. However, this has caused our /dockerfile endpoint to consistently return 401 codes to our docker client. Since all the route does is serve a static string, we initially thought it could be removed altogether and move the string to the docker client side of our code.

However, there is some unexpected complexity here because of the way we use Docker's ImageBuild api which requires us to pass in some endpoint to retrieve the dockerfile from, which currently is our /dockerfile endpoint, and it's not straightforward to remove that logic and directly provide the dockerfile string.

## Decision Outcome

Since we are just serving a static string for this route, and given the non-trivial work needed to remove the route / give it auth, the simplest path forward was to make an exception to remove auth from only this route.
