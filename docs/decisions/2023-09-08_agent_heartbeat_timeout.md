# 2023-09-08 Agent Heartbeat Timeout

* status: accepted
* date: 2023-09-08
* authors: Kim Tao

## Context and Problem Statement

The heartbeat provides a mechanism for the agent to prove to the Evergreen application server that it is still running a
task. It is intentionally implemented to keep it as simple as possible, because it is the critical mechanism that
prevents a running task from being considered dead. A serious bug in the heartbeat could cause the task to spuriously
system-fail. Unfortunately, there have been cases where its simplicity and independence from the running task operations
has caused issues. If the foreground operations for the running task have hit an unrecoverable blocking condition (such
as a deadlock), the task may not be actively progressing towards completion but the heartbeat can still report the task
as alive, leaving the task permanently stuck and unable to finish.

## Considered Options

The major question to answer is if the heartbeat cannot run forever, what is a reasonable timeout for the heartbeat? One
option was to set a single (generous) timeout on how long a task could run. This would have the benefit of being simple
and easy to understand. However, the downsides are that enforcing a task maximum runtime cap belongs more in project
validation, where it can check that the user has configured reasonable timeout values. Using a single large timeout also
has the downside of being too generous on how long it allows the task to run to avoid unintentionally timing out a
long-running task.

## Decision Outcome

In order to tie the heartbeat more closely to the task runtime, the heartbeat inherits a timeout from whatever is the
timeout configured for the current task operations, and falls back to a short default timeout in the brief periods where
the agent is in between command blocks. The heartbeat should never hit this heartbeat timeout unless there's a serious
issue in the agent implementation. If the heartbeat has hit the heartbeat timeout, it will stop the heartbeat, and
eventually Evergreen will mark the task as system-failed. To avoid spurious heartbeat timeouts, it also has a grace
period so that even if it hits the timeout, it gives the task some extra time to progress in case the agent somehow
encounters a transient slowdown (such as a slow network).
