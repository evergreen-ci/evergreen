# General FAQ

## How and what commit is chosen when running `evergreen patch`?

The Evergreen CLI grabs the user's config file and determines what project should be used. The CLI then establishes a connection with the Evergreen server and retrieves that project's information, specifically the GitHub org + repo + branch (which we'll call, org, repo, and main respectively).

The CLI then fetches the corresponding [remote](https://git-scm.com/docs/git-remote) (which we'll call upstream).
Using that, it runs `git merge-base upstream/main HEAD` to find the common ancestor of the Evergreen tracking branch and the currently checked out branch. This is the commit that will be used as the base for the patch. Changes (including commits and uncommited changes) made after this common ancestor will be included as diff changes in the patch rather than as commits.

Some Caveats to be aware of:

- Projects can configure an oldest allowed merge-base in their project settings. If the found merge-base is older than this, the CLI will error out and not allow the patch to be created. Here is an example of the project setting:

![Oldest Allowed Merge Base](../images/oldest_allowed_merge_base.png)

- If the local instance of the remote branch is out of sync, the CLI will get an older/incorrect merge base. This may result in an error or just unexpected behavior, it depends on how out of sync it is. To solve this, run `git fetch upstream`, where upstream is your remote.

## What is the difference between cron, batchtime, and periodic build?

- **Batchtime**: Delays activating a task on an existing mainline version until a specified time has passed since it's last run.
- **Cron**: Activates a task on an existing mainline version at a specified time or interval.
- **Periodic Builds**: Creates a new version that runs at a specified time or interval, regardless if the project has had any commits.

For more on their differences and examples, see [controlling when tasks run](Project-Configuration/Controlling-when-tasks-run).

## Why am I seeing a 'fatal: ...no merge base' error?

This is most likely because your repo was cloned with a specified depth and the merge base was outside the range of the depth. To fix this, rebase your HEAD to the latest master. If this happens often, we recommend increasing the clone depth in your project's [`git.get_project`](Project-Configuration/Project-Commands#gitget_project) to a more suitable depth.
