# General FAQ

##### What is the difference between cron, batchtime, and periodic build?

The main difference is that cron and batchtime are tied to activating builds for existing commits and periodic builds creates new builds on a schedule regardless of commit activity. For more on their differences and examples, see [controlling when tasks run](Project-Configuration/Controlling-when-tasks-run).

##### Why am I seeing a 'fatal: ...no merge base' error?

This is most likely because your repo was cloned with a specified depth and the merge base was outside the range of the depth. To fix this, rebase your HEAD to the latest master. If this happens often, we recommend increasing the clone depth in your project's [`git.get_project`](Project-Configuration/Project-Commands#gitget_project) to a more suitable depth.  
