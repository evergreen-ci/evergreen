# Project Configuration Files
Project configurations are how you tell Evergreen what to do. They
contain a set of tasks and variants to run those tasks on, and are
stored within the repository they test. Project files are written in a
simple YAML config language.

## Examples

Before reading onward, you should check out some example project files:

1.  [Sample tutorial project file](https://github.com/evergreen-ci/sample.git)
2.  [Evergreen's own project file](https://github.com/evergreen-ci/evergreen/blob/master/self-tests.yml)
3.  [The MongoDB Tools project file](https://github.com/mongodb/mongo-tools/blob/master/common.yml)
4.  [The MongoDB Server project file](https://github.com/mongodb/mongo/blob/master/etc/evergreen.yml)

Though some of them are quite large, the pieces that make them up are
very simple.

## Basic Features

### Tasks

A task is any discrete job you want Evergreen to run, typically a build,
test suite, or deployment of some kind. They are the smallest unit of
parallelization within Evergreen. Each task is made up of a list of
commands/functions. Currently we include commands for interacting with
git, running shell scripts, parsing test results, and manipulating
Amazon s3.

For example, a couple of tasks might look like:

``` yaml
tasks:
- name: compile
  commands:
    - command: git.get_project
      params:
        directory: src
    - func: "compile and upload to s3"
- name: passing_test
  run_on: my_other_distro
  depends_on:
  - name: compile
  commands:
    - func: "download compiled artifacts"
    - func: "run a task that passes"
```

Notice that tasks contain:

1.  A name
2.  A set of dependencies on other tasks. `depends_on` can be defined at
    multiple levels of the YAML. If there are conflicting `depends_on`
    definitions at different levels, the order of priority is defined
    [here](#dependency-override-hierarchy).
3.  A distro or list of distros to run on (documented more under
    ["Build
    Variants"](#build-variants)).
    `run_on` can be defined at multiple levels of the YAML. If there are
    conflicting `run_on` definitions at different levels, the order of priority
    is defined [here](#task-fields-override-hierarchy).
4.  A list of commands and/or functions that tell Evergreen how to run
    it.

Another useful feature is [task tags](#task-and-variant-tags),
which allows grouping tasks to limit whether [those tasks should run on
patches/git
tags/etc.](#limiting-when-a-task-will-run)

#### Commands

Commands are the building blocks of tasks. They do things like clone a
project, download artifacts, and execute arbitrary shell scripts. Each
command has a set of parameters that it can take. A full list of
commands and their parameters is accessible [here](Project-Commands).

#### Functions

Functions are a simple way to group a set of commands together for
reuse. They are defined within the file as

``` yaml
functions:
  "function name":
    - command: "command.name"
    - command: "command.name2"
    ## ...and so on


  ## a real example from Evergreen's tests:
  "start mongod":
      - command: shell.exec
        params:
          background: true
          script: |
            set -o verbose
            cd mongodb
            echo "starting mongod..."
            ./mongod${extension} --dbpath ./db_files &
            echo "waiting for mongod to start up"
      - command: shell.exec
        params:
          script: |
            cd mongodb
            ./mongo${extension} --nodb --eval 'assert.soon(function(x){try{var d = new Mongo("localhost:27017"); return true}catch(e){return false}}, "timed out connecting")'
            echo "mongod is up."
```

and they are referenced within a task definition by

``` yaml
- name: taskName
  commands:
  - func: "run tests"
  - func: "example with multiple args"


  - func: "run tests" ## real example from the MongoDB server
    vars:
      resmoke_args: --help
      run_multiple_jobs: false
  - func: "example with multiple args"
    vars:
      resmoke_args: >- ## syntax needed to allow multiple arguments.
        --hello=world
        --its=me
      
```

Notice that the function reference can define a set of `vars` which are
treated as expansions within the configuration of the commands in the
function.

A function cannot be called within another function. However, it is still
possible to reuse commands using YAML aliases and anchors. For example:

```yaml
variables:
  - &download_something
    command: shell.exec
    params:
      script: |
          curl -LO https://example.com/something
  - &download_something_else
    command: shell.exec
    params:
      script: |
        curl -LO https://example.com/something-else

tasks:
  - name: my-first-task
    commands:
      - *download_something
  - name: my-second-task
    commands:
      - *download_something
      - *download_something_else
```

### Tests

As you've read above, a task is a single unit of work in Evergreen. A
task may contain any number of logical tests. A test has a name, status,
time taken, and associated logs, and each test displays in a table in
the Evergreen UI. If your task runs tests, note that Evergreen does not
automatically parse the results of the tests to display in the UI - you
have to do a bit more configuration to tell it how to parse/attach the
results.

In order to tell Evergreen how to handle tests, you'll need to add a
command at the end of your task which attaches test results. The Project
Commands section of this wiki has a list of the commands available, each
supporting a different format, with specific ones to accommodate common
formats. For example, if your task runs some golang tests, adding the
following command at the end of your task will parse and attach those
test results:

``` yaml
- command: gotest.parse_files
  type: system
  params:
    files:
      - "gopath/src/github.com/evergreen-ci/evergreen/bin/output.*"
```

If you specify one of these commands and there are no results to attach,
the command will no-op by default. If you'd like the task to instead
fail in this scenario, you can specify `must_have_test_results: true` in
your task

### Build Variants

Build variants are a set of tasks run on a given platform. Each build
variant has control over which tasks it runs, what distro it runs on,
and what expansions it uses.

``` yaml
buildvariants:
- name: osx-108
  cron: 0 * * * *
  display_name: OSX
  run_on:
  - localtestdistro
  expansions:
    test_flags: "blah blah"
  tasks:
  - name: compile
  - name: passing_test
    cron: @daily // overrides build variant cron
  - name: failing_test
  - name: timeout_test
- name: ubuntu
  display_name: Ubuntu
  batchtime: 60
  patch_only: true
  run_on:
  - ubuntu1404-test
  expansions:
    test_flags: "blah blah"
  tasks:
  - name: compile
  - name: passing_test
    depends_on: 
    - name: compile
    - name: passing_test
      variant: osx-108
    exec_timeout_secs: 20
    priority: 10
    batchtime: 20 // overrides build variant batchtime of 60
  - name: failing_test
    activate: false
    tags: ["special"]
  - name: timeout_test
    patchable: false
  - name: git_tag_release
    git_tag_only: true
  - name: inline_task_group_1
    task_group:
      <<: *example_task_group
      tasks:
      - example_task_1
  - name: inline_task_group_2
    task_group:
      share_processes: true
      max_hosts: 3
      teardown_group:
      - command: attach.results
      tasks:
      - example_task_2
      - example_task_3
```

Fields:

-   `name`: an identification string for the variant
-   `display_name`: how the variant is displayed in the Evergreen UI
-   `run_on`: a list of acceptable distros to run tasks for that variant
    The first distro in the list is the primary distro. The others
    are secondary distros. Each distro has a primary queue, a queue of
    all tasks that have specified it as their primary distro; and a
    secondary queue, a queue of tasks that have specified it as a
    secondary distro. If the primary queue is not empty, the distro will
    process that queue and ignore the secondary queue. If the primary
    queue is empty, the distro will process the secondary queue. If both
    queues are empty, idle hosts will eventually be terminated.
    `run_on` can be defined at multiple levels of the YAML. If there are
    conflicting `run_on` definitions at different levels, the order of priority
    is defined [here](#task-fields-override-hierarchy).
-   `depends_on`: a list of dependencies on other tasks. All tasks in the build
    variant will depend on these tasks. `depends_on` can be defined under a
    task, under an entire build variant, or for a specific task under a specific
    build variant. If there are conflicting `depends_on` definitions at
    different levels, the order of priority is defined
    [here](#dependency-override-hierarchy).
-   `expansions`: a set of key-value expansion pairs
-   `tasks`: a list of tasks to run, referenced either by task name or by tags.
    Tasks listed here can also include other task-level fields, such as
    `batchtime`, `cron`, `activate`, `depends_on`, and `run_on`. We can also
    [define when a task will run](#limiting-when-a-task-will-run). If there are
    conflicting settings definitions at different levels, the order of priority
    is defined [here](#task-fields-override-hierarchy).
-   `activate`: by default, we'll activate if the whole version is
    being activated or if `batchtime` specifies it should be activated. If
    we instead want to activate immediately, then set activate to true.
    If this should only activate when manually scheduled or by
    stepback/dependencies, set activate to false.
-   `batchtime`: interval of time in minutes that Evergreen should wait
    before activating this variant. The default is set on the project
    settings page. This can also be set for individual tasks. Only applies to
    tasks from mainline commits.
-   `cron`: define with [cron syntax](https://crontab.guru/) (i.e. Min \| Hour \| DayOfMonth \|
    Month \| DayOfWeekOptional) when (in UTC) a task or variant should be activated
    (cannot be combined with batchtime). This also accepts descriptors
    such as `@daily` (reference
    [cron](https://godoc.org/github.com/robfig/cron) for more example),
    but does not accept intervals. (i.e.
    `@every <duration>`). Only applies to tasks from mainline commits.
-   `task_group`: a [task
    group](#task-groups)
    may be defined directly inline or using YAML aliases on a build
    variant task. This is an alternative to referencing a task group
    defined in `task_groups` under the tasks of a given build variant.
-   `tags`: optional list of tags to group the build variant for alias definitions (explained [here](#task-and-variant-tags))
-   Build variants support [all options that limit when a task will run](#limiting-when-a-task-will-run). If set for the
    build variant, it will apply to all tasks under the build variant.

Additionally, an item in the `tasks` list can be of the form

``` yaml
tasks:
- name: compile
  run_on: 
  - ubuntu1404-build
```

allowing tasks within a build variant to be run on different distros.
This is useful for optimizing tasks like compilations, that can benefit
from larger, more powerful machines.

### Version Controlled Project Settings
Project configurations can version control some select project settings (e.g. aliases, plugins) directly within the yaml
rather than on the project page UI, for better accessibility and maintainability. Read more
[here](Project-and-Distro-Settings.md#version-control).

## Advanced Features

These features will help you do more complicated workloads with
Evergreen.

### Include

Configuration files listed in `include` will be merged with the main
project configuration file. All top-level configuration files can define
includes. This will accept a list of filenames and module names. If the
include isn't given, we will only use the main project configuration
file.

Note: included files do not support [version-controlled project settings configuration](Project-and-Distro-Settings.md#version-control)

``` yaml
include: 
   - filename: other.yml
   - filename: small.yml ## path to file inside the module's repo
     module: module_name
```

Warning: YAML anchors currently not supported

#### Merging Rules

We will maintain the following merge rules:

-   Lists where order doesn't matter can be defined across different
    yamls, but there cannot be duplicate keys within the merged lists
    (i.e. "naming conflicts"); this maintains our existing validation.
    Examples: tasks and task group names, parameter keys, module names,
    function names.
-   Unordered lists that don't need to consider naming conflicts.
    Examples: ignore and loggers.
-   Lists where order does matter cannot be defined for more than one
    yaml. Examples: pre, post, timeout, early termination.
-   Non-list values cannot be defined for more than one yaml. Examples:
    stepback, batchtime, pre error fails task, OOM tracker, display
    name, command type, and exec timeout.
-   It is illegal to define a build variant multiple times except to add
    additional tasks to it. That is, a build variant should only be
    defined once, but other files can include this build variant's
    definition in order to add more tasks to it. This is also how we
    merge generated variants.
-   Matrix definitions or axes cannot be defined for more than one yaml.

#### Validating changes to config files

When editing yaml project files, you can verify that the file will work
correctly after committing by checking it with the "validate" command.
To validate local changes within modules, use the `local_modules` flag
to list out module name and path pairs.

Note: Must include a local path for includes that use a module.

``` evergreen validate <path-to-yaml-project-file> -lm <module-name>=<path-to-yaml> ```

The validation step will check for:

-   valid yaml syntax
-   correct names for all commands used in the file
-   logical errors, like duplicated variant or task names
-   invalid sets of parameters to commands
-   warning conditions such as referencing a distro pool that does
    not exist
-   merging errors from include files

### Modules

For patches that run tests based off of changes across multiple
projects, the modules field may be defined to specify other git projects
with configurations specifying the way that changes across them are
applied within the patch at runtime. If configured correctly, the left
hand side of the Spruce UI under "Version Manifest" will contain
details on how the modules were parsed from YAML and which git revisions
are being used.

For manual patches and GitHub PRs, by default, the git revisions in the
version manifest will be inherited from its base version. You can change
the git revision for modules by setting a module manually with 
[evergreen set-module](../CLI.md#operating-on-existing-patches) or
by specifying the `auto_update` option (as described below) to use the
latest revision available for a module.

``` yaml
modules:
- name: evergreen
  repo: https://github.com/deafgoat/mci_test.git
  prefix: src/mongo/db/modules
  branch: master
- name: sandbox
  repo: https://github.com/deafgoat/sandbox.git
  branch: main
  ref: <some_hash>
- name: mci
  repo: https://github.com/deafgoat/mci.git
  branch: main
  auto_update: true
```

Fields:

-   `name`: alias to refer to the module
-   `branch`: the branch of the module to use in the project
-   `repo`: the git repository of the module
-   `prefix`: the path prefix to use for the module
-   `ref`: the git commit hash to use for the module in the project (if
    not specified, defaults to the latest revision that existed at the
    time of the Evergreen version creation)
-   `auto_update`: if true, the latest revision for the module will be
    dynamically retrieved for each Github PR and CLI patch submission

### Pre and Post

All projects can have a `pre` and `post` field which define a list of commands
to run at the start and end of every task that isn't in a task group. For task
groups, `setup_task` and `teardown_task` will run instead of `pre` and `post`
(see [task groups](#task-groups) for more information). These are incredibly
useful as a place for results commands or for task setup and cleanup.

``` yaml
pre_error_fails_task: true
pre_timeout_secs: 1800 # 30 minutes
pre:
  - command: shell.exec
    params:
      working_dir: src
      script: |
        ## do setup

post_error_fails_task: true
post_timeout_secs: 1800 # 30 minutes
post:
  - command: attach.results
    params:
      file_location: src/report.json
```

Parameters:

- `pre`: commands to run prior to the task. Note that `pre` does not run for
  task group tasks.
- `pre_error_fails_task`: if true, task will fail if a command in `pre` fails.
  Defaults to false.
- `pre_timeout_secs`: set a timeout for `pre`. Defaults to 2 hours. Hitting this
  timeout will stop the `pre` commands but will not cause the task to fail
  unless `pre_error_fails_task` is true.
- `post`: commands to run after the task. Note that `post` does not run for task
  group tasks.
- `post_error_fails_task`: if true, task will fail if a command in `post` fails.
- `post_timeout_secs`: set a timeout for `post`. Defaults to 30 minutes. Hitting
  this timeout will stop the `post` commands but will not cause the task to fail
  unless `post_error_fails_task` is true.

### Timeout Handler

Project configs offer a hook for running command when a task times out, allowing
you to automatically run a debug script when something is stuck.

``` yaml
callback_timeout_secs: 60
timeout:
  - command: shell.exec
    params:
      working_dir: src
      script: |
        echo "Calling the hang analyzer..."
        python buildscripts/hang_analyzer.py
```

Parameters:

- `timeout`: commands to run when the task hits a timeout. The timeout commands
  will only run if the timeout occurs in `pre`, `setup_group`, `setup_task`, or
  the task commands. Furthermore, for `pre`, `setup_group`, and `setup_task`,
  because they do not fail the task by default, they must be explicitly set to
  fail for the timeout to trigger. For example, if a command in `pre`
  hits the default 2 hour timeout for `pre` but `pre_error_fails_task` is not
  set to true, then the timeout block will not trigger.
- `callback_timeout_secs`: set a timeout for the `timeout` block. Defaults to
  15 minutes.

**Exec timeout: exec_timeout_secs**
You can customize the points at which the "timeout" conditions are
triggered. To cause a task to stop (and fail) if it doesn't complete
within an allotted time, set the key `exec_timeout_secs` on the project
or task to the maximum allowed length of execution time. Exec timeout only
applies to commands that run in `pre`, `setup_group`, `setup_task`, and the main
task commands; it does not apply to the `post`, `teardown_task`, and
`teardown_group` blocks. This timeout defaults to 6 hours. `exec_timeout_secs`
can only be set on the project or on a task. It cannot be set on functions.

You can also set `exec_timeout_secs` using [timeout.update](Project-Commands.md#timeoutupdate).

**Idle timeout: timeout_secs**
You may also force a specific command to trigger a failure if it does not appear
to generate any output on `stdout`/`stderr` for more than a certain threshold,
using the `timeout_secs` setting on the command. As long as the command produces
output to `stdout`/`stderr`, it will be allowed to continue, but if it does not
write any output for longer than `timeout_secs` then the command will time out.
If this timeout is hit, the task will stop (and fail). Idle timeout only applies
to commands that run in `pre`, `setup_group`, `setup_task` and the main task
commands; it does not apply to the `post`, `teardown_task`, and `teardown_group`
blocks. This timeout defaults to 2 hours.

You can also overwrite the default `timeout_secs` for all later commands using
[timeout.update](Project-Commands.md#timeoutupdate).

Example:

``` yaml
exec_timeout_secs: 60 ## automatically fail any task if it takes longer than a minute to finish.
buildvariants:
- name: osx-108
  display_name: OSX
  run_on:
  - localtestdistro
  tasks:
  - name: compile

tasks:
  name: compile
  commands:
    - command: shell.exec
      timeout_secs: 10 ## force this command to fail if it stays "idle" for 10 seconds or more
      params:
        script: |
          sleep 1000
```

```yaml
exec_timeout_secs: 60
```

### Limiting When a Task Will Run

To limit the conditions when a task will run, the following settings can be
added to a task definition, to a build variant definition, or to a specific task
listed under a build variant (so that it will only affect that variant's task).

To cause a task to only run in commit builds, set `patchable: false`.

To cause a task to only run in patches, set `patch_only: true`.

To cause a task to only run in versions NOT triggered from git tags, set
`allow_for_git_tag: false`.

To cause a task to only run in versions triggered from git tags, set
`git_tag_only: true`.

To cause a task to not run at all, set `disable: true`.

-   This behaves similarly to commenting out the task but will not
    trigger any validation errors.
-   If a task is disabled and is depended on by another task, the
    dependent task will simply exclude the disabled task from its
    dependencies.

Can also set activate, batchtime or cron on tasks or build variants, detailed
[here](#build-variants).

If there are conflicting settings defined at different levels, the order of
priority is defined [here](#task-fields-override-hierarchy).

#### Allowed Requesters

If the above settings do not provide the particular combination of conditions
when you want a task to run, you can specify `allowed_requesters` to enumerate
the list of conditions when a task is allowed to run. For example, if you wish
for a task to only run for manual patches and git tag versions, you can specify
it like this:

```yaml
tasks:
- name: only-run-for-manual-patches-and-git-tag-versions
  allowed_requesters: ["patch", "github_tag"]
```

The valid requester values are:
- `patch`: manual patches.
- `github_pr`: GitHub PR patches.
- `github_tag`: git tag versions.
- `commit`: mainline commits.
- `trigger`: downstream trigger versions.
- `ad_hoc`: periodic build versions.
- `commit_queue`: Evergreen's commit queue.
- `github_merge_queue`: GitHub's merge queue.

By default, if no `allowed_requesters` are explicitly specified, then a task can
run for any requester. If you specify an empty `allowed_requesters` list (i.e.
`allowed_requesters: []`), this is also treated the same as the default.
`allowed_requesters` is not compatible with `patchable`, `patch_only`,
`allow_for_git_tag`, or `git_tag_only` (if combined, the
`allowed_requesters` will always take higher precedence).

If `allowed_requesters` is specified and a conflicting project setting is also
specified, `allowed_requesters` will take higher precedence. For example, if the
project settings configure a [GitHub PR patch
definition](Project-and-Distro-Settings.md#github-pull-request-testing) to run
tasks A and B but task A has `allowed_requesters: ["commit"]`, then GitHub PR
patches will only run task B.

### Expansions

Expansions are variables within your config file. They take the form
`${key_name}` within your project, and are defined on a project-wide
level on the project configuration page or on a build variant level
within the project. They can be used **as inputs to commands**,
including shell scripts.

Expansions cannot be used recursively. In other words, you can't define an
expansion whose value uses another expansion.

``` yaml
command: s3.get
   params:
     aws_key: ${aws_key}
     aws_secret: ${aws_secret}
```

Expansions can also take default arguments, in the form of
`${key_name|default}`.

``` yaml
command: shell.exec
   params:
     working_dir: src
     script: |
       if [ ${has_pyyaml_installed|false} = false ]; then
       ...
```

Likewise, the default argument of an expansion can be an expansion
itself. Prepending an asterisk to the default value will lookup the
expansion value of the default value, rather than the hard coded string.

``` yaml
command: shell.exec
   params:
    script: |
      VERSION=${use_version|*use_version_default} ./foo.sh
```

If an expansion is used in your project file, but is unset, it will be
replaced with its default value. If there is no default value, the empty
string will be used. If the default value is prepended with an asterisk
and that expansion also does not exist, the empty string will also be
used.


Expansions are also case-sensitive.

``` yaml
command: shell.exec
   params:
      working_dir: src
     script: |
       echo ${HelloWorld}
```


#### Usage

Expansions can be used as input to any yaml command field that expects a
string. The flip-side of this is that expansions are not currently
supported for fields that expect boolean or integer inputs, including
`timeout_secs`.

If you find **a command** that does not accept string expansions, please
file a ticket or issues. That's a bug.

#### Default Expansions

Every task has some expansions available by default:

-   `${is_patch}` is "true" if the running task is in a patch build and
    undefined if it is not.
-   `${is_stepback}` is "true" if the running task was stepped back.
-   `${author}` is the patch author's username for patch tasks or the
    git commit author for git tasks
-   `${author_email}` is the patch or the git commit authors email
-   `${task_id}` is the task's unique id
-   `${task_name}` is the name of the task
-   `${execution}` is the execution number of the task (how many times
    it has been reset)
-   `${build_id}` is the id of the build the task belongs to
-   `${build_variant}` is the name of the build variant the task belongs
    to
-   `${version_id}` is the id of the task's version
-   `${workdir}` is the task's working directory
-   `${revision}` is the commit hash of the base commit of a patch or of
    the commit for a mainline build
-   `${github_commit}` is the commit hash of the commit that triggered
    the patch run
-   `${project}` is the project identifier the task belongs to
-   `${project_identifier}` is the project identifier the task belongs
    to // we will be deprecating this, please use `${project}`
-   `${project_id}` is the project ID the task belongs to (note that for
    later projects, this is the unique hash, whereas for earlier
    projects this is the same as `${project}`. If you aren't sure which
    you are, you can check using this API route:
    `https://evergreen.mongodb.com/rest/v2/<project_id>`)
-   `${branch_name}` is the name of the branch tracked by the
    project
-   `${distro_id}` is name of the distro the task is running on
-   `${created_at}` is the time the version was created
-   `${revision_order_id}` is Evergreen's internal revision order
    number, which increments on each commit, and includes the patch
    author name in patches
-   `${github_pr_number}` is the Github PR number associated with PR
    patches and PR triggered commit queue items
-   `${github_org}` is the GitHub organization for the repo in which
    a PR or PR triggered commit queue item appears
-   `${github_repo}` is the GitHub repo in which a PR or PR triggered
    commit queue item appears
-   `${github_author}` is the GitHub username of the creator of a PR
    or PR triggered commit queue item
-   `${triggered_by_git_tag}` is the name of the tag that triggered this
    version, if applicable
-   `${is_commit_queue}` is the string "true" if this is a commit
    queue task
-   `${commit_message}` is the commit message if this is a commit queue
    task
-   `${requester}` is what triggered the task: `patch`, `github_pr`,
    `github_tag`, `commit`, `trigger`, `commit_queue`, or `ad_hoc`
-   `${otel_collector_endpoint}` is the gRPC endpoint for Evergreen's
    OTel collector. Tasks can send traces to this endpoint.
-   `${otel_trace_id}` is the OTel trace ID this task is running under.
    Include the trace ID in your task's spans so they'll be hooked
    in under the task's trace.
    See [Hooking tests into command spans](Task_Traces.md#hooking-tests-into-command-spans) for more information.
-   `${otel_parent_id}` is the OTel span ID of the current command.
    Include this ID in your test's root spans so it'll be hooked
    in under the command's trace.
    See [Hooking tests into command spans](Task_Traces.md#hooking-tests-into-command-spans) for more information.

The following expansions are available if a task was triggered by an
inter-project dependency:

-   `${trigger_event_identifier}` is the ID of the task or build that
    initiated this trigger
-   `${trigger_event_type}` will be "task" or "build," identifying
    what type of ID `${trigger_event_identifier}` is
-   `${trigger_status}` is the task or build status of whatever
    initiated this trigger
-   `${trigger_revision}` is the githash of whatever commit initiated
    this trigger
-   `${trigger_repo_owner}` is Github repo owner for the project that
    initiated this trigger
-   `${trigger_repo_name}` is Github repo name for the project that
    initiated this trigger
-   `${trigger_branch}` is git branch for the project that initiated
    this trigger

The following expansions are available if a task has modules:

`<module_name>` represents the name defined in the project yaml for a
given module

-   `${<module_name>_rev}` is the revision of the evergreen module
    associated with this task
-   `${<module_name>_branch}` is the branch of the evergreen module
    associated with this task
-   `${<module_name>_repo}` is the Github repo for the evergreen module
    associated with this task
-   `${<module_name>_owner}` is the Github repo owner for the evergreen
    module associated with this task

### Task and Variant Tags

Most projects have some implicit grouping at every layer. Some tests are
integration tests, others unit tests; features can be related even if
their tests are stored in different places. Evergreen provides an
interface for manipulating tasks using this kind of reasoning through
*tag selectors.*

Tags are defined as an array as part of a task or variant definition. Tags should
be self-explanatory and human-readable. Variant tags are used for grouping alias definitions.

``` yaml
tasks:
  ## this task is an integration test of backend systems; it requires a running database
- name: db
  tags: ["integration", "backend", "db_required"]
  commands:
    - func: "do test"

  ## this task is an integration test of frontend systems using javascript
- name: web_admin_page
  tags: ["integration", "frontend", "js"]
  commands:
    - func: "do test"

  ## this task is an integration test of frontend systems using javascript
- name: web_user_settings
  tags: ["integration", "frontend", "js"]
  commands:
    - func: "do test"

buildvariants:
  ## this variant has a tag to be used for alias definitions
- name: my_variant
  tags: ["pr_testing"]
```

Tags can be referenced in variant definitions to quickly include groups
of tasks.

``` yaml
buildvariants:
  ## this project only does browser tests on OSX
- name: osx
    display_name: OSX
    run_on:
    - osx-distro
    tasks:
    - name: ".frontend"
      run_on:
        - osx-distro-test
    - name: ".js"

  ## this variant does everything
- name: ubuntu
    display_name: Ubuntu
    run_on:
    - ubuntu-1440
    tasks:
    - name: "*"

  ## this experimental variant runs on a tiny computer and can't use a database or run browser tests
- name: ubuntu_pi
    display_name: Ubuntu Raspberry Pi
    run_on:
    - ubuntu-1440
    tasks:
    - name: "!.db_required !.frontend"
```

Tags can also be referenced in dependency definitions.

``` yaml
tasks:
  ## this project only does long-running performance tests on builds with passing unit tests
- name: performance
  depends_on:
  - ".unit"
  commands:
    - func: "do test"

  ## this task runs once performance and integration tests finish, regardless of the result
- name: publish_binaries
  depends_on:
  - name: performance
    status: *
  - name: ".integration"
    status: *
```

Tag selectors are used to define complex select groups of tasks based on
user-defined tags. Selection syntax is currently defined as a
whitespace-delimited set of criteria, where each criterion is a
different name or tag with optional modifiers. Formally, we define the
syntax as:

    Selector := [whitespace-delimited list of Criterion]
    Criterion :=  (optional ! rune)(optional . rune)<Name> or "*" // where "!" specifies a negation of the criteria and "." specifies a tag as opposed to a name
    Name := <any string> // excluding whitespace, '.', and '!'

Selectors return all items that satisfy all of the criteria. That is,
they return the *set intersection* of each individual criterion.

For Example:

-   `red` would return the item named "red"
-   `.primary` would return all items with the tag "primary"
-   `!.primary` would return all items that are NOT tagged "primary"
-   `.cool !blue` would return all items that are tagged "cool" and
    NOT named "blue"
-   `.cool !.primary` would return all items that are tagged "cool" and
    NOT tagged "primary"
-   `*` would return all items

### Display Tasks

Evergreen provides a way of grouping tasks into a single logical unit
called a display task. These units are displayed in the UI as a single
task. Only display tasks, not their execution tasks, are available to
schedule patches against. Individual tasks in a display task are visible
on the task page. Display task pages do not include any logs, though
execution tasks' test results render on the display task's page. Users
can restart the entire display task or only its failed execution tasks, but not individual execution
tasks.

To create a display task, list its name and its execution tasks in a
`display_tasks` array in the variant definition. The execution tasks
must be present in the `tasks` array.

``` yaml
- name: lint-variant
  display_name: Lint
  run_on:
    - archlinux
  tasks:
    - name: ".lint"
  display_tasks:
    - name: lint
      execution_tasks:
      - ".lint"
```

### Stepback

Stepback is set to true if you want to stepback and test earlier commits
in the case of a failing task. This can be set or unset at the
top-level, at the build variant level, and for individual tasks (in the task definition or for the
task within a specific build variant).

### Out of memory (OOM) Tracker

This is set to true at the top level if you'd like to enable the OOM Tracker for your project.

If there is an OOM kill, immediately before the post-task starts, there will be
a task log message saying whether it found any OOM killed processes, with their
PIDs. A message with PIDs will also be displayed in the metadata panel in the
UI.

### Matrix Variant Definition

The matrix syntax is deprecated in favor of the
[generate.tasks](Project-Commands.md#generatetasks)
command. **Evergreen is unlikely to do further development on matrix
variant definitions.** The documentation is here for completeness, but
please do not add new matrix variant definitions. It is typically
incorrect to test a matrix, as a subset of the tasks is usually
sufficient, e.g., all tasks one one variant, and a small subset of tasks
on other variants.

Evergreen provides a format for defining a wide range of variants based
on a combination of matrix axes. This is similar to configuration
definitions in systems like Jenkins and Travis.

Take, for example, a case where a program may want to test on
combinations of operating system, python version, and compile flags. We
could build a matrix like:

``` yaml
## This is a simple matrix definition for a fake MongoDB python driver, "Mongython".
## We have several test suites (not defined in this example) we would like to run
## on combinations of operating system, python interpreter, and the inclusion of
## python C extensions.

axes:
  ## we test our fake python driver on Linux and Windows
- id: os
  display_name: "OS"
  values:

  - id: linux
    display_name: "Linux"
    run_on: centos6-perf

  - id: windows
    display_name: "Windows 95"
    run_on: windows95-test

  ## we run our tests against python 2.6 and 3.0, along with
  ## external implementations pypy and jython
- id: python
  display_name: "Python Implementation"
  values:

  - id: "python26"
    display_name: "2.6"
    variables:
      ## this variable will be used to tell the tasks what executable to run
      pybin: "/path/to/26"

  - id: "python3"
    display_name: "3.0"
    variables:
      pybin: "/path/to/3"

  - id: "pypy"
    display_name: "PyPy"
    variables:
      pybin: "/path/to/pypy"

  - id: "jython"
    display_name: "Jython"
    variables:
      pybin: "/path/to/jython"

  ## we must test our code both with and without C libraries
- id: c-extensions
  display_name: "C Extensions"
  values:

  - id: "with-c"
    display_name: "With C Extensions"
    variables:
      ## this variable tells a test whether or not to link against C code
      use_c: true

  - id: "without-c"
    display_name: "Without C Extensions"
    variables:
      use_c: false

buildvariants:
- matrix_name: "tests"
  matrix_spec: {os: "*", python: "*", c-extensions: "*"}
  exclude_spec:
    ## pypy and jython do not support C extensions, so we disable those variants
    python: ["pypy", "jython"]
    c-extensions: with-c
  display_name: "${os} ${python} ${c-extensions}"
  tasks : "*"
  rules:
  ## let's say we have an LDAP auth task that requires a C library to work on Windows,
  ## here we can remove that task for all windows variants without c extensions
  - if:
      os: windows
      c-extensions: false
      python: "*"
    then:
      remove_task: ["ldap_auth"]
```

In the above example, notice how we define a set of axes and then
combine them in a matrix definition. The equivalent set of matrix
definitions would be much longer and harder to maintain if built out
individually.

#### Axis Definitions

Axes and axis values are the building block of a matrix. Conceptually,
you can imagine an axis to be a variable, and its axis values are
different values for that variable. For example the YAML above includes
an axis called "python_version", and its values enumerate different
python interpreters to use.

Axes are defined in their own root section of a project file:

``` yaml
axes:
- id: "axis_1"               ## unique identifier
  display_name: "Axis 1"     ## OPTIONAL human-readable identifier
  values:
  - id: "v1"               ## unique identifier
    display_name: "Value 1"  ## OPTIONAL string for substitution into a variant display name (more on that later)
    variables:               ## OPTIONAL set of key-value pairs to update expansions
      key1: "1"
      key2: "two"
    run_on: "ec2_large"      ## OPTIONAL string or array of strings defining which distro(s) to use
    tags: ["1", "taggy"]     ## OPTIONAL string or array of strings to tag the axis value
    batchtime: 3600          ## OPTIONAL how many minutes to wait before scheduling new tasks of this variant
    modules: "enterprise"    ## OPTIONAL string or array of strings for modules to include in the variant
    stepback: false          ## OPTIONAL whether to run previous commits to pinpoint a failure's origin (off by default)
  - id: "v2"
    ## and so on...
```

During evaluation, axes are evaluated from *top to bottom*, so earlier
axis values can have their fields overwritten by values in later-defined
axes. There are some important things to note here:

*ONE:* The `variables` and `tags` fields are *not* overwritten by later
values. Instead, when a later axis value adds new tags or variables,
those values are *merged* into the previous defintions. If axis 1
defines tag "windows" and axis 2 defines tag "64-bit", the resulting
variant would have both "windows" and "64-bit" as tags.

*TWO:* Axis values can reference variables defined in previous axes. Say
we have four distros: windows_small, windows_big, linux_small,
linux_big. We could define axes to create variants the utilize those
distros by doing:

``` yaml
axes:
-id: size
 values:
 - id: small
   variables:
     distro_size: small
 - id: big
   variables:
     distro_size: big
- id: os
  values:
  - id: win
    run_on: "windows_${distro_size}"

  - id: linux
    run_on: "linux_${distro_size}"
    variables:
```

Where the run_on fields will be evaluated when the matrix is parsed.

#### Matrix Variants

You glue those axis values together inside a variant matrix definition.
In the example python driver configuration, we defined a matrix called
"test" that combined all of our axes and excluded some combinations we
wanted to avoid testing. Formally, a matrix is defined like:

``` yaml
buildvariants:
- matrix_name: "matrix_1"            ## unique identifier
  matrix_spec:                       ## a set of axis ids and axis value selectors to combine into a matrix
    axis_1: value
    axis_2:
    - v1
    - v2
    axis_3: .tagged_values
  exclude_spec:                      ## OPTIONAL one or an array of "matrix_spec" selectors for excluding combinations
    axis_2: v2
    axis_3: ["v5", "v6"]
  display_name: "${os} and ${size}"  ## string expanded with axis display_names (see below)
  run_on: "ec2_large"                ## OPTIONAL string or array of strings defining which distro(s) to use
  tags: ["1", "taggy"]               ## OPTIONAL string or array of strings to tag the resulting variants
  batchtime: 3600                    ## OPTIONAL how many minutes to wait before scheduling new tasks
  modules: "enterprise"              ## OPTIONAL string or array of strings for modules to include in the variants
  stepback: false                    ## OPTIONAL whether to run previous commits to pinpoint a failure's origin (off by default)
  tasks: ["t1", "t2"]                ## task selector or array of selectors defining which tasks to run, same as any variant definition
  rules: []                          ## OPTIONAL special cases to handle for certain axis value combinations (see below)
```

Note that fields like "modules" and "stepback" that can be defined by
axis values will be overwritten by their axis value settings.

The `matrix_spec` and `exclude_spec` fields both take maps of
`axis: axis_values` as their inputs. These axis values are combined to
generate variants. The format itself is relatively flexible, and each
axis can be defined as either `axis_id: single_axis_value`,
`axis_id: ["value1", "value2"]`, or `axis_id: ".tag .selector"`. That
is, each axis can define a single value, array of values, or axis value
tag selectors to show which values to contribute to the generated
variants. The most common selector, however, will usually be
`axis_id: "*"`, which selects all values for an axis.

Keep in mind that YAML is a superset of JSON, so

``` yaml
matrix_spec: {"a1":"*", "a2":["v1", "v2"]}
```

is the same as

``` yaml
matrix_spec:
  a1: "*"
  a2:
  - v1
  - v2
```

Also keep in mind that the exclude_spec field can optionally take
multiple matrix specs, e.g.

``` yaml
exclude_spec:
- a1: v1
  a2: v1
- a1: v3
  a4: .tagged_vals
```

#### The Rules Field

Sometimes certain combinations of axis values may require special
casing. The matrix syntax handles this using the `rules` field.

Rules is a list of simple if-then clauses that allow you to change
variant settings, add tasks, or remove them. For example, in the python
driver YAML from earlier:

``` yaml
rules:
- if:
    os: windows
    c-extensions: false
    python: "*"
  then:
    remove_task: ["ldap_auth"]
```

tells the matrix parser to exclude the "ldap_auth" test from windows
variants that build without C extensions.

The `if` field of a rule takes a matrix selector, similar to the matrix
`exclude_spec` field. Any matrix variants that are contained by the
selector will have the rules applied. In the example above, the variant
`{"os":"windows", "c-extensions": "false", "python": "2.6"}` will match
the rule, but `{"os":"linux", "c-extensions": "false", "python": "2.6"}`
will not, since its `os` is not "windows."

The `then` field describes what to do with matching variants. It takes
the form

``` yaml
then:
  add_tasks:                            ## OPTIONAL a single task selector or list of task selectors
  - task_id
  - .tag
  - name: full_variant_task
    depends_on: etc
  remove_tasks:                         ## OPTIONAL a single task selector or list of task selectors
  - task_id
  - .tag
  set:                                  ## OPTIONAL any axis_value fields (except for id and display_name)
    tags: tagname
    run_on: special_snowflake_distro
```

#### Referencing Matrix Variants

Because generated matrix variant ids are not meant to be easily
readable, the normal way of referencing them (e.g. in a `depends_on`
field) does not work. Fortunately there are other ways to reference
matrix variants using variant selectors.

The most succinct way is with tag selectors. If an axis value defines a
`tags` field, then you can reference the resulting variants by
referencing the tag.

``` yaml
variant: ".tagname"
```

More complicated selector strings are possible as well

``` yaml
variant: ".windows !.debug !special_variant"
```

You can also reference matrix variants with matrix definitions, just
like `matrix_spec`. A single set of axis/axis value pairs will select
one variant

``` yaml
variant:
  os: windows
  size: large
```

Multiple axis values will select multiple variants

``` yaml
variant:
  os: ".unix" ## tag selector
  size: ["large", "small"]
```

Note that the `rules` `if` field can only take these matrix-spec-style
selectors, not tags, since rules can modify a variant's tags.

#### Matrix Tips and Tricks

For more examples of matrix project files, check out \* [Test Matrix
1](https://github.com/evergreen-ci/evergreen/blob/master/model/testdata/matrix_simple.yml)
\* [Test Matrix
2](https://github.com/evergreen-ci/evergreen/blob/master/model/testdata/matrix_python.yml)
\* [Test Matrix
3](https://github.com/evergreen-ci/evergreen/blob/master/model/testdata/matrix_deps.yml)

When developing a matrix project file, the Evergreen command line tool
offers an `evaluate` command capable of expanding matrix definitions
into their resulting variants client-side. Run
`evergreen evaluate --variant my_project_file.yml` to print out an
evaluated version of the project.

### Task Groups

Task groups pin groups of tasks to sets of hosts. When tasks run in a
task group, the task directory is not removed between tasks, which
allows tasks in the same task group to share state, which can be useful
for purposes such as reducing the amount of time running expensive
setup and teardown for every single task.

A task group contains arguments to set up and tear down both the entire
group and each individual task. Tasks in a task group will not run the `pre`
and `post` blocks in the YAML file; instead, the tasks will run the task group's
setup and teardown blocks.

Task groups have additional options available that can be configured
directly inline inside the config's [build
variants](#build-variants).

``` yaml
task_groups:
  - name: example_task_group
    max_hosts: 2
    setup_group_can_fail_task: true
    setup_group_timeout_secs: 1200
    setup_group:
      - command: shell.exec
        params:
          script: echo setup_group
    teardown_group_timeout_secs: 60
    teardown_group:
      - command: shell.exec
        params:
          script: echo teardown_group
    setup_task_can_fail_task: true
    setup_task_timeout_secs: 1200
    setup_task:
      - command: shell.exec
        params:
          script: echo setup_task
    teardown_task_can_fail_task: true
    teardown_task_timeout_secs: 1200
    teardown_task:
      - command: shell.exec
        params:
          script: echo teardown_task
    callback_timeout_secs: 60
    timeout:
      - command: shell.exec
      - params:
          script: echo timeout
    tasks:
      - example_task_1
      - example_task_2
      - .example_tag

buildvariants:
  - name: ubuntu1604
    display_name: Ubuntu 16.04
    run_on:
      - ubuntu1604-test
    tasks:
      - name: "example_task_group"
```

Parameters:

-   `setup_group`: commands to run prior to running this task group. These
    commands run once per host that's running tasks in the task group. Note that
    `pre` does not run for task group tasks.
-   `setup_group_can_fail_task`: if true, task will fail if a command in
    `setup_group` fails. Defaults to false.
-   `setup_group_timeout_secs`: set a timeout for the `setup_group`. Defaults to
    2 hours. Hitting this timeout will stop the `setup_group` commands but will
    not cause the task to fail unless `setup_group_can_fail_task` is true.
-   `teardown_group`: commands to run after running this task group. These
    commands run once per host that's running the task group tasks. Note that
    `post` does not run for task group tasks.
-   `teardown_group_timeout_secs`: set a timeout for the `teardown_group`.
    Defaults to 15 minutes. Hitting this timeout will stop the `teardown_task`
    commands but will not cause the task to fail.
-   `setup_task`: commands to run prior to running each task in the task group.
    Note that `pre` does not run for task group tasks.
-   `setup_task_can_fail_task`: if true, task will fail if a command in
    `setup_task` fails. Defaults to false.
-   `setup_task_timeout_secs`: set a timeout for the `setup_task`. Defaults to 2
    hours. Hitting this timeout will stop the `setup_task` commands but will not
    cause the task to fail unless `setup_group_can_fail_task` is true.
-   `teardown_task`: commands to run after running each task in the task group.
    Note that `post` does not run for task group tasks.
-   `teardown_task_can_fail_task`: if true, task will fail if a command in
    `teardown_task` fails. Defaults to false.
-   `teardown_task_timeout_secs`: set a timeout for the `teardown_task`.
    Defaults to 30 minutes. Hitting this timeout will stop the `teardown_task`
    commands but will not cause the task to fail unless
    `teardown_task_can_fail_task` is true.
-   `max_hosts`: number of hosts across which to distribute the tasks in
    this group. This defaults to 1. There will be a validation warning
    if max hosts is less than 1 or greater than the number of tasks in
    task group. When max hosts is 1, this is a special case where the tasks
    will run serially on a single host. If any task fails, the task group
    will stop, so the remaining tasks after the failed one will not run.
-   `timeout`: timeout handler which will be called instead of the top-level
    timeout handler. If it is not present, the top-level timeout handler will
    run if a top-level timeout handler exists. See [timeout
    handler](#timeout-handler).
-   `callback_timeout_secs`: set a timeout for the `timeout` block. Defaults to
    15 minutes.
-   `share_processes`: by default, processes and Docker state changes
    (e.g. containers, images, volumes) are cleaned up between each
    task's execution. If this is set to true, cleanup will be deferred
    until the task group is finished. Defaults to false.

The following constraints apply:

-   Tasks can appear in multiple task groups. However, no task can be
    assigned to a build variant more than once.
-   Task groups are specified on variants by name. It is an error to
    define a task group with the same name as a task.
-   Some operations may not be permitted within the "teardown_group"
    phase, such as "attach.results" or "attach.artifacts".
-   Tasks within a task group will be dispatched in order declared.
-   Any task (including members of task groups), can depend on specific
    tasks within a task group using the existing dependency system.
-   `patchable`, `patch_only`, `disable`, and `allow_for_git_tag` from the project
    task will overwrite the task group parameters (Note: these settings currently do not work when defined on the task
    group due to [EVG-19706](https://jira.mongodb.org/browse/EVG-19706)).

Tasks in a group will be displayed as
separate tasks. Users can use display tasks if they wish to group the
task group tasks.

### Task Dependencies

A task can be made to depend on other tasks by adding the depended on
tasks to the task's depends_on field. The following additional
parameters are available:

-   `status` - string (default: "success"). One of ["success",
    "failed", or "`*`"]. "`*`" includes any finished status as well
    as when the task is blocked.
-   `variant` - string (by default, uses existing variant). Can specify a 
     variant for the dependency to exist in, or "`*`" will depend on the task
     for all matching variants.
-   `patch_optional` - boolean (default: false). If true the dependency
    will only exist when the depended on task is present in the version
    at the time the dependent task is created. The depended on task will
    not be automatically pulled in to the version.
-   `omit_generated_tasks` - boolean (default: false). If true and the
    dependency is a generator task (i.e. it generates tasks via the
    [`generate.tasks`](Project-Commands.md#generatetasks) command), then generated tasks will not be included
    as dependencies.

So, for example:

``` yaml
- name: my_dependent_task
  depends_on:
    - name: "must_succeed_first"
      variant: "bv0"
    - name: "must_run_or_block_first"
      variant: "bv0"
      status: "*"
    - name: "must_succeed_first_if_present"
      variant: "bv0"
      patch_optional: true
    - name: "generator_task_one"
      variant: "bv0"
    - name: "generator_task_two"
      variant: "bv0"
      omit_generated_tasks: true
```

You can specify NOT with `!` and ALL with `*`. Multiple arguments are
supported as a space-separated list. For example,

``` yaml
- name: push
  depends_on:
  - name: test
    variant: "* !E"

- name: push
  depends_on:
  - name: test
    variant: "A B C D"
```

[Task/variant tags](#task-and-variant-tags) 
can also be used to define dependencies.

``` yaml
- name: push
  depends_on:
  - name: test
    variant: ".favorite"

- name: push
  depends_on:
  - "!.favorite !.other" ## runs all tasks that don't match these tags
```

### Ignoring Changes to Certain Files

Some commits to your repository don't need to be tested. The obvious
examples here would be documentation or configuration files for other
Evergreen projects---changes to README.md don't need to trigger your
builds. 

To address this, project files can define a top-level `ignore`
list of gitignore-style globs which tell Evergreen to not automatically
run tasks for commits that only change ignored files, and we will not 
create PR patches but instead send a successful status. 

``` yaml
ignore:
    - "version.json" ## don't schedule tests for changes to this specific file
    - "*.md" ## don't schedule tests for changes to any markdown files
    - "*.txt" ## don't schedule tests for changes to any txt files
    - "!testdata/sample.txt" ## EXCEPT for changes to this txt file that's part of a test suite
```

In the above example, a commit that only changes `README.md` would not
be automatically scheduled, since `*.md` is ignored. A commit that
changes both `README.md` and `important_file.cpp` *would* schedule
tasks, since only some of the commit's changed files are ignored.

Full gitignore syntax is explained
[here](https://git-scm.com/docs/gitignore). Ignored versions may still
be scheduled manually, and their tasks will still be scheduled on
failure stepback.

### Customizing Logging

By default, tasks will log all output to Cedar buildlogger. You can
override this behavior at the project or command level by log type
(task, agent, system). The available loggers are:

-   `file` - Output is sent to a local file on the agent, which will be
    uploaded to S3 at the end of the task. Links to the files will be
    available on the task page. This option is not available to system
    logs for security reasons. The `log_directory` parameter may be
    specified to override the default storage directory for log files.
-   `splunk` - Output is sent to Splunk. No links will be provided on
    the task page, but the logs can be searched using
    `metadata.context.task_id=<task_id>` as a Splunk query. Choosing
    this logger requires that the `splunk_server` parameter be set to
    the Splunk server URL and the `splunk_token` set to the API token
    used to authenticate.

Multiple loggers can be specified. An example config file that wants to
send task logs to S3 in addition to the Cedar buildlogger is:

``` yaml
command_type: test
loggers:
  task:
    - type: buildlogger
    - type: file
  agent:
    - type: buildlogger
  system:
    - type: buildlogger
tasks:
  [...]
```

All of this can be specified for a specific command as well, which will
only apply the settings to that command. An example is:

``` yaml
- command: subprocess.exec
  type: setup
  loggers:
    task:
      - type: splunk
        splunk_server: ${splunk_server}
        splunk_token: ${splunk_token}
    agent:
      - type: buildlogger
    system:
      - type: buildlogger
  params:
    [...]
```

### The Power of YAML

YAML as a format has some built-in support for defining variables and
using them. You might notice the use of node anchors and references in
some of our project code. For a quick example, see:
<http://en.wikipedia.org/wiki/YAML#Reference>

### Command Failure Colors

Evergreen tasks can fail with different colors. By default failing tasks
turn red, but there are 3 different modes.

-   `test`: red
-   `system`: purple
-   `setup`: lavender

In general you should use purple to indicate that something has gone
wrong with the host running the task, since Evergreen will also use this
color. You can use lavender to indicate something has gone wrong with
test setup, or with some external service that the task depends on.

You can set the default at the top of the config file.

``` yaml
command_type: system
```

You can set the failure mode of individual commands.

``` yaml
- command: shell.exec
     type: test
```

Note that although you cannot conditionally make a command fail
different colors, you can hack this by having a command write to a file
based on its exit status, and then subsequent commands with different
types can exit non-zero conditionally based on the contents of that
file.

### Task Fields Override Hierarchy

Some task fields can be specified at multiple levels in the YAML.

If a field is defined at multiple levels and they conflict, the one with the
highest priority will overwrite the others. The task's specific fields will be
taken into priority in the following order (from highest to lowest):

- Tasks listed under a build variant.
- The task definition.
- The build variant definition.

Example:

``` yaml
buildvariants:
- name: build_variant_definition
  run_on:
    - lowest_priority
  tasks: 
    - name: task_definition
      run_on: highest_priority
tasks:
- name: task_definiton
  run_on: mid_priority
```

#### Dependency Override Hierarchy
Task fields all follow the same priority rules, except for `depends_on`, for
which a build variant's `depends_on` overrides the task definition's
`depends_on`. `depends_on` will be taken into priority in the following order
(from highest to lowest):

- Tasks listed under a build variant.
- The build variant definition.
- The task definition.
