# Task Debugger

> **Notice:**
> The task debugger is currently in **beta**. Features and behavior may change.

## Why Use the Task Debugger?

When a task fails in Evergreen CI, debugging can have friction points:

- Needing to add debug logging, push a commit, and wait for a new run
- Inability to inspect the exact state when the failure occurred
- Each debugging attempt requires a full CI cycle

**The task debugger addresses this** by letting you re-run failed commands interactively on a spawn host. You can step through commands one by one, inspect output, change expansions, and retry failures, all without waiting for new CI runs.

## How It Works

The task debugger has three main components:

1. **Debug-enabled spawn host** - A special spawn host created from your task that has its same environment
2. **Debugger daemon** - A background process (already running when you SSH in) that executes your commands
3. **Debug CLI commands** - Commands you run to control execution (`evergreen debug ...`)

This would be the typical workflow:

```text
Failed Task in UI → Create Debug Spawn Host → SSH into Host → Run Debug Commands
```

## Getting Started

### Create a Debug Spawn Host

From your failed task in the Evergreen UI:

1. Click the **"Spawn Host"** button on the task page
2. In the spawn host form, check **"Debug Mode"** in the optional settings
3. (Optional) Select a starting task [step](#understanding-step-numbers) from the dropdown in the spawnhost modal. The dropdown shows the step numbers and command names from your task. If you select a starting step, Evergreen will automatically run all prior steps during host setup, so you don't need to manually load the YAML, select the task, or run earlier steps yourself.

### Start Debugging

Follow these steps to start a debugging session:

1. **Check the debugger is ready.** This confirms the daemon is running.

   ```bash
   evergreen debug daemon status
   ```

2. **Load your project's configuration file.** This tells the debugger where to find your task definitions. The file is pre-loaded into the `~/debug_project_config/` directory on the spawn host.

   ```bash
   evergreen debug load ~/debug_project_config/evergreen.yml
   ```

3. **Select the task you want to debug.** Use the exact task name as it appears in your Evergreen configuration.

   ```bash
   evergreen debug select compile
   ```

4. **List the available steps.** This shows all steps in the task with their numbers, so you know what will run.

   ```bash
   evergreen debug list-steps
   ```

5. **Run the next step.** This executes the next step in sequence. If you selected a starting step when creating the host, execution begins from that step instead of the beginning.

   ```bash
   evergreen debug next
   ```

6. **Check the logs if something failed.** Use the step number from `list-steps` to inspect a specific step's output.

   ```bash
   evergreen debug logs --step 3
   ```

## Common Debugging Workflows

### Debugging a Failed Step

Your task failed at [step](#understanding-step-numbers) 5. Here's how to debug it:

```bash
# Load config and select your task
evergreen debug load ~/debug_project_config/evergreen.yml
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

### Running With Different Expansions

Need to test with different expansion values?

```bash
# Set a custom expansion
evergreen debug set-var VERBOSE=true
evergreen debug set-var BUILD_FLAGS="--debug"

# Re-run the affected step
evergreen debug jump 3
evergreen debug next
```

### Skipping Setup Steps

If you know steps 1-3 work fine and want to debug step 4:

```bash
evergreen debug load ~/debug_project_config/evergreen.yml
evergreen debug select my_task

# Jump straight to step 4 and execute it
evergreen debug jump 4
evergreen debug next
```

### Hot Reloading Configuration

You can modify your `evergreen.yml` file and reload it between steps to test configuration changes. This continues from your current position and execution environment, it does not restart the debugger from the beginning.

```bash
# Edit your evergreen.yml file
vim ~/debug_project_config/evergreen.yml

# Reload the modified configuration
evergreen debug load ~/debug_project_config/evergreen.yml

# Your task selection, step position, and custom expansions are all preserved
# Continue debugging with the updated configuration
evergreen debug next
```

This is useful when:

- Testing different command arguments or flags
- Adjusting timeout values
- Modifying shell scripts within the config
- Adding or removing commands from a task

The hot reload preserves:

- Your current task selection (no need to re-select)
- Your current step position
- Custom expansions set with `set-var`
- Execution history of completed steps

## Command Reference

### Configuration Commands

#### `evergreen debug load <config.yml>`

Load a project configuration file. The path can be relative or absolute. Must be run before selecting a task.

```bash
evergreen debug load ~/debug_project_config/evergreen.yml
evergreen debug load /home/user/project/evergreen.yml
```

On success, reports the number of tasks and build variants found:

```text
Loaded configuration: /home/user/project/evergreen.yml
Tasks: 12, Variants: 5
```

#### `evergreen debug select <task_name> [--variant <variant_name>]`

Select a task from the loaded configuration to debug. Reports the total number of steps in the task.

```bash
evergreen debug select compile
evergreen debug select compile --variant ubuntu2204
```

Output:

```text
Selected task: compile
Total steps: 8
```

| Flag        | Description                                                      |
| ----------- | ---------------------------------------------------------------- |
| `--variant` | (Optional) Select a specific build variant's version of the task |

Note: Selecting a new task clears session logs. Custom expansions set with `set-var` persist across task selections.

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

```text
Jumped to step 3
```

#### `evergreen debug set-var <key>=<value>`

Set a custom expansion for the debug session. This can be used to override expansion values used by task commands. Expansions persist until you select a new task.

```bash
evergreen debug set-var MY_FLAG=--verbose
evergreen debug set-var BUILD_TYPE=debug
```

Output:

```text
Set expansion: MY_FLAG=--verbose
```

### Inspection Commands

#### `evergreen debug list-steps`

Display all steps in the selected task with their execution status.

```bash
evergreen debug list-steps
```

Example output:

```text
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

| Symbol | Meaning                              |
| ------ | ------------------------------------ |
| `→`    | Current step (will be executed next) |
| `✓`    | Step completed successfully          |
| `✗`    | Step failed                          |

#### `evergreen debug logs`

View logs from the current debug session.

```bash
evergreen debug logs           # All logs
evergreen debug logs --step 3.1  # Just step 3.1
evergreen debug logs --tail 50 # Last 50 lines
evergreen debug logs --setup    # Setup phase logs
```

| Flag          | Description                                                                                    |
| ------------- | ---------------------------------------------------------------------------------------------- |
| `--step STEP` | Show logs from a specific [step](#understanding-step-numbers) only (e.g., `3`, `2.1`, `pre:1`) |
| `--setup`     | Show setup phase logs instead of session logs                                                  |
| `--tail N`    | Show only the last N lines                                                                     |

### Daemon Management Commands

The debugger runs as a background process. **Note: The daemon is already running when you SSH into a debug spawn host.** You only need these commands if the daemon has stopped or crashed.

#### `evergreen debug daemon status`

Check whether the debugger is running.

```bash
evergreen debug daemon status
```

Output when running with a task selected:

```text
Daemon is running
Task: compile (step 3/10)
```

Output when not running:

```text
Daemon is not running
```

#### `evergreen debug daemon start`

Start the debugger (only needed if it's not already running).

```bash
evergreen debug daemon start
evergreen debug daemon start --port 8080
```

| Flag           | Description                        | Default |
| -------------- | ---------------------------------- | ------- |
| `--port`, `-p` | Port for the debugger to listen on | `9090`  |

#### `evergreen debug daemon stop`

Stop a running debugger.

```bash
evergreen debug daemon stop
```

## Debugging Locally (Without a Spawn Host)

You can also use the task debugger on your local laptop without creating a spawn host. Instead of selecting a task manually, you must provide a task ID and the debugger fetches the project configuration directly from Evergreen. Be mindful that your local device architecture may have key discrepancies with the OS used by the passed in task ID.

### Requirements

- You must have **patch submit** permissions on the project associated with the task.
- Debug spawn hosts must be enabled for the project.

### Quick Start

1. **Find the task ID** of the task you want to debug.

2. **Start the daemon:**

   ```bash
   evergreen debug daemon start
   ```

3. **Load the task by ID:**

   ```bash
   evergreen debug load --task-id <task_id>
   ```

   This fetches the project configuration and task expansions from the server and automatically selects the task and its build variant. No local config file is needed.

   Output:

   ```text
   Loaded and auto-selected task: compile (variant: ubuntu2204)
   Total steps: 8
   ```

4. **Start stepping through:**

   ```bash
   evergreen debug list-steps
   evergreen debug next
   ```

In local usage, the task is automatically loaded into the debugger based on its ID. The `evergreen debug select` command is not permitted. Optionally, you can provide a local YAML path as well, which overrides the server-fetched config while keeping the task's expansions and variables. For example:

```bash
evergreen debug load --task-id <task_id> ./my-modified-evergreen.yml
```

## Prerequisites and Limitations

### Prerequisites

- **Debug Mode Required**: When creating the spawn host, you must check the "Debug Mode" option
- **Project Must Enable**: Debug spawn hosts are disabled by default. Your project admin must enable them in project settings.

### For Project Admins

Debug spawn hosts are **disabled by default**. To enable them, go to your project settings under the **General** section and uncheck the **"Debug Spawn Hosts Disabled"** toggle. Re-enabling the toggle will also automatically terminate any running debug hosts for the project.

Before enabling debug spawn hosts for your project, be aware of the following:

- **External state mutation**: Debug sessions may allow users to re-run task commands interactively. If your tasks mutate external state (e.g., uploading or downloading sensitive files, modifying databases, or calling external APIs), users could trigger those side effects repeatedly or in unexpected order. While many state-modifying Evergreen commands are [automatically skipped](#commands-that-will-no-op), custom shell commands in `subprocess.exec` or `shell.exec` are not restricted.
- **Credential exposure**: Admin-only project variables are excluded from debug sessions, but **private** and **public** project variables are still accessible. Before enabling this feature, ensure that any highly sensitive secrets (credentials, signing keys, etc.) are marked as **admin-only** so they are withheld from debug sessions entirely. For lower-sensitivity secrets, ensure they are at least marked as private so that they are redacted from task logs and other places where Evergreen may transmit data off the host.
- **AWS role assumptions**: Debug sessions use a [different external ID format](#ec2assume_role) for `ec2.assume_role`. Ensure your IAM trust policies are configured appropriately to scope debug access.
- **GitHub token permissions**: Debug sessions use a separate [permission configuration](#githubgenerate_token) for generated GitHub tokens. Configure the "Debug" requester type in your project's GitHub token settings to restrict permissions as needed.

### Security Limitations

- Admin-only variables are not available in debug sessions for non-admins
- Service users are not permitted to use the feature

### Special Command Behaviors

#### `github.generate_token`

When generating GitHub tokens in debug mode, the permissions are determined by the **"Debug"** requester type configured in your [github token project settings](../Project-Configuration/Github-Integrations#restricting-generated-tokens). This may be more restrictive than the permissions available during regular task execution.

Your project admin must configure the appropriate GitHub permissions for the task debugger in the project settings.

#### `ec2.assume_role`

When using `ec2.assume_role` in debug mode, the AWS external ID format is different from regular task execution:

- **Regular projects:** `debug-[project_id]-[requester]` (e.g., `debug-myproject-github_pull_request`)
- **Untracked projects:** `debug-untracked-[repo_ref_id]-[requester]`

Your AWS IAM trust policy must explicitly allow these debug external IDs. For example, if your regular external ID is `myproject-github_pull_request`, the debug external ID would be `debug-myproject-github_pull_request`.

Contact your infrastructure team to [update the trust policies](../Project-Configuration/Project-Commands#assumerole-aws-setup) for any roles you need to assume during debugging.

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

| Format   | Meaning                | Example  |
| -------- | ---------------------- | -------- |
| `N`      | Main task step         | `3`      |
| `N.M`    | Sub-step in a function | `2.1`    |
| `pre:N`  | Pre-task step          | `pre:1`  |
| `post:N` | Post-task step         | `post:1` |

These step numbers correspond exactly to what appears in the original task logs. For example, you might see a log line like:

```text
Running command 'shell.exec' in function 'get-project-and-modules' (step 1.3 of 4).
```

This tells you the command is at step `1.3`. Use `list-steps` to see the exact numbering for your task.

## Setup Phase

When creating a debug spawn host, you can have Evergreen automatically run steps 1-N (but NOT including N) before you SSH in:

1. In the spawn host UI, select a "starting step" for the debug host
2. Evergreen runs all prior steps automatically
3. The spawn host setup script completion notification will indicate the host is ready
4. SSH in with the environment already prepared

This is useful when debugging later steps that need setup (e.g., debugging tests after compilation).

## Troubleshooting

When investigating issues, check the following:

1. Output from `evergreen debug daemon status`
2. Relevant daemon logs from `~/.evergreen-local/daemon.log`
3. Relevant execution logs from `evergreen debug logs`
4. Relevant execution logs from the setup phase from `evergreen debug logs --setup`
