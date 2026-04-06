# Task Debugger

# Notice: Beta Feature
The task debugger is currently in beta. Features and behavior may change.

## Why Use the Task Debugger?

When a task fails in Evergreen CI, debugging can be painful:
- You add debug logging, push a commit, and wait for a new run
- You can't inspect the exact state when the failure occurred  
- Each debugging attempt requires a full CI cycle

**The task debugger addresses this** by letting you re-run failed commands interactively on a spawn host. You can step through commands one by one, inspect output, change variables, and retry failures, all without waiting for new CI runs.

## How It Works

The task debugger has three main components:

1. **Debug-enabled spawn host** - A special spawn host created from your task that has its same environment
2. **Debugger daemon** - A background process (already running when you SSH in) that executes your commands
3. **Debug CLI commands** - Commands you run to control execution (`evergreen debug ...`)

This would be the typical workflow:

```
Failed Task in UI → Create Debug Spawn Host → SSH into Host → Run Debug Commands
```

## Getting Started

### Create a Debug Spawn Host

From your failed task in the Evergreen UI:
1. Click the **"Spawn Host"** button on the task page
2. In the spawn host form, check **"Debug Mode"** in the optional settings
3. (Optional) Select a starting task [step](#understanding-step-numbers) you would like to start debugging at

### Start Debugging

The debugger is already running by default once you SSH in. Here's a minimal debugging session:

```bash
# Verify the debugger is ready
evergreen debug daemon status

# Load your project's configuration file
evergreen debug load ./evergreen.yml

# Select the task that failed (use the exact task name)
evergreen debug select compile

# See all the steps in your task
evergreen debug list-steps

# Execute the next step
evergreen debug next

# Check the logs if something failed
evergreen debug logs --step 3
```

That's it! You're now debugging your task interactively.

## Command Reference

### Configuration Commands

#### `evergreen debug load <config.yml>`

Load a project configuration file. The path can be relative or absolute. Must be run before selecting a task.

```bash
evergreen debug load ./evergreen.yml
evergreen debug load /home/user/project/evergreen.yml
```

On success, reports the number of tasks and build variants found:
```
Loaded configuration: /home/user/project/evergreen.yml
Tasks: 12, Variants: 5
```

#### `evergreen debug select <task_name>`

Select a task from the loaded configuration to debug. Reports the total number of steps in the task.

```bash
evergreen debug select compile
```

Output:
```
Selected task: compile
Total steps: 8
```

### Execution Commands

#### `evergreen debug next`

Execute the next [step](#understanding-step-numbers) and see its output in real-time. If the step fails, execution stops and the error is displayed.

```bash
evergreen debug next
```

#### `evergreen debug run-all`

Run all remaining [steps](#understanding-step-numbers) from the current position to the end of the task. Stops immediately on the first step that fails.

```bash
evergreen debug run-all
```

#### `evergreen debug run-until <step>`

Run from the current position up to and including the specified [step](#understanding-step-numbers). Stops immediately on the first step that fails.

```bash
evergreen debug run-until 5
evergreen debug run-until 3.2
evergreen debug run-until pre:1
```

#### `evergreen debug jump <step>`

Move the current position to a [step](#understanding-step-numbers) without executing it. Useful for skipping ahead or going back to re-run a step.

```bash
evergreen debug jump 3
```

Output:
```
Jumped to step 3
```

#### `evergreen debug set-var <key>=<value>`

Set a custom variable for the debug session. This can be used to override expansion variables used by task commands. Variables persist until you select a new task.

```bash
evergreen debug set-var MY_FLAG=--verbose
evergreen debug set-var BUILD_TYPE=debug
```

Output:
```
Set variable: MY_FLAG=--verbose
```

### Inspection Commands

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
evergreen debug logs           # All logs
evergreen debug logs --step 3.1  # Just step 3.1
evergreen debug logs --tail 50 # Last 50 lines
evergreen debug logs --setup    # Setup phase logs
```

| Flag | Description |
|------|-------------|
| `--step STEP` | Show logs from a specific [step](#understanding-step-numbers) only (e.g., `3`, `2.1`, `pre:1`) |
| `--setup` | Show setup phase logs instead of session logs |
| `--tail N` | Show only the last N lines |

### Daemon Management Commands

The debugger runs as a background process. **Note: The daemon is already running when you SSH into a debug spawn host.** You only need these commands if the daemon has stopped or crashed.

#### `evergreen debug daemon status`

Check whether the debugger is running.

```bash
evergreen debug daemon status
```

Output when running with a task selected:
```
Daemon is running
Task: compile (step 3/10)
```

Output when not running:
```
Daemon is not running
```

#### `evergreen debug daemon start`

Start the debugger (only needed if it's not already running).

```bash
evergreen debug daemon start
evergreen debug daemon start --port 8080
```

| Flag | Description | Default |
|------|-------------|---------|
| `--port`, `-p` | Port for the debugger to listen on | `9090` |

#### `evergreen debug daemon stop`

Stop a running debugger.

```bash
evergreen debug daemon stop
```

## Common Debugging Workflows

### Debugging a Failed Step

Your task failed at [step](#understanding-step-numbers) 5. Here's how to debug it:

```bash
# Load config and select your task
evergreen debug load ./evergreen.yml
evergreen debug select my_failing_task

# Execute up to the problem step
evergreen debug run-until 5 

# Try running it
evergreen debug next

# Check the detailed logs
evergreen debug logs --step 5

# Make any fixes needed (edit files, install tools, etc.)

# Try running the step again
evergreen debug jump 5
evergreen debug next
```

### Running With Different Variables

Need to test with different expansions or environment variables?

```bash
# Set a custom variable
evergreen debug set-var VERBOSE=true
evergreen debug set-var BUILD_FLAGS="--debug"

# Re-run the affected step
evergreen debug jump 3
evergreen debug next
```

### Skipping Setup Steps

If you know steps 1-3 work fine and want to debug step 4:

```bash
evergreen debug load ./evergreen.yml
evergreen debug select my_task

# Jump straight to step 4
evergreen debug jump 4
evergreen debug next
```

## Prerequisites and Limitations

### Prerequisites

- **Debug Mode Required**: When creating the spawn host, you must check the "Debug Mode" option
- **Project Must Enable**: Debug spawn hosts must be enabled for your project (contact your admin if not available)

### Security Limitations

- Admin-only variables are not available in debug sessions
- Service users cannot use the debug feature

### Commands That Will No-op

Commands that modify external Evergreen state are automatically skipped. These commands will show as "skipped" but won't block execution:

- `host.create`
- `host.list`
- `generate.tasks`
- `downstream_expansions.set`
- `attach.results`
- `attach.xunit_results`
- `gotest.parse_files`
- `attach.artifacts`
- `papertrail.trace`
- `keyval.inc`
- `perf.send`
- `s3.put`
- `s3Copy.copy`

## Understanding Step Numbers

Steps are numbered based on their position in your task:

| Format | Meaning | Example |
|--------|---------|---------|
| `N` | Main task step | `3` |
| `N.M` | Sub-step in a function | `2.1` |
| `pre:N` | Pre-task step | `pre:1` |
| `post:N` | Post-task step | `post:1` |

Use `list-steps` to see the exact numbering for your task.


## Setup Phase

When creating a debug spawn host, you can have Evergreen automatically run steps 1-N (but NOT including N) before you SSH in:

1. In the spawn host UI, select a "starting step" for the debug host
2. Evergreen runs all prior steps automatically
3. The spawn host secup script completion notification will indicate the host is ready
4. SSH in with the environment already prepared

This is useful when debugging later steps that need setup (e.g., debugging tests after compilation).

## Troubleshooting

| Problem | Solution                                                      |
|---------|---------------------------------------------------------------|
| "daemon not running" | Run `evergreen debug daemon start`                            |
| "daemon not responding" | Run `daemon stop` then `daemon start`                         |
| "step number not found" | Check valid steps with `list-steps`                           |
| "no more steps to execute" | Use `jump` to go back to an earlier step                      |