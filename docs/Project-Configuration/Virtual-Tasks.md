# Virtual Tasks

A virtual task is a special type of task that has the option to either:
1. run like a regular task OR
2. have its final task information (e.g. status, test results, logs)
   populated by another task in the version (called the "runner").

Virtual tasks are useful when the real work for a task is completed externally
but you still want to display results in the normal task UI and support other
task features like notifications. When first created, a virtual task is inactive
by default, so it doesn't run on a host. Instead, the typical expectation is that
its results are produced and sent to Evergreen by the runner task.

## Regular Tasks vs. Virtual Tasks Behavior

| Behavior                  | Regular task                                 | Virtual task                                                                                |
| ---                       | ---                                          | ---                                                                                         |
| First execution           | Runs on a host by default                    | Inactive by default; does not run on a host unless activated                                |
| Results                   | Comes directly from the task's execution     | Typically pushed to it by a runner task; can be produced like a regular task if activated)  |
| Cron/batchtime activation | Activates the task at the specified interval | Does **not** activate the task                                                              |
| Host info                 | Shown                                        | Hidden if push-completed (e.g. spawn host, host/distro info, cost), shown if runs regularly |

A virtual task appears in the UI like any other task. A "Virtual" badge is
shown on the task page for all executions.

- On **push-completed** executions, the badge tooltip explains that the task's
  results were pushed by another task and that it did not run on a host.
- On **activated or restarted** executions where the task actually ran, the
  tooltip notes that this is a virtual task that ran on a host for that
  execution.

## Enabling Virtual Tasks

Virtual tasks must be enabled in the project before they can be used (see
[Project and Distro Settings](Project-and-Distro-Settings#virtual-tasks)).

If the setting is disabled, any virtual tasks in a version are skipped — they
are not created, and any pre-existing virtual tasks cannot be push-completed.

## Defining a Virtual Task

Define a virtual task by setting `virtual: true` on its task definition (either
in the project configuration YAML or in [generate.tasks](Project-Commands#generatetasks)):

```yaml
tasks:
  - name: my_virtual_task
    virtual: true
    run_on: ubuntu2204
    depends_on:
      - name: compile
    commands:
      - func: "run my virtual task"
```

The task's `commands` define what the virtual task runs when it is [activated or
restarted to run on a host](#activation-and-execution-lifecycle).

## Push-Completing a Virtual Task

The runner pushes results for virtual tasks using the
[`virtual_tasks.complete`](Project-Commands#virtualtaskscomplete) agent command.
The runner can push a task's status, test results, artifacts, and external
execution metadata.

### Push-Completing a Virtual Task Through the REST API

As an alternative to the `virtual_tasks.complete` command, the push-completion
API route can be called directly by a service user that has task admin
permissions.

```
POST /rest/v2/task/{task_id}/virtual_tasks/complete
```

See the [REST route](../API/REST-V2-Usage#tag/tasks/paths/~1task~1{task_id}~1virtual_tasks~1complete/post)
for the full route specification, including the request and response schema.

## Activation and Execution Lifecycle

While virtual tasks are default inactive, they can be activated manually (e.g.
by a user scheduling it), by stepback, or if it's a dependency of another
activated task. If a virtual task is restarted, the new execution is activated
to run. Virtual tasks ignore cron/batchtime activation.
