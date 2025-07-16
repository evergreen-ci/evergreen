# 2023-03-30 Project Create and Enable Logic

- status: accepted
- date: 2023-03-30
- authors: Bynn Lee

## Context and Problem Statement

Ticket: PM-1403

Because Evergreen admins can now create projects, we wanted a way to limit users from creating and enabling too many projects or else that could result in us constantly hitting our GitHub API limit.

## Decision Outcome

By running an aggregation, I found that all owner, repo combinations only own 10 projects max at the moment. We set the current repo project limit to 15.
One exception to that was mongodb/mongo which had 23 projects and has been added to the override list.
I also analyzed our average GitHub API usage and determined that we would be able to support around 400 projects compared to the current 315 projects right now.

I also got rid of the ability to default to repo on enabled for projects.
This feature was not being used by any project owner and was slowing our abilities to query for enabled projects and making the code overly complicated.
