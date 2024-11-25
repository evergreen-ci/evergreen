# Task Runtime Behavior

This article describes the detailed specification for how a task behaves once
it's running. It also assumes you already have basic knowledge of task
configuration. For more information on how to configure a task, see [project
configuration](Project-Configuration-Files) or [project
commands](Project-Commands).

## Initial Task Setup

Before a task actually runs, it has to run some initial steps to prepare to run
the task, such as gathering task data, setting up logging for the task, and
creating a working directory. This setup is called `setup.initial`. If
`setup.initial` encounters an issue, your task may system fail. Since this setup
is not directly configurable by users, any failures caused by `setup.initial`
are issues on Evergreen's end.

## Command Block Execution
### Command Block Order

Once the task is set up, the command blocks will run their commands. For a
task that's not part of a task group, the blocks will run in this order:

1. [`pre`](Project-Configuration-Files#pre-and-post)
2. Main task commands (i.e. commands listed under the task definition)
3. [`timeout`](Project-Configuration-Files#timeout-handler) (only runs if the task [hit a timeout](#task-timeouts))
4. [`post`](Project-Configuration-Files#pre-and-post)

For a task that _is_ part of a [task group](Project-Configuration-Files#task-groups), the blocks will run in this
order:

1. `setup_group` (only runs if it's the first task in this task group running on
   the host)
2. `setup_task`
3. Main task commands (i.e. commands listed under the task definition)
4. `timeout` (only runs if the task [hit a timeout](#task-timeouts))
5. `teardown_task`
6. `teardown_group` (only runs if it's the last task in the task group running
   on the host)

### Command Behavior

Each block will run its list of commands from top to bottom. As long as commands
succeed, the commands will continue running in order. If a command fails, it may
either continue on error, or treat it as a task failure (and stop running
further commands in the block). Whether it continues on error or fails the task
depends on which block it runs in and how that block is configured in the
project YAML.

For example, consider the following `pre` configuration:

```yaml
pre:
  # This command will fail and continue on error. This failing command will not
  # cause the task to fail.
  - command: shell.exec
    params:
      script: exit 1
  # This command will succeed.
  - command: shell.exec
    params:
      script: exit 0
```

If a command fails in `pre`, by default the task will continue on error, and
will _not_ cause the task to fail. The `pre` block will simply log the failing
command's error and continue running any remaining `pre` commands.

However, if [`pre_error_fails_task`](Project-Configuration-Files#pre-and-post) is set to true and a `pre` command fails:

```yaml
pre_error_fails_task: true
pre:
  # This command will fail and cause the task to skip forward to the post
  # commands. This failing command will cause the task to fail.
  - command: shell.exec
    params:
      script: exit 1
  # This command will not run.
  - command: shell.exec
    params:
      script: exit 0
```

In this case, the `pre` block will not run any further commands because the
first command failed. Instead, it will skip forward to running `post` commands
and eventually report the task as failed.

By default, all blocks follow the above behavior of continuing on error (with
configurable options to have commands instead fail on error) _except_ the main
task command block. The main task commands are treated specially when compared
to the other command blocks. If a command fails in the main task commands, it
will cause the task to fail. The remaining main task commands will not run, it
will skip forward to running `post` commands, and will eventually report the
task as failed.

### Process Cleanup during a Task

In between some command blocks, the task will try to clean up processes and
Docker resources that were potentially created by commands. Process cleanup will
stop any lingering processes and clean up Docker resources such as containers,
images, and volumes. For example, if the task has a background `mongod` process
started via [`subprocess.exec`](Project-Commands#subprocessexec) or if a `subprocess.exec` executable has also
started some child processes, the resource cleanup process will catch these
processes and kill them.

For a task that's not part of a task group, the task will clean up processes in
between these blocks:

- `pre`
- Main task commands
- `timeout`
- Process cleanup
- `post`
- Process cleanup

For a task that _is_ part of a task group, whether or not it cleans up between
command blocks is determined by its `share_procs` configuration. If
`share_procs` is false (or unset), the task will clean up processes in between
these command blocks:

- `setup_group`
- `setup_task`
- Main task commands
- `timeout`
- Process cleanup
- `teardown_task`
- Process cleanup
- `teardown_group`
- Process cleanup

If `share_procs` is true, the task group will not clean up processes for the
entire duration of the task. It will only clean up those processes once the
entire task group is finished.

Check the Agent Logs on a task to see logs about the process cleanup.

### Task Directory Cleanup

The task working directory is removed when a task finishes as part of cleaning
up the task. Note that _only_ the task directory is cleaned up - if any file is
written outside the task directory, it will not be cleaned up. As part of
[Evergreen best practices](Best-Practices#task-directory),
writing outside of the task directory is discouraged.

For a task that's not part of a task group, the task will clean up at the end of
the task, after all commands have finished running. For a task that _is_ part of
a task group, it will keep the task directory as long as it is running tasks
in the same task group. Once all the task group tasks have finished, it will
clean up the task directory.

### Global Git Config and Git Credentials Cleanup
For tasks not in a task group, the global git config and git credentials will be reset 
at the end of the task after all commands have finished running. This will be done by 
deleting the .git-credentials and .gitconfig files from the home directory. 
For a tasks in a task group, the reset will occur after the all the tasks in the task 
group tasks have finished.

## Task Timeouts

Tasks are not allowed to run forever, so all commands that run for a task are
subject to (configurable) timeouts. However, tasks cannot be configured to have a timeout 
greater than 86400 seconds (24 hours). If a command hits a timeout, that command
will stop with an error. Furthermore, if that command can cause the task to fail
and that command is in `pre`, `setup_task`, `setup_group`, or the main task
block, it will skip forward to running the `timeout` block and will eventually
report the task as failed due to timeout.

For example, consider the following YAML configuration:

```yaml
exec_timeout_secs: 10

tasks:
  - name: some-task
    commands:
      # This command will time out after 10 seconds due to execution timeout.
      - command: shell.exec
        params:
          script: sleep 100000

timeout:
  # This command will run after the main task command hits the execution
  # timeout.
  - command: shell.exec
    params:
      script: echo task hit a timeout
```

Since the main task command will hit the execution timeout and a failure in a
main task command always causes a task to fail, this will trigger the `timeout`
block commands to run.

Note that some blocks, such as `post`, `teardown_task`, and `teardown_group`
run after the `timeout` block and therefore cannot trigger the `timeout` block
to run.

## Aborting a Task

A task can be aborted once it's started running but before it's finished. If a
task is aborted, the task will try to finish, while still performing any final
cleanup.

If the task is running `pre` `setup_group`, `setup_task`, or the main task
commands when the task is aborted, the task will skip forward to running `post`
(or the task group equivalents, `teardown_task` and `teardown_group`). The task
runs `post` commands in order to perform any final cleanup for the task before
moving on to the next task. Note that because `post` must run, aborting a task
once it's already running `post` will not do anything.

## REST API for Tasks

When a task is running, there are some local REST endpoints available to get
information about the runtime environment or influence how the task runs.

### Manually Set Task Status

The following endpoint was created to give tasks the ability to define their own
task end status, rather than relying on hacky YAML tricks to use different
failure types for the same command. This user-defined status will take
precedence over the default status the task would have otherwise received. For
example, if the task encountered no command errors and was going to finish with
"success" but the task posted a "failed" status to this endpoint, the task's
status will be "failed".

Note: Overriding the default task status is intended for fairly niche use cases,
such as manually setting the task to "failed" after a background process
abruptly exits. Only use this if the default task status does not suit your
needs.

Posting a task status will not immediately stop the currently-running command.
Once the current command completes, it will check if the task end status is
defined, and will either continue or stop depending on whether `should_continue`
is set.

    POST localhost:2285/task_status

| Name                      | Type     | Description                                                                                                                                                                                                                                                    |
|---------------------------|----------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| status                    | string   | Required. The overall task status. This can be "success" or "failed". If this is configured incorrectly, the task will system fail.                                                                                                                            |
| type                      | string   | The failure type. This can be "setup", "system", or "test" (see [project configuration files](Project-Configuration-Files#command-failure-colors) for corresponding colors). If not specified, will default to the failure type of the last command that runs. |
| desc                      | string   | Provide details on the task failure. This is limited to 500 characters. If not specified or the message is too long, it will default to the display name of the last command that runs.                                                                        |
| should_continue           | boolean  | If set, the task will continue running commands, but the final status will be the one explicitly set. Defaults to false.                                                                                                                                       |
| add_failure_metadata_tags | []string | If set and the task status is set to "failed", then additional metadata tags will be associated with the failing command. See [here](Project-Commands#basic-command-structure) for more details on `failure_metadata_tags`.                                    |

Example in a command:

``` yaml
- command: shell.exec
     params:
        shell: bash
        # Manually set task end status to setup-failed and append failure metadata tags.
        script: |
          curl -d '{"status":"failed", "type":"setup", "desc":"this should be set", "should_continue": false, "add_failure_metadata_tags": ["failure_tag"]}' -H "Content-Type: application/json" -X POST localhost:2285/task_status
```
