# GitHub Merge Queue

[GitHub's merge queue](https://github.blog/2023-07-12-github-merge-queue-is-generally-available/)
ensures that all pull requests pass required tests, rebased on HEAD, and it
batches pull requests to test them as a unit to increase throughput.

This is an alternative to Evergreen's commit queue, which the Evergreen team
has deprecated in favor of GitHub's merge queue.

Gating every merge on a green build means every commit on the tracked branch had a green build. This way:

* No one bases their work on broken code.
* Every commit on the mainline branch is potentially releasable/deployable.

To turn it on, you must turn on Evergreen's merge queue integration, and then
turn on the GitHub merge queue in GitHub.

You cannot use Evergreen's commit queue if the GitHub merge queue is on.

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

## Concurrency

Concurrency is on by default for the GitHub merge queue. If there are multiple
PRs in the queue, your PR might be tested with other commits. This means that
the Evergreen versions on a project patches page might be testing your PR even
if they have a different commit queue title. This title is the title of the
HEAD PR of a merge group, but the merge group could contain multiple PRs. Note
that GitHub merges all commits from each PR before adding that PR to a version,
so a given version has as many commits in it as there are PRs in it.

## Additional Resources

For more information on GitHub's merge queue feature and how to customize its
settings, refer to the [official GitHub documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue).

## FAQ

**Q:** I don't see any candidate statuses in the list of possible required
checks in the branch protection configuration.

**A:** This is probably because you haven't generated any in a while. GitHub only shows
recent statuses. To get some statuses to choose from, you can retrigger tests on
an existing PR by typing `evergreen retry`, open an empty PR, or commit an empty
commit.

**Q:** Is there a plan to have Evergreen send merge notifications? 

**A:** No. We believe it makes more sense for notifications to come from GitHub,
since it manages the queue, and supports sending notifications. 

**Q:** Is it possible to attribute the merge queue patch to your Evergreen user?

**A:** There’s a many-to-many relationship between versions and PRs. A version
can have multiple authors because GitHub’s merge queue supports concurrency, and a PR can
have multiple versions for the same reason. It’s also possible for a version to
succeed and not yield a merge on GitHub’s side. This makes it difficult to link back
from Evergreen versions to PRs. Instead, users can use the GitHub UI as the primary
starting point, and link to Evergreen builds from there.

**Q:** How can I get the commit titles of the PRs in a merge queue version?

**A:** Evergreen doesn't expose these as an expansion, as they aren't available in the
webhook message. You can use `git` to get them, where `<tracking branch>` is the
branch your project is tracking, usually main or master, since GitHub squashes
each PR's commits into a single commit with the PR title as its commit message.

```shell
git log --pretty=format:"%s" <tracking branch>...HEAD
```

**Q:** Is it possible to get a notification for a merge?

**A:** You can set up email notifications, but the Slack integration does not
send notifications for merges done by the GitHub merge queue.

**Q:** How do branch protection rules apply to PRs and the merge queue?

**A:** The same branch protection rules apply to PRs (which determine whether you can
add the PR to the merge queue) and the merge queue itself (which determines
whether or not GitHub will merge the PR).  Nevertheless, you can run different
tasks in PRs and the merge queue with the separate settings on the GitHub &
Commit Queue project settings page.

Evergreen will post a status check called "evergreen" when the entire merge
queue build has finished, and it will post a status for each variant that
finishes called "evergreen/variant_name". Typically projects should set a branch
protection rule for "evergreen". However, it's also possible to instead to set a
branch protection rule for one or more "evergreen/variant_name" statuses. You
might wish to do this if you wish more variants to run in a PR than are actually
required to merge. For example, there may be long-running tasks which users care
about only some of the time, and you do not wish to block PRs or merges on those
tasks, but you still wish them to run automatically.