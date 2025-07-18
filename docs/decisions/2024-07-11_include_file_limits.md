# 2024-07-11 Appropriate limit for include files per project

- status: approved
- date: 2024-07-11
- authors: Bynn Lee

## Context and Problem Statement

The introduction of the `include` capability to project configuration has increased Evergreen's use of git clones since we ask GitHub for each individual included file separately. We needed a way to stop users from infinitely scaling the number of includes until we hit our GitHub API limit.

Include files take up a significant percentage of all of Evergreen's GitHub operations. After switching to GitHub apps, our new hourly limit is 15,000 requests to GitHub per hour. On average, we only use around 4-8000 of our limit per hour, depending on how active users are. Of those requests, around 60% can be contributed to retrieving include files. For example, on an hour we used 7000 of our limit, we would also have around 4000 include file retrieval requests logged.

The two biggest loads are the the mongo repo projects that have around 25 includes on average, and the mms repo projects which currently have 32 include files. However, the mongo projects have not added extra include files in the past year whereas the mms projects have added almost 10 new include files.

## Decision Outcome

I propose a limit of 35 includes on all projects. Assuming our GitHub usage scales consistently with what we have been seeing, we should be able to handle a 100% increase on the number of include files. So in theory, a limit of 45-50 should not regularly exceed our limits. However, there are intermittent cases (about once a month) where we do end up using 10,000 of our API limit for the hour and setting the limit too high could use up our limit during those times and leaves no room for growth.
