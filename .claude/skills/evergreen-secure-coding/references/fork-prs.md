# Fork PRs and GitHub Trust

Code from a pull request opened from a fork is untrusted, even when the PR targets a trusted repository. Evergreen decides whether to run it, with which variables, and on whose behalf.

## Rules

### 1. Check permissions on the base repository and the PR author

- Write or collaborator checks use the **base** owner and repo (`pr.Base.Repo`). Never the head or fork repo, where the PR author always has write access.
- The identity being authorized is the **PR author**. The webhook sender only tells you who triggered the event.
- Every action triggered by a PR comment authorizes the **commenter** against the base repository before acting.

### 2. Untrusted patches get untrusted privileges

- A patch from an unauthorized fork contributor must not receive private or admin-only project variables. It must not get repository write tokens, run on privileged distros, or run until someone approves it.
- Privileges follow the author and origin of the code, not the user who approved or scheduled it.
- Anything that creates or finalizes patches on someone's behalf inherits the original trust level and must not bypass approval.
- Patch-supplied configuration cannot override server-side project settings or the commands run by privileged tooling.

### 3. Tokens are scoped to the project's repositories

GitHub App tokens handed to tasks are restricted to the repositories the project is configured for, with the minimum permissions. Nothing supplied by a patch or task adds repositories to that set.

## Tests

- A PR from a fork whose author is not a collaborator on the base repo is not authorized.
- A comment from a non-collaborator does not trigger any action.
- An unauthorized-fork task does not receive admin-only variables.
