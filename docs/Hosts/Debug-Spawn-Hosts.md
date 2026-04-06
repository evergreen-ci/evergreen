# Task Debugger

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
3. (Optional) Select a starting task step you would like to start debugging at

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

## Essential Commands

These four commands handle 80% of debugging scenarios:

### `evergreen debug next`
Execute the next step and see its output in real-time.

### `evergreen debug list-steps`
See all steps and where you are:
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

### `evergreen debug logs`
View output from executed steps:
```bash
evergreen debug logs           # All logs
evergreen debug logs --step 3  # Just step 3
evergreen debug logs --tail 50 # Last 50 lines
```

### `evergreen debug jump <step>`
Skip to a different step without executing:
```bash
evergreen debug jump 4  # Move to step 4
```

## Common Debugging Workflows

### Debugging a Failed Step

Your task failed at step 5. Here's how to debug it:

```bash
# Load config and select your task
evergreen debug load ./evergreen.yml
evergreen debug select my_failing_task

# Jump directly to the problem step
evergreen debug jump 5

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

Need to test with different environment variables?

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
- **Project Must Allow It**: Debug spawn hosts must be enabled for your project (contact your admin if not available)

### Security Limitations

- Admin-only variables are not available in debug sessions
- Service users cannot use the debug feature

### Commands That Will No-op

Commands that modify external Evergreen state are automatically skipped. These steps will show as "skipped" but won't block execution:

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

When creating a debug spawn host, you can have Evergreen automatically run steps 1-N before you SSH in:

1. In the spawn host UI, select a "starting step"
2. Evergreen runs all prior steps automatically
3. You receive a notification when ready
4. SSH in with the environment already prepared

This is useful when debugging later steps that need setup (e.g., debugging tests after compilation).

### Full Command Reference

#### Execution Commands

- `evergreen debug next` - Execute the next step
- `evergreen debug run-all` - Run all remaining steps
- `evergreen debug run-until <step>` - Run up to a specific step
- `evergreen debug jump <step>` - Move to a step without executing

#### Configuration Commands

- `evergreen debug load <config.yml>` - Load your project configuration
- `evergreen debug select <task_name>` - Select a task to debug
- `evergreen debug set-var <key>=<value>` - Set a custom variable

#### Inspection Commands

- `evergreen debug list-steps` - Show all steps with status
- `evergreen debug logs [options]` - View execution logs

#### Daemon Commands (rarely needed)

- `evergreen debug daemon status` - Check if daemon is running
- `evergreen debug daemon start` - Start daemon (if stopped)
- `evergreen debug daemon stop` - Stop the daemon

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "daemon not running" | Run `evergreen debug daemon start` |
| "daemon not responding" | Run `daemon stop` then `daemon start` |
| "step number not found" | Check valid steps with `list-steps` |
| "no more steps to execute" | Use `jump` to go back to an earlier step |
| "Variable X is not available" | Variable might be admin-only, use `set-var` to override |
| Debug Mode option not visible | Debug spawn hosts not enabled for your project, contact admin |

## Tips

- The daemon is usually already running when you SSH in—you don't need to start it
- Your task's working directory is typically where you land when you SSH in
- Use `list-steps` to understand where you are in the execution
- You can edit files directly on the spawn host between step executions