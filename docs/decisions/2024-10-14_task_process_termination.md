# 2024-10-14 cleanup task processes by user

- status: accepted
- date: 2024-10-14
- authors: Jonathan Brill

## Context and Problem Statement

Among the inter-task cleanup steps the agent does is terminating processes started by the previous task. On UNIX systems, picking out the specific processes started by a task has always been a challenge. The current solution is to mark processes by setting an environment variable and rely on the task to propagate its environment to any child processes. This doesn't always work so we added an additional check for processes with a working directory set to the task's working directory ([EVG-12948](https://jira.mongodb.org/browse/EVG-12948)). This is not foolproof either. Additionally, when SIP is enabled on macOS a process is not allowed to inspect another processes's environment variables ([DEVPROD-6539](https://jira.mongodb.org/browse/DEVPROD-6539)).

## Considered Options

1. For macOS specifically, the process sandbox ([sandbox-exec](https://reverse.put.as/wp-content/uploads/2011/09/Apple-Sandbox-Guide-v1.0.pdf)) was investigated. It was discovered the sandbox does not support containing processes as it is more geared to limiting the permissions of a process. Additionally, the sandbox is officially deprecated.

2. Manually tracking process forks to reconstruct the process tree despite daemonized processes getting reparented to init. This is overly complex and its implementation would be coupled with the underlying system. For example, auditd could be configured to log when a process forks but it's disabled on macOS and will soon be removed.

3. An easy solution to [DEVPROD-6539](https://jira.mongodb.org/browse/DEVPROD-6539) would have been if we could have somehow made the agent able to read the environment variables of other processes, but it appears Apple has really locked it down. A short list of exceptions appears [here](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/kern/kern_sysctl.c#L1380-L1386) but there's no way to get the agent to qualify as any of them since the entitlement doesn't work.

## Decision Outcome

Start processes as a user that's dedicated to task processes. A utility such as pkill can signal all processes belonging to that user. Turning it on for a distro will require that distro to have an agent user with passwordless sudo and another user to use for tasks.

## More Information

Giving processes a new user isolated from the agent's user will be good for other reasons as well. For one, the agent user's permissions vary and can include passwordless sudo, so a new user will give us the opportunity to apply a more consistently secure profile.
