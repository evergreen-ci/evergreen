# GitHub Merge Queue

[GitHub's merge queue](https://github.blog/2023-07-12-github-merge-queue-is-generally-available/)
ensures that all pull requests pass required tests, rebased on HEAD, and it
batches pull requests to test them as a unit to increase throughput.

Gating every merge on a green build means every commit on the tracked branch had a green build. This way:

- No one bases their work on broken code.
- Every commit on the mainline branch is potentially releasable/deployable.

To turn it on, you must turn on Evergreen's merge queue integration, and then
turn on the GitHub merge queue in GitHub.

GitHub's merge queue requires that you have write access to the repository to
merge, like you would have to without the queue.

Evergreen will fail the entire version if any task in a merge queue version
fails, so only include tasks that must pass for a merge queue version to pass.
That is, in the GitHub section of your project settings in Evergreen, you can
set Patch Definitions for GitHub Pull Request Testing that select tasks beyond
those required by your GitHub branch protection rules. But in the Patch
Definitions for the Merge Queue, the selected tasks must be exactly those
required by your GitHub branch protection rules.

## Enable the merge queue

### Turn on Evergreen's merge queue integration

1. From <https://spruce.corp.mongodb.com/>, from the More drop down, select Project Settings.
2. Select your project from the project dropdown.
3. From the GitHub section, set the Merge Queue to Enabled.
4. Add variant and task tags or regexes for the variants and tasks you wish to run when a pull request is added to the queue.

### Turn on the GitHub merge queue

To set a branch protection rule for the "evergreen" GitHub status, which is used by the merge queue, follow these steps:

1. Navigate to the repository's **Settings** page on GitHub.
2. Click on the **Branches** tab.
3. Scroll down to the **Branch protection rules** section and click on the **Add rule** or **Edit rule** button.
4. Enable the **Require a pull request before merging** option.
5. Enable the **Require status checks to pass before merging** option.
6. Uncheck **Require branches to be up to date before merging** unless you'd
   like to require users to rebase code on the branch. Note, however, that this
   would require users to manually update their PRs.
7. Under the **Status checks** section, select the **evergreen** check from the list of available status checks.
8. Enable **Require merge queue**.
9. Save the branch protection rule.

By setting this branch protection rule, the "evergreen" status will be required
to pass before any changes can be merged into the protected branch.
Alternatively, you can require a single or multiple variants to pass before
merging, instead of all variants.

## Merge Queue Behavior

Concurrency is on by default for the GitHub merge queue. If there are multiple
PRs in the queue, your PR might be tested with other commits. This means that
the Evergreen versions on a project patches page might be testing your PR even
if they have a different merge queue title. This title is the title of the
HEAD PR of a merge group, but the merge group could contain multiple PRs. Note
that GitHub merges all commits from each PR before adding that PR to a version,
so a given version has as many commits in it as there are PRs in it.

The merge queue is not trying to merge individual PRs, but groups of PRs. This
leads to some unintuitive behavior. Here is a typical sequence of events when a
user adds 2 PRs to the merge queue. In this case we assume "Minimum pull
requests to merge" is set to 1, "Maximum pull requests to merge" is set to 5,
and "Maximum pull requests to build" ("Build concurrency") is set to 5.

The diagram names its branches like "main/pr-1", but in reality they are named
like "gh-readonly-queue/main/pr-1-\<hash\>".

```mermaid
sequenceDiagram
    User->>GitHub: Add PR 1 to merge queue
    Note over GitHub: Create branch main/pr-1
    GitHub->>Evergreen: Send merge_group webhook
    Note over Evergreen: Create version for PR 1
    Evergreen->>GitHub: Clone main/pr-1
    User->>GitHub: Add PR 2 to merge queue
    Note over GitHub: Create branch main/pr-2
    GitHub->>Evergreen: Send merge_group webhook
    Note over Evergreen: Create version for PR 1 + 2
    Evergreen->>GitHub: Clone main/pr-2
    Note over Evergreen: Tasks pass for PR 1
    Evergreen->>GitHub: Send variant and version statuses for PR 1
    Note over Evergreen: Tasks pass for PR 1 + 2
    Evergreen->>GitHub: Send variant and version statuses for PR 1 + 2
    Note over GitHub: Merge PR 1 + 2
```

There is some unintuitive behavior to be aware of:

1. If main/pr-1 succeeds and main/pr-2 fails, only main/pr-1 will be merged.
   _But note that merging main/pr-1 waits for main/pr-2 to fail or for the timeout
   to be hit, because the "Maximum pull requests to merge" is set to 5. That is,
   GitHub conceptualizes the fundamental unit of work as the merge group, not the
   PR._
2. If main/pr-1 fails and main/pr-2 succeeds, GitHub will remove pr-1 from the
   queue and re-run main/pr-2 (not rebased on pr-1). _This is conceptually similar
   to the above. GitHub is trying to merge a group of PRs, not individual PRs._
3. If both succeed, main/pr-2 (rebased on pr-1) will be merged. _But merging
   main/pr-1 waits for main/pr-2 to succeed, because the maximum pull requests to
   merge is set to 5, and, again, GitHub is testing the entire group._

If, in the branch protection rules, "only merge non-failing pull requests" is
checked, then the merge will not happen if any of the PRs fail. Otherwise, if
main/pr-1 is red and main/pr-2 is green, then the latter will be merged, which
contains both PRs. Note that, although this says "pull requests," it's really
about merge queue behavior. All pull requests must pass the branch protection
rules before they can be added to the merge queue. This setting is only about
the behavior once they're in the merge queue.

The temporary branch gets deleted only after the PR is merged, or if the PR
fails the check or is removed from the queue.

## Merge Queue Settings

GitHub's merge queue docs and UI hints can be confusing. The descriptions of the
merge queue settings in the repo branch protection rules and in the rulesets are
different, and the ones in the rulesets are more accurate. See the table below
for a comparison.

| Repo Setting                   | Repo Description                                                                   | Ruleset Setting                      | Ruleset Description                                                                                                                                                                                                     |
| ------------------------------ | ---------------------------------------------------------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Maximum pull requests to build | Limit the number of queued pull requests running at the same time                  | Build concurrency                    | Limit the number of queued pull requests requesting checks and workflow runs at the same time.                                                                                                                          |
| Minimum pull requests to merge | no description                                                                     | Minimum group size                   | The minimum number of PRs that will be merged together in a group.                                                                                                                                                      |
| or after N minutes             | no description                                                                     | Wait time to meet minimum group size | The time merge queue should wait after the first PR is added to the queue for the minimum group size to be met. After this time has elapsed, the minimum group size will be ignored and a smaller group will be merged. |
| Maximum pull requests to merge | no description                                                                     | Maximum group size                   | The maximum number of PRs that will be merged together in a group.                                                                                                                                                      |
| Status check timeout           | Time a required status check must report a conclusion to not be considered failed. | Status check timeout                 | Maximum time for a required status check to report a conclusion. After this much time has elapsed, checks that have not reported a conclusion will be assumed to have failed.                                           |

## Useful Links for Troubleshooting

Here are example links for the 10gen/mongo repository:

- <https://github.com/10gen/mongo/activity?actor=github-merge-queue%5Bbot%5D> : View the branch creations and deletions by clicking the Activity link on the repository main page under the About section.
- <https://github.com/10gen/mongo/queue/master> : View the queue itself.
- <https://github.com/10gen/mongo/branches/all?query=gh-readonly> : View the active merge queue branches.

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
whether or not GitHub will merge the PR). Nevertheless, you can run different
tasks in PRs and the merge queue with the separate settings on the GitHub &
Merge Queue project settings page.

Evergreen will post a status check called "evergreen" when the entire merge
queue build has finished, and it will post a status for each variant that
finishes called "evergreen/<variant_name>". Typically projects should set a branch
protection rule for "evergreen". However, it's also possible to instead to set a
branch protection rule for one or more "evergreen/<variant_name>" statuses. You
might wish to do this if you wish more variants to run in a PR than are actually
required to merge. For example, there may be long-running tasks which users care
about only some of the time, and you do not wish to block PRs or merges on those
tasks, but you still wish them to run automatically.

**Q:** How do I add a new branch protection rule for a new variant?

**A:** If you rely on having branch protection rules for individual variants, then changes for this need to be made both
in Evergreen and in GitHub. In Evergreen, at least one task from this variant needs to be added to both the PR aliases
and the merge queue aliases (as mentioned above, these rules apply to both the PR and
the merge queue). Ensure you've saved the page.

In GitHub, you can now add a new branch protection rule for the "evergreen/<variant_name>" status check.

**Q:** Can I reduce what variants are run in the merge queue based on what's being tested?

**A:** Yes, by using [Build Variant Path Filtering](Project-Configuration-Files.md#build-variant-path-filtering)
_(This is currently disabled, but will be rolled out by the end of 2/2026.)_
Note that this will only run tasks that match the paths for the CURRENT merge group. For example, if the first patch in the merge queue
modifies only the `src` directory, and the second patch modifies the `test` directory, the second patch will only run variants
that match the `test` directory.

**Q:** Can I disable path filtering for the merge queue while keeping it for PR patches?

**A:** Yes, you can set `disable_merge_queue_path_filtering: true` at the [top level of your project YAML](Project-Configuration-Files.md#disabling-merge-queue-path-filtering).
This will skip path filtering for merge queue versions while still applying it to PR patches.
This is useful when you want selective testing for PRs but comprehensive testing before merging.

**Q:** How can I turn off the merge queue to block users from merging?

**A:** Check the "Lock branch" setting in the branch protection rules. There are
no changes to make on the Evergreen side.

**Q:** Can I see the patch or patches associated with my merge attempt?

**A:** GitHub abstracts the process of making builds from the queue. There might be
many builds (which Evergreen calls versions) associated with your PR, and it
might not be obvious when GitHub decided to make them. To see the behavior of
the queue, you can look at the Activity page, accessible from under the About
section of a repo, and limit the user to "GitHub Merge Queue[bot]", e.g.,
<https://github.com/10gen/mongo/activity?actor=github-merge-queue%5Bbot%5D>. On
the Evergreen side, you can click on More -> Project Patches, and look for
patches prepended "GitHub Merge Queue:", e.g.,
<https://spruce.corp.mongodb.com/project/mongodb-mongo-master/patches>. There is not
a way, however, to map directly from a GitHub PR to its patches.

**Q:** Why can't I activate, deactivate, or restart tasks in my running merge queue version?

**A:** This is a protection mechanism to ensure that the tasks in a merge queue version
exactly represent the merge queue alias definition. Instead, you should rely on the
UI available in GitHub to modify their patches. You can remove/requeue items in the queue
via GitHub if needed.

**Q:** What does it mean if GitHub times out my merge queue request in my PR?

**A:** There is a setting called "Status check timeout" in the branch protection rules
or rulesets. This setting is the maximum time for a required status check to
report success or failed. This is _not_ the same as the makespan of an
Evergreen version, for two reasons:

1. Makespan does not start until the version starts running, but there is time
   in between when GitHub sends a webhook and when the version starts running. If
   Evergreen is under load, it might not schedule the version for some time.
2. The status checks might be configured to listen for variant statuses, not
   version statuses.

**Q:** How can I make the merge queue work better for flaky tasks?

**A:** In your branch protection rules or rulesets, under
"[Managing a merge queue](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue#managing-a-merge-queue)",
set "Only merge non-failing pull requests" to **No**. When this setting is disabled,
if one PR in a merge group fails but another succeeds, GitHub will still merge the
successful group. This is useful when you have flaky tasks that occasionally fail
but don't indicate a real problem with the code. Note that this setting only affects
behavior within the merge queue (i.e. all PRs must still pass branch protection rules
before they can be added to the queue).

## Additional Resources

For more information on GitHub's merge queue feature and how to customize its
settings, refer to the [official GitHub documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue).

## Monitoring and Alerting

### Available Metrics

#### Queue Depth Metrics (Sampled Every 5 Minutes)

Evergreen samples merge queue depth metrics every 5 minutes and emits them to Honeycomb as `merge_queue.depth_sample` spans. These metrics help you monitor queue backlogs, capacity, and identify stuck patches.

- `evergreen.merge_queue.depth` - Total patches in queue (sampled every 5 minutes)
  - **Note:** Because items in the merge queue are grouped together, the merge queue depth may not match the number of items you see in the queue. For example, there may be 10 items in the queue but only 4 patches and a depth of 4 because of how they are grouped together.
- `evergreen.merge_queue.pending_count` - Patches not yet started
- `evergreen.merge_queue.running_count` - Patches currently running
- `evergreen.merge_queue.running_tasks_count` - Count of running tasks across all patches in queue
- `evergreen.merge_queue.has_running_tasks` - Whether any queue patches have running tasks
- `evergreen.merge_queue.oldest_patch_age_ms` - Age of oldest pending patch
- `evergreen.merge_queue.top_of_queue_patch_id` - Patch ID at the top of the queue
- `evergreen.merge_queue.top_of_queue_status` - Status of the patch at the top of the queue
- `evergreen.merge_queue.top_of_queue_sha` - SHA of the patch at the top of the queue

These metrics include standard attribution fields (`project_id`, `org`, `repo`, `queue_name`, `base_branch`) for filtering and grouping.

#### Lifecycle Event Metrics

Evergreen emits OpenTelemetry spans at key points in the merge queue lifecycle:

- `merge_queue.intent_created` - When a merge queue patch is created
- `merge_queue.patch_processing` - When patch processing begins
- `merge_queue.patch_completed` - When a merge queue version completes. Status is determined from removal reason if the patch was removed from the queue by GitHub, otherwise from the final version status.

**Latency Metrics:**

- `evergreen.merge_queue.time_in_queue_ms` - The total time from merge queue entry to completion
- `evergreen.merge_queue.time_to_first_task_ms` - The time from merge queue entry to first task start
- `evergreen.merge_queue.slowest_task_duration_ms` - The duration of the slowest task in the patch

**Health & Failure Metrics:**

- `evergreen.merge_queue.status` - The outcome of the merge queue build, emitted in `merge_queue.patch_completed` spans when the version finishes. Determined from `removal_reason` if present (patch was removed from queue), otherwise from final version status. Values:
  - `"success"` - Merged successfully (reason: "merged") or version succeeded
  - `"failed"` - Failed status checks (reason: "invalidated") or version failed
  - `"removed"` - Manually dequeued or other removal (reason: "dequeued")
- `evergreen.merge_queue.removal_reason` - The reason GitHub removed the patch from the queue ("invalidated", "merged", or "dequeued"). Present in `merge_queue.patch_completed` spans when the patch was removed from the queue.
- `evergreen.merge_queue.has_test_failure` - Whether the version contains any tasks that failed due to test failures
- `evergreen.merge_queue.has_system_failure` - Whether the version contains any tasks that failed due to system failures
- `evergreen.merge_queue.has_setup_failure` - Whether the version contains any tasks that failed due to setup failures
- `evergreen.merge_queue.has_timeout_failure` - Whether the version contains any tasks that failed due to timeouts
- `evergreen.merge_queue.failed_task_count` - The number of failed and aborted tasks
- `evergreen.merge_queue.total_task_count` - The total number of tasks
- `evergreen.merge_queue.has_running_tasks` - Whether tasks are currently running

**Attribution Metrics:**

- `evergreen.merge_queue.project_id` - The project identifier
- `evergreen.merge_queue.org` - The GitHub organization
- `evergreen.merge_queue.repo` - The GitHub repository
- `evergreen.merge_queue.queue_name` - The name of the queue (same as base_branch)
- `evergreen.merge_queue.base_branch` - The base branch for the merge queue (e.g., "main")
- `evergreen.merge_queue.variants` - A comma-separated list of build variants
- `evergreen.merge_queue.slowest_task_id` - The ID of the slowest task
- `evergreen.merge_queue.slowest_task_name` - The name of the slowest task
- `evergreen.merge_queue.slowest_task_variant` - The variant of the slowest task

**Debugging Attributes:**

- `evergreen.merge_queue.patch_id` - The Evergreen patch ID
- `evergreen.merge_queue.head_sha` - The SHA of the merge queue head
- `evergreen.merge_queue.msg_id` - The GitHub webhook message ID (available in intent_created spans)
- `evergreen.merge_queue.github_head_pr_url` - The GitHub PR URL for the HEAD PR of the merge queue entry. When concurrency is enabled (the default), a merge queue entry may contain multiple PRs tested together, but this URL only points to the HEAD PR of that merge group.

### Example Honeycomb Queries

#### Queue Depth Analysis

**Queue depth over time:**

```text
WHERE evergreen.merge_queue.depth `exists`
VISUALIZE MAX(evergreen.merge_queue.depth)
GROUP BY evergreen.merge_queue.project_id
```

**Times queue depth exceeded 10:**

```text
WHERE evergreen.merge_queue.depth > 10
COUNT
TIMEFRAME: Last 30 days
```

This shows how many 5-minute samples had depth > 10 in the last month.

**Oldest patch age (P95):**

```text
WHERE evergreen.merge_queue.oldest_patch_age_ms `exists`
VISUALIZE P95(evergreen.merge_queue.oldest_patch_age_ms)
GROUP BY evergreen.merge_queue.project_id
```

#### Lifecycle Analysis

**Average time in queue:**

```text
WHERE evergreen.merge_queue.status `exists`
VISUALIZE P50(evergreen.merge_queue.time_in_queue_ms),
         P95(evergreen.merge_queue.time_in_queue_ms)
GROUP BY evergreen.merge_queue.project_id
```

**Failure type breakdown:**

```text
WHERE evergreen.merge_queue.status = "failed"
VISUALIZE COUNT
GROUP BY evergreen.merge_queue.has_test_failure,
         evergreen.merge_queue.has_system_failure,
         evergreen.merge_queue.has_setup_failure,
         evergreen.merge_queue.has_timeout_failure
```

**Time to first task (P95):**

```text
WHERE evergreen.merge_queue.time_to_first_task_ms `exists`
VISUALIZE P95(evergreen.merge_queue.time_to_first_task_ms)
GROUP BY evergreen.merge_queue.project_id
```

**Removal reason breakdown:**

```text
WHERE evergreen.merge_queue.removal_reason `exists`
VISUALIZE COUNT
GROUP BY evergreen.merge_queue.status, evergreen.merge_queue.removal_reason
```

### Setting Up Alerts

You can create Honeycomb Triggers to alert on merge queue issues:

1. In Honeycomb, navigate to Triggers → Create Trigger
2. Build your query (e.g., `WHERE evergreen.merge_queue.time_in_queue_ms > 3600000 COUNT`)
3. Set threshold
4. Configure notification channels (e.g. Slack, Email)
5. Set evaluation frequency (e.g., every 5 minutes)

**Example alerts:**

- **High queue depth:** Alert when depth > 10 for > 15 minutes
- **Stuck patches:** Alert when oldest patch age > 60 minutes and running_tasks_count = 0
- **Slow processing:** Alert when P95 time in queue > 60 minutes
- **High failure rate:** Alert when failure rate > 20% over the last hour

For detailed Honeycomb trigger setup instructions, see the [Honeycomb documentation](https://docs.honeycomb.io/working-with-your-data/triggers/).

## For Evergreen Developers

To troubleshoot why a merge group version is not being created, find the `event
= merge_group` message in Splunk. You can then search for the values of the
`head_sha` and `msg_id` properties to track merge intent creation and the amboy
job, as well as find the branch that Evergreen will clone for that project.
