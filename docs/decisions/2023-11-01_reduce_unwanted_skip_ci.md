# 2023-11-01 Reduce Issue of Unintentionally Skipping CI in PRs

* status: accepted
* date: 2023-11-01
* authors: Kim Tao

## Context and Problem Statement

Evergreen has a feature to skip PR testing by adding the `[skip ci]` label to the PR title or description.
Unfortunately, detecting that label in the PR description can lead to unintentionally skipping CI in scenarios where it
should not. For example, because the description for Dependabot PRs can contain commit messages from other repos, the
description may coincidentally contain the word `[skip ci]`, thus causing Evergreen to skip testing.

## Considered Options
* Don't read the `[skip ci]` label from the PR description - this would fix the problem, but not every repo would be
  able to put `[skip ci]` in their PR title. For example, if repos have linting rules around the PR title before
  merging, including `[skip ci]` in it may break commit format conventions.
* Don't read the `[skip ci]` label from the PR description, and instead allow it for commit messages - this behavior is
  used by many other CI systems (GitHub Actions, CircleCI, GitLab CI) and eliminates the problem of false skips.
  However, using the commit message would put pressure on the GitHub API rate limit, because Evergreen would have to
  make a GitHub API request for each PR to detect commits containing `[skip ci]`.
* Keep reading the `[skip ci]` label, but only read it if it's near the beginning of the description - this one will
  preserve existing workflows that users have set up of putting `[skip ci]` in the description, but make it less likely
  to unintentionally skip CI. However, it cannot get rid of the ambiguity entirely since the description could include
  any text.

## Decision Outcome
Only reading the first 100 characters from the PR description was chosen as the simplest solution. It should not
significantly change workflows for users already putting `[skip ci]` in their PR descriptions, and it sufficiently
fixes the issue for Dependabot PRs that reference commit messages containing `[skip ci]`. While it still poses some
small chance of a PR unintentionally skipping CI, it mitigates the problem enough that it's much less likely to occur.

## More Information
* [Skip CI in GitLab CI](https://docs.gitlab.com/ee/ci/pipelines/#skip-a-pipeline)
* [Skip CI in GitHub Actions](https://docs.github.com/en/actions/managing-workflow-runs/skipping-workflow-runs)
* [Skip CI in CircleCI](https://circleci.com/docs/skip-build/#skip-jobs)
