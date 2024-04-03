# 2024-03-26 Teardown Group Idleness

- status: accepted
- date: 2024-03-26
- authors: Chaya Malik

## Context and Problem Statement

The check for whether or not a host needs to tear down a group occurs when the next task is assigned. This is because teardown groups can be split among multiple hosts. Therefore, we cannot simply keep track of the number of tasks that need to run and how many have already run. When assigning the next task, if we determine that it is no longer part of the same task group, instead of sending the task to the agent, we send back a nil task and a boolean indicating that the agent should tear down the task group. The agent will then proceed to tear it down and request another task once it is done.

However, we ran into a problem when the jobs that are set up to check in on the hosts (running every 30 seconds) encountered these hosts. These jobs used the presence or absence of a running task, or the time elapsed since the last task, as indicators of the host being up and running and in good shape. As a result, hosts that were not running tasks because they were tearing down a group were vulnerable to being mistaken for being idle and therefore decommissioned.

It is unclear how long this issue has persisted. However, teardown groups are intended for short units of work. Additionally, teardown groups only run after the task has completed, which means there is no immediate indication of their execution except by examining the task logs after the task has finished and has had a chance to run them (immediately after the task finished running it doesn't show any logs from the teardown, it gets appended afterwards). This lack of visibility can lead to silent failures or unmet expectations over an extended period. Additionally, the acceptable idle time for hosts was not a fixed value and was adjusted in admin settings as deemed necessary. For example, on February 23, it was changed from 300 to 30 seconds, and on March 4, it was changed from 30 to 60 seconds. This inadvertently dictated the duration for which teardown groups were allowed to run. This left no visible logs when a host was decommissioned while running a teardown group; it appeared as if the teardown group simply did not run at all.

## Considered Options

One potential fix would be to simply freeze hosts that are running teardown groups until they are done or hit a user-set timeout (with a default if unset). This would work in most cases. However, if something were to happen to the agent making it unable to continue (for example, a panic), the host would remain frozen indefinitely without self-recovery. For tasks, we mitigate this by decomissioning the host if no heartbeat is received within a certain timeframe. However, the heartbeat stops when the task ends, and the teardown occurs after the task is completed.

Another approach to address this issue is to record the start time of the teardown group on the host and utilize it to determine the host's state.

## Decision Outcome

The solution was to add a `TaskGroupTeardownStartTime` field to hosts that sets the time when `assignNextTask` sends back a boolean to the agent indicating that it should tear down the task group. A maximum teardown group threshold of 3 minutes was implemented. The user-configurable teardown group timeout will still be respected, but only if it is less than the maximum teardown group threshold. The choice of 3 minutes aligns with the original intent of teardown tasks being used for short operations while allowing some flexibility for special case scenarios.

The `TaskGroupTeardownStartTime` field was then added to the logic that determines if a host is free or idle. If a host is tearing down a task group but the elapsed time since the teardown started has not yet exceeded the maximum of 3 minutes, the host is allowed to continue tearing down the group.
