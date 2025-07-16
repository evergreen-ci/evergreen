# 2023-08-23 Split Callback Timeout

- status: accepted
- date: 2023-08-23
- authors: Kim Tao

## Context and Problem Statement

The callback timeout was a global configurable timeout that applied to many blocks (post, setup group, teardown task,
and teardown group) in the absence of another more specific timeout. Because the setting applied to so many blocks, this
caused a number of issues:

- The default callback timeout is too short for the post block, because it interacts poorly with the fact that by
  default, failing commands in post do not fail the task. Therefore, tasks can invisibly time out in post without
  affecting the task status, meaning they could fail to do some cleanup and nobody would notice.
- Increasing the callback timeout is bad for teardown group - Evergreen discourages people from increasing callback
  timeout because it applies to the teardown group. This is due to a design issue where the teardown group only runs
  once the agent has already been assigned the next task to run. The consequence is that giving the teardown group more
  time to run holds up the next task it's supposed to run.

Therefore, the callback timeout exists and can be configured but there is no reasonable setting for it - it can be
neither increased (due to teardown group) nor reduced (due to post) without causing unwanted side effects, and in
practice, nobody ever set this value.

## Considered Options

One potentially easy fix would be to remove the callback timeout entirely. This is undesirable because Evergreen must
make sure that all task blocks have some timeout and can't simply run forever.

Another alternative is to just leave it as-is. However, some users have brought up issues where their post block times
out and the only option is to increase callback timeout, which for reasons described above, is undesirable due to
it also increasing the teardown group timeout.

## Decision Outcome

The solution was to split the callback timeout so that there's instead one user-configurable timeout per command block.
Each block should be given a large enough default timeout that existing tasks will not unintentionally hit the timeout
and fail the task (or silently fail to run some commands). The downside of providing a more generous default timeout is
that users may potentially spend more time waiting for tasks than necessary in edge cases (e.g. where their post task is
taking unreasonably long), but this is less problematic than the task invisibly timing out.

The takeaway here is that choosing reasonable defaults are important since they're difficult to change later on, and
when designing a feature, consider how its default behavior could potentially interact with other default behaviors.
