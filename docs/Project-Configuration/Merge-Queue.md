# GitHub Merge Queue

[GitHub's merge queue](https://github.blog/2023-07-12-github-merge-queue-is-generally-available/)
ensures that all pull requests pass required tests, rebased on HEAD, and it
batches pull requests to test them as a unit to increase throughput.

This is an alternative to Evergreen's commit queue, which the Evergreen team
plans to deprecate in favor of GitHub's merge queue.

Gating every merge on a green build means every commit on the tracked branch had a green build. This way:

* No one bases their work on broken code.
* Every commit on the mainline branch is potentially releasable/deployable.

To turn it on, you must turn on Evergreen's merge queue integration, and then
turn on the GitHub merge queue in GitHub.

Note that you cannot use Evergreen's commit queue if the GitHub merge queue is on.

## Enable the merge queue

### Turn on Evergreen's merge queue integration

1. From <https://spruce.mongodb.com/>, from the More drop down, select Project Settings.
2. Select your project from the project dropdown.
3. From the GitHub & Commit Queue section, set the Commit Queue to Enabled.
4. For the merge queue type, select the GitHub radio button.
5. Add variant and task tags or regexes for the variants and tasks you wish to run when a pull request is added to the queue.

### Turn on the GitHub merge queue

To set a branch protection rule for the "evergreen" GitHub status, which is used by the merge queue, follow these steps:

1. Navigate to the repository's **Settings** page on GitHub.
2. Click on the **Branches** tab.
3. Scroll down to the **Branch protection rules** section and click on the **Add rule** or **Edit rule** button.
4. Enable the **Require a pull request before merging** option.
5. Uncheck **Require branches to be up to date before merging** unless you'd
   like to require users to rebase code on the branch. Note, however, that this
   would require users to manually update their PRs.
6. Enable the **Require status checks to pass before merging** option.
7. Under the **Status checks** section, select the **evergreen** check from the list of available status checks.
8. Enable **Require merge queue**.
9. Save the branch protection rule.

By setting this branch protection rule, the "evergreen" status will be required
to pass before any changes can be merged into the protected branch. Alternative,
you can require a single or multiple variants to pass before merging, instead of
all variants.

## Additional Resources

For more information on GitHub's merge queue feature and how to customize its
settings, refer to the [official GitHub documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue).

## FAQ

**Q:** Is there a plan to have the merge queue send outcome notifications or
to attribute the merge queue patch to your Evergreen user?

**A:** No. We believe it makes more sense for notifications to come from GitHub,
since it manages the queue, and supports sending notifications. The versions
aren’t commit queue entries, they’re commit queue builds. There’s a many-to-many
relationship between versions and PRs: A version can have multiple authors
because GitHub’s merge queue supports concurrency, and a PR can have multiple
versions for the same reason. It’s also possible for a version to succeed and
not yield a merge on GitHub’s side.

**Q:** Is it possible to get a notification for a merge?

**A:** You can set up email notifications, but the Slack integration does not
send notifications for merges done by the GitHub merge queue.
