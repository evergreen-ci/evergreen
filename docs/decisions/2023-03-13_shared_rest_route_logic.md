# 2023-03-13 Shared REST Route Logic

- status: accepted
- date: 2023-03-13
- authors: Malik Hadjri

## Context and Problem Statement

Ticket: EVG-16701

A feature request was made for a new endpoint to perform a bulk operation on a group of versions. Since there was an existing GET endpoint to retrieve a list of versions based off of detailed request parameters, the
PR to implement the new endpoint was made to use the existing GET endpoint's logic to retrieve the list of versions and then perform the bulk operation on the list of versions. This was done to avoid duplicating request parsing and version retrieving
logic.

However, this refactoring also included some cleanup of the existing GET endpoint, doing things such as tweaking the names of request variables and slightly changing the way that the request parameters were applied.
The existing endpoint was a longstanding one that users' scripts were relying on, so this seemingly minor change caused a breaking change for users' scripts and a rollback was needed.

## Decision Outcome

Despite the potential similarities of the two routes, they ultimately serve different purposes and should be treated as such. The GET route is meant to be a general purpose route that can be used to retrieve a list of versions based off of a variety of request parameters. By contrast, the new route is meant to be a more specific route that is used to perform a priority-specific operation on a group of versions.

Because of this, we should consider that refactoring existing logic to suit new feature requests isn't always the ideal solution, especially when it requires that we make significant changes to existing endpoints and their logic. If we do go forward with such a decision, we'll want to at minimum communicate this beforehand.
