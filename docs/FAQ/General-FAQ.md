# General FAQ

## What is the difference between cron, batchtime, and periodic build?

- **Batchtime**: Delays activating a task on an existing mainline version until a specified time has passed since it's last run.
- **Cron**: Activates a task on an existing mainline version at a specified time or interval.
- **Periodic Builds**: Creates a new version that runs at a specified time or interval, regardless if the project has had any commits.

For more on their differences and examples, see [controlling when tasks run](Project-Configuration/Controlling-when-tasks-run).

## Why am I seeing a 'fatal: ...no merge base' error?

This is most likely because your repo was cloned with a specified depth and the merge base was outside the range of the depth. To fix this, rebase your HEAD to the latest master. If this happens often, we recommend increasing the clone depth in your project's [`git.get_project`](Project-Configuration/Project-Commands#gitget_project) to a more suitable depth.
