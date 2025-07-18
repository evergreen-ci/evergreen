# 2023-07-18 Evergreen Smoke Test with Manual Patch

- status: accepted
- date: 2023-07-18
- authors: Kim Tao

## Context and Problem Statement

Evergreen's smoke test is valuable for catching regressions in behavior, especially those in the agent. Unfortunately,
it is also notoriously flaky and tricky to understand primarily due to the way the test setup is written, which relies
on using data (such as the smoke test's project YAML and the latest git commit) directly from Evergreen's GitHub repo.
This often makes it difficult to deduce if a smoke test failure is legitimate or due to unrelated flakiness, which
reduces confidence in the smoke test's purpose of reducing risky deploys.

## Considered Options

One option was to make a smaller change that keeps the smoke test setup mostly as-is, but reduce the overall flakiness
by not relying on the interaction between the GitHub API and the smoke test's repotracker. Unfortunately, this doesn't
address some of the other pain points, such as the fact that the test relies on the particular state of data committed
to Evergreen's repo. Any test that uses the repotracker will inherently rely on external state because the repotracker
is designed to integrate directly with GitHub.

## Decision Outcome

It was decided that submitting a manual patch would provide a good way to fix the described issues. Submitting a patch
is a fairly non-flaky since submitting a patch does not rely on services that are external to the test, like GitHub. It
also eliminates the issue of having the smoke test result depend on the current state of the Evergreen repo in GitHub,
since the patch is based on local Evergreen repo state. Lastly, it has an added benefit of testing one of the most
common workflows in Evergreen, which is submitting a patch using the `evergreen patch` command.
