# Task Debugger

The task debugger lets you re-run Evergreen task commands on a debug-enabled spawn host. Instead of waiting for a new task run in CI, you can step through commands one at a time, inspect output, override variables, and re-run failed steps interactively. All debugger commands are accessed via `evergreen debug`.

## Prerequisites

- When creating a spawn host from a task, you must select the **"Debug Mode"** checkbox in the optional host details section of the spawn host UI. This option is only available if debug spawn hosts are enabled for your project.
- Debug spawn hosts must be enabled for your project. If the Debug Mode option is not available or you see an error about debug spawn hosts being disabled, contact your project admin.
- You must be authenticated via OAuth. The CLI will prompt you to authenticate on first use after SSHing into the host.

## Security Limitations
- Service users cannot use the debug feature, you must use a regular user account.
- Admin-only variables configured in your project are not available in debug sessions for security reasons.

## Setup Phase

When creating a debug spawn host from a task, you can optionally run a setup phase to prepare the environment:

- In the spawn host UI, select a specific step as your starting point
- Evergreen will automatically run all commands up to (but not including) that step
- You'll receive a notification when the setup completes successfully

This is useful when you want to debug a step that requires prior setup. For example, if you want to debug step 5 which runs tests, you can have the setup phase run steps 1-4 which install dependencies and compile code. When you SSH in, the environment will be ready with everything up to step 5 already completed.

## Quick Start

When you SSH into a debug spawn host, the debugger daemon is already running. You can start debugging immediately:

```bash
# 1. Check that the daemon is running
evergreen debug daemon status

# 2. Load your project configuration
evergreen debug load path/to/project.yml

# 3. Select a task to debug
evergreen debug select my_task_name

# 4. List steps to see what will run
evergreen debug list-steps

# 5. Step through commands one at a time
evergreen debug next

# 6. Or run all remaining steps at once
evergreen debug run-all
```

## Command Reference

### Daemon Management

The debugger runs as a background process. **Note: The daemon is typically already running when you SSH into a debug spawn host.** You only need to use these commands if the daemon has stopped or crashed.

#### `evergreen debug daemon start`

Start the debugger

```bash
evergreen debug daemon start
evergreen debug daemon start --port 8080
```

| Flag | Description | Default |
|------|-------------|---------|
| `--port`, `-p` | Port for the debugger to listen on | `9090` |

Only one debugger can run at a time. If one is already running, you will get an error. Use `daemon stop` first if you need to restart it.

#### `evergreen debug daemon stop`

Stop a running debugger.

```bash
evergreen debug daemon stop
```

Safe to run even if the debugger is not currently running.

#### `evergreen debug daemon status`

Check whether the debugger is running.

```bash
evergreen debug daemon status
```

If a task is selected, also displays the current step and total number of steps:

```
Daemon is running
Task: compile (step 3/10)
```

If the debugger is not running, prints:

```
Daemon is not running
```

### Configuration

#### `evergreen debug load <config.yml>`

Load a project config file. The path can be relative or absolute. Must be run after starting the daemon and before selecting a task.

```bash
evergreen debug load evergreen.yml
# OR
evergreen debug load /home/user/project/evergreen.yml
```

On success, reports the number of tasks and build variants found:

```
Loaded configuration: /home/user/project/evergreen.yml
Tasks: 12, Variants: 5
```

### Task Selection

#### `evergreen debug select <task_name>`

Select a task from the loaded configuration to debug. Reports the total number of steps in the task.

```bash
evergreen debug select compile
```

```
Selected task: compile
Total steps: 8
```

Selecting a new task clears any previous session state, including logs, step progress, and custom variables.

### Execution Control

These commands run task steps and stream their output to the terminal in real time.

#### `evergreen debug next`

Execute the next step.

```bash
evergreen debug next
```

If the step fails, execution stops and the error is displayed.

#### `evergreen debug run-all`

Run all remaining steps from the current position to the end of the task.

```bash
evergreen debug run-all
```

Stops immediately on the first step that fails.

#### `evergreen debug run-until <step>`

Run from the current position up to and including the specified step. See [Understanding Step Numbers](#understanding-step-numbers) for the step format.

```bash
evergreen debug run-until 5
evergreen debug run-until 3.2
evergreen debug run-until pre:1
```

Stops immediately on the first step that fails.

#### `evergreen debug jump <step>`

Move the current position to a step without executing it. Useful for skipping ahead or going back to re-run a step.

```bash
evergreen debug jump 3
```

```
Jumped to step 3
```

### Inspection

#### `evergreen debug list-steps`

Display all steps in the selected task with their execution status.

```bash
evergreen debug list-steps
```

Example output:

```
Steps:
  pre:1: setup environment ✓
  pre:2: install dependencies ✓
  1: clone repository ✓
  2: apply patch ✓
→ 3: compile
  4: run tests
  5.1: upload_results > attach.results
  5.2: upload_results > s3.put
  post:1: cleanup workspace
```

| Symbol | Meaning |
|--------|---------|
| `→` | Current step (will be executed next) |
| `✓` | Step completed successfully |
| `✗` | Step failed |
| _(no symbol)_ | Step has not been executed yet |

#### `evergreen debug logs`

View logs from the current debug session.

```bash
evergreen debug logs
evergreen debug logs --step 3
evergreen debug logs --tail 50
evergreen debug logs --setup
```

| Flag | Description |
|------|-------------|
| `--step STEP` | Show logs from a specific step only (e.g., `3`, `2.1`, `pre:1`) |
| `--setup` | Show setup phase logs instead of session logs |
| `--tail N` | Show only the last N lines |

If there are no logs matching your filters, prints:

```
No logs found.
```

### Variables

#### `evergreen debug set-var <key>=<value>`

Set a custom variable for the debug session. This can be used to override expansion variables used by task commands.

```bash
evergreen debug set-var MY_FLAG=--verbose
```

```
Set variable: MY_FLAG=--verbose
```

Variables persist until you select a new task.

## Understanding Step Numbers

Steps are numbered based on where they appear in the task definition. The debugger uses these step numbers with the `run-until`, `jump`, and `logs --step` commands.

| Format | Meaning | Example |
|--------|---------|---------|
| `N` | Step N in the main task block | `3` |
| `N.M` | Sub-step M within step N (commands inside a function) | `2.1` |
| `pre:N` | Step N in the pre-task block | `pre:1` |
| `pre:N.M` | Sub-step M within pre-task step N | `pre:1.2` |
| `post:N` | Step N in the post-task block | `post:1` |
| `post:N.M` | Sub-step M within post-task step N | `post:1.2` |

When a task definition calls a **function** that contains multiple commands, each command inside that function becomes a sub-step. For example, if step 5 calls a function with two commands, they appear as `5.1` and `5.2`. Functions with a single command do not use sub-step numbering.

Use `list-steps` to see the full step numbering for your selected task.

## Limitations

The debug environment has some intentional limitations for security and operational reasons:

- **Admin-only variables** are not available in debug sessions. If your task depends on admin-only variables, you'll need to work with your project admin to either make them non-admin or provide alternative values.
- **Service users** cannot use the debug feature. You must use a regular user account.
- **GitHub tokens** generated in debug mode use "evergreen debug agents" permissions configured by your project admin, which may differ from regular task permissions.
- **AWS assume role** operations use a different external ID format (`debug-[project_id]`) which must be explicitly allowed in your AWS trust policy.

## Commands Skipped in Local Execution

Some Evergreen commands cannot run outside the Evergreen CI environment. These are automatically skipped during local execution. When a command is skipped, a message is logged and the step is marked as succeeded so execution can continue.

| Command | Reason |
|---------|--------|
| `host.create` | Dynamic host creation requires the Evergreen service |
| `host.list` | Host listing requires the Evergreen service |
| `generate.tasks` | Dynamic task generation requires the Evergreen service |
| `downstream_expansions.set` | Downstream expansions are not available locally |
| `attach.xunit_results` | Test result attachment requires the Evergreen service |
| `attach.results` | Result attachment requires the Evergreen service |
| `attach.artifacts` | Artifact attachment requires the Evergreen service |
| `papertrail.trace` | Papertrail tracing is not available locally |
| `keyval.inc` | Key-value increment requires the Evergreen service |
| `perf.send` | Performance metrics submission requires the Evergreen service |
| `s3.put` | S3 uploads require Evergreen-managed credentials |
| `s3Copy.copy` | S3 copies require Evergreen-managed credentials |

Skipped steps still appear in `list-steps` and are marked as succeeded (`✓`).

## Reading Logs

Debug session logs are stored locally and can be viewed with `evergreen debug logs`. Each log line includes a timestamp and step number:

```
[2025-01-15T10:30:45.123Z] [step:3] + go test -v ./...
[2025-01-15T10:30:45.456Z] [step:3] ok  github.com/example/pkg  1.234s
```

Step boundaries are marked with delimiters:

```
=== STEP 3 START run_tests (main) ===
[2025-01-15T10:30:45.123Z] [step:3] + go test -v ./...
[2025-01-15T10:30:46.789Z] [step:3] ok  github.com/example/pkg  1.234s
=== STEP 3 END success=true duration=1.7s ===
```

Common log viewing patterns:

```bash
# View all session logs
evergreen debug logs

# View logs for a specific step
evergreen debug logs --step 3

# View logs for a pre-task step
evergreen debug logs --step pre:1

# View just the last 20 lines
evergreen debug logs --tail 20

# View setup phase logs (separate from session logs)
evergreen debug logs --setup
```

## Common Workflows

### Investigating a Failed Step

```bash
# List steps to find which one failed
evergreen debug list-steps

# Check the logs for the failed step
evergreen debug logs --step 3

# After fixing the issue, jump back to re-run it
evergreen debug jump 3
evergreen debug next
```

### Skipping Past Known-Good Steps

If you know the first several steps succeed, jump ahead to save time:

```bash
evergreen debug jump 5
evergreen debug next
```

Or run only the steps you care about:

```bash
evergreen debug run-until 7
```

### Overriding a Variable

If a step uses a variable you want to change:

```bash
evergreen debug set-var BUILD_TYPE=debug
evergreen debug jump 3
evergreen debug next
```

## Troubleshooting

| Error | Cause | Solution |
|-------|-------|----------|
| "daemon not running (port file not found)" | Ran a command without starting the debugger | Run `evergreen debug daemon start` first |
| "daemon not responding (may have crashed)" | Debugger process exited unexpectedly | Run `evergreen debug daemon stop` then `evergreen debug daemon start` |
| "step number '...' not found" | Step number does not match any step in the task | Run `evergreen debug list-steps` to see valid step numbers |
| "no more steps to execute" | Already at the end of the task | Use `jump` to go back, or select a new task |
| "Variable '...' is not available" | Variable is admin-only | Contact your project admin or use a different variable |
