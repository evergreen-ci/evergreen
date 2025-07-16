# 2023-07-10 Essential Tasks

- status: accepted
- date: 2023-05-18
- authors: Kim Tao

## Context and Problem Statement

Ticket: EVG-19325

Evergreen has historically considered a patch successful if at least one task has run, there are no further tasks
actively waiting to run, and none of the tasks have failed. This creates a loophole in the integration between GitHub
and Evergreen where all the automatically-configured tasks that are expected to pass for the GitHub PR checks might not
actually run if a user intervenes to deactivate them.

## Decision Outcome

The behavior was changed so that all tasks automatically selected to run for GitHub PR patches must be in a finished
state before the version can be marked successful. This still allows users to manually schedule more (optional) tasks on
top of the auto-selected ones, while ensuring that the essential ones need to finish for the GitHub status to show
successful. This is encoded in each task.

One alternative that was considered was to redefine a successful patch as one that has finished all of its tasks, so
that any deactivated task prevents a build/version from being marked finished. However, this came with downsides such as
the fact that all builds/versions, regardless of whether it's a PR patch or something else, share the same logic for how
they decide their status. This would be problematic for other versions like mainline commits, which often intentionally
have many deactivated tasks and would never be considered finished. Isolating the status redefinition logic to specially
handle GitHub patches would be difficult to maintain. For these reasons, it was rejected.
