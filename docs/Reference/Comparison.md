# CI Comparison

## Terminology

| GitHub Actions                                                       | CircleCI          | Evergreen                                                                              |
| -------------------------------------------------------------------- | ----------------- | -------------------------------------------------------------------------------------- |
| None - With GitHub Actions each Workflow lives in its own YAML file. | Pipeline          | Project                                                                                |
| Workflow                                                             | Workflow          | Build Variant (see below for more details)                                             |
| Job                                                                  | Job               | Task                                                                                   |
| Step                                                                 | Step              | Function or Command                                                                    |
| Action                                                               | Orb               | Function or Command                                                                    |
| Matrix                                                               | Matrix Job        | None, but see below                                                                    |
| Variable                                                             | Variable          | Expansion                                                                              |
| PR / Workflow Run                                                    | PR / Pipeline Run | Version (but you will also see people call these a Patch - see below for more details) |

In Evergreen, all of the config for a Project lives in a single YAML file (but you can use [includes](../Project-Configuration/Project-Configuration-Files#include) to break things up). This is like a CircleCI Pipeline. With GitHub Actions (GHA), the project’s config is split into multiple files, one per workflow.

An Evergreen Build Variant is like a single GHA or CircleCI Workflow or with no matrix.

Like GHA and CircleCI do with Jobs, Evergreen runs Tasks in parallel by default. You can declare dependencies between Tasks using the [depends_on](../Project-Configuration/Project-Configuration-Files#task-dependencies) config key.

In Evergreen, a Task consists of one or more Functions and Commands. A Command is a single operation, like running a shell script or uploading files to S3. A Function is a collection of multiple Commands that are run in sequence.

The closest equivalent of a GHA Action or CircleCI Orb is a Function or Command. Evergreen provides a number of [built-in Commands](../Project-Configuration/Project-Commands). It’s also possible to define your own Functions in your project’s configuration. However, Evergreen **does not** have a mechanism for sharing Actions between config files in a way similar to Actions.

Evergreen does not have an equivalent of matrixes from GHA or CircleCI. However, there are a few different ways to generate config that can serve a similar purpose:

- You can use the [generate.tasks](../Project-Configuration/Project-Commands#generatetasks) Command to generate Tasks, Build Variants, etc. dynamically during a run.
- You can write code that uses a shrub library to generate your project’s YAML config and then simply check that in:
  - [https://github.com/evergreen-ci/shrub](https://github.com/evergreen-ci/shrub) - Golang
  - [https://github.com/evergreen-ci/shrub.py](https://github.com/evergreen-ci/shrub.py) - Python

## What’s a “Version” or “Patch”?

You will see the term “Patch” used in the Evergreen docs a lot, and people may use that word in discussions about Evergreen.

Technically a Version is a single CI run, and a Patch is just one kind of Version. But in practice people will use the word “Patch” when they’re talking about something that is a Version, even if that Version is not a Patch.

This is roughly equivalent to the set of GHA Workflows or CircleCI Pipelines run for a PR, but it’s a bit more than that. A Version is a single run of a project’s config in Evergreen. Most projects will be set up to run a Patch for every PR, both when the PR is first created and when you push to its branch after creation. But a Version can also be run in other ways:

- Pushing a tag to a repo with an Evergreen project.
- Scheduled version runs based on a cron schedule. This is configured in the Evergreen project settings.
- Manually using the evergreen [CLI tool](../CLI)’s `evergreen patch` command. When you run this, the set of changes being tested is based on your **local checkout’s commits, not just a PR**. You can even include uncommitted changes in the patch.
- Triggered because of inter-project dependencies.
- And several more.
