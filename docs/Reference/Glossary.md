# Glossary

- **agent**: An agent is the evergreen client that runs tasks on a host.
- **artifact**: An artifact is a file or directory that is uploaded to Evergreen by a task and can be downloaded from the task page.
- **build**: A build is a set of tasks in a single build variant in a single version.
- **build variant**: A build variant is a distro configured in a project. It contains a distro name and some metadata.
- **command**: A tasks runs one or more commands. There are separate commands to do things like run a program or upload a file to s3.
- **display task**: A display task is a set of tasks grouped together in the UI by the project configuration. Display tasks have no set distro.
- **distro**: A distro is a set of hosts that runs tasks. A static distro is configured with a list of IP addresses, while a dynamic distro scales with demand.
- **expansions**: Expansions are Evergreen- and user-specified variables that can be used in the project configuration file. They can be set by the `expansions.update` command, in the variant, on the project page, and on the distro page.
- **function**: Commands may be grouped together in functions.
- **requester**: The type of patch or version. Valid requester types are documented [here](#requesters).
- **patch build**: A patch build is a version not triggered by a commit to a repository. It either runs tasks on a base commit plus some diff if submitted by the CLI or on a git branch if created by a GitHub pull request.
- **project configuration file**: The project configuration file is a file parsed by Evergreen that defines commands, functions, build variants, and tasks.
- **stepback** When a task fails and the offending commit is unknown, Evergreen will perform linear or bisection stepback depending on the project settings. Linear incrementally runs the same task in previous versions in O(n) tasks while bisection performs binary search in O(logn) tasks.
- **task group**: A task group is a set of tasks which run with project-specified setup and teardown code without cleaning up the task directory before or after each task and without any tasks running in between tasks in the group.
- **task**: The fundamental unit of execution is the task. A task corresponds to a box on the waterfall page.
- **test**: A test is sent to Evergreen in a known format by a command during a task, parsed by Evergreen, and displayed on the task page.
- **user configuration file**: The user configuration file is a file parsed by the Evergreen CLI. It contains the user’s API key and various settings.
- **version**: A version, which corresponds to a vertical slice of tasks on the waterfall, is all tasks for a given commit or patch build.
- **working directory**: The working directory is the temporary directory which Evergreen creates to run a task. It is available as [an expansion](../Project-Configuration/Project-Configuration-Files#default-expansions).

## Requesters

The requester is a field in the version/patch document that indicates how it was created. These immutable values are set upon creation. They can be used to control what tasks [allow requesters](../Project-Configuration/Project-Configuration-Files.md#allowed-requesters) to run the task (note: the names used for `allowed_requesters` are slightly different than their actual names). There are some shorthands to [limit when a task or variant will run](../Project-Configuration/Project-Configuration-Files.md#limiting-when-a-task-or-variant-will-run). They can also used to provide elevated permissions to certain versions/patches. For example, a project can have [admin only](../Project-Configuration/Project-and-Distro-Settings.md#variables) variables that only get provided to versions/patches by admins or from mainline commits. They can also be used with [ec2.assume_role](../Project-Configuration/Project-Commands.md#ec2assume_role) to allow specific requesters to assume a role (e.g.mainline commits) and prevent others from not (e.g. CLI requests.).

Here is a list of valid requester types:

- `github_pull_request`: A pull request from GitHub.
- `github_merge_request`: A merge request from GitHub.
- `git_tag_request`: A tag from GitHub.
- `gitter_request`: A commit on the tracking GitHub branch.
- `trigger_request`: Trigger patches.
- `ad_hoc`: Periodic builds.
- `patch_request`: CLI requests.

Using our [REST API](../API/REST-V2-Usage.mdx) you can find the requester type of a version or patch.
