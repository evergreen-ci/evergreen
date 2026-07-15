# 2026-07-07 GitHub PR Alias Selection via Required Labels

- status: proposed
- date: 2026-07-07
- authors: Maciej Karaś

## Context and Problem Statement

`github_pr_aliases` select which variants/tasks run on a GitHub PR, but the set is
static per project config: every alias entry runs on every PR. Teams want to scope
expensive or situational checks (e.g. e2e, upgrade tests) to only the PRs that need
them, without editing config or pushing new commits, to cut PR-check cost and keep
default PR runs fast. How should Evergreen let a PR dynamically opt into additional
alias-selected tasks, using a signal already native to GitHub PRs (DEVPROD-36828)?

## Considered Options

GitHub labels were chosen as the opt-in signal because they're a first-class GitHub PR
concept: users can add/remove them without touching YAML or making commits, and GitHub
already restricts who can mutate them on a PR.

For materializing newly-implied tasks when a label is added after a version already
exists, three options were considered:

1. **In-place injection** into the existing PR version for the current head SHA,
   reusing the `generate.tasks` add-tasks machinery.
2. **A separate version per label group**, created alongside the original PR version.
3. **Recreate a single version** from the union of default and label-implied tasks
   (supersede the existing version).

Option 2 was rejected because it produces multiple GitHub statuses per SHA, which is
confusing UX and complicates status rollup. Option 3 was rejected because Evergreen has
no general mechanism to safely replace a version's tasks after work may already be in
progress on it, and doing so would risk losing state or duplicating effort for tasks
that already ran.

## Decision Outcome

Add a `required_labels` field to `github_pr_aliases` entries. An entry contributes its
variant/task selection to a PR when it has no `required_labels` (today's default), or
the PR carries any one of the entry's `required_labels` (OR semantics, exact
case-sensitive match). Multiple entries union as PR aliases do today.

Labels are read at two points:

- **Version creation** (`opened`/`reopened`/`synchronize`/
  `automatic_base_change_succeeded`): the PR's current label set, taken from the
  webhook payload, filters the resolved PR aliases before task/variant selection.
- **`labeled` webhook events**: newly-implied tasks are injected in place into the
  existing PR version for the current head SHA, chosen (Option 1 above) because it
  reuses `generate.tasks`'s existing, proven mechanism for mutating a version's builds
  and tasks in place, and it results in exactly one version and one GitHub status per
  SHA — the best user experience. The injection computes an idempotent delta against
  tasks already present in the version, so repeated or out-of-order `labeled`
  deliveries are safe.

Two related behaviors follow from reusing `generate.tasks`:

- **`unlabeled` is a no-op.** `generate.tasks` only ever adds tasks to a version, never
  removes them, so there is no existing reuse path for removal. Canceling
  mid-flight tasks when a label is removed would also be surprising and unpredictable
  for users, so it was explicitly rejected. Removing a label only affects which
  entries are selected the next time a version is created.
- **No auto-creation of a version from a label.** If a `labeled` event arrives and no
  PR version exists yet for the current SHA (e.g. manual-testing projects, an
  unauthorized PR, or a push still being processed), the event is a no-op. Evergreen
  never creates a version purely from a label change; the labels are still honored the
  next time a version is created normally. This preserves the existing contract for
  projects that only patch on-demand.

Authorization is unchanged: injected tasks inherit the version's existing
activation/authorization state, so an outside-org PR's label-implied tasks remain
unactivated until a maintainer authorizes, via the same gate applied to default PR
tasks. Label additions cannot bypass this gate. Check-run limits and
max-tasks-per-version are re-checked on injection, matching the constraints already
enforced on the `generate.tasks` path.

This is a v1, YAML-only change: there is no project/repo settings UI, GraphQL, or REST
support for `required_labels` yet, only the underlying config and data model field.

## More Information

Positive consequences:

- Reduces PR-check cost by letting teams scope expensive checks to the PRs that need
  them, without config changes or new commits.
- A single version and GitHub status per SHA regardless of how many labels are added,
  preserving today's UX.
- Injection reuses the `generate.tasks` machinery, which is already proven safe for
  in-place version mutation, and the delta computation makes repeated label events
  idempotent.

Negative consequences / limitations:

- Removing a label never cancels or unschedules tasks that were already added; users
  must be aware that `unlabeled` only affects future version creation.
- v1 is YAML-only, so there's no UI/GraphQL/REST visibility or editing of
  `required_labels` until follow-up work adds it.
- The injection job relies on idempotent delta computation and the version-scoped lock
  shared with `generate.tasks`, rather than a fully independent concurrency
  mechanism, so its safety is coupled to `generate.tasks`'s existing guarantees.
