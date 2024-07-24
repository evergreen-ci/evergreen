# 2024-07-18 Allow module to be expanded in build variant configuration

- status: approved
- date: 2024-07-18
- authors: Zackary Santana

## Context and Problem Statement

Ticket: DEVPROD-8993

Typically, all project commands use options from their `params` field to configure the command, allowing the command to be customizable at runtime. The `git.get_project` command uses the `modules` field from the build variant the command is running in, which is not expandable and not customizable at runtime.

## Considered Options

1. Allow the `modules` field to be expanded in the build variant configuration as an exception to the rule. Since the `modules` field is _only_ used by the `git.get_project` command, it is only read at runtime (unlike every other field in the build variant configuration).

2. Create a new parameter in the `git.get_project` command that overrides the `modules` field in the build variant configuration. This field would be expandable but the `modules` field in the build variant configuration would remain unexpandable.

3. Recommend users generate a build variant with `generate.tasks` that includes the module they want to use in the `git.get_project` command. This would allow the user to customize the module at runtime.

## Decision Outcome

Option 2 introduces a new flow for the command which would make it harder to debug. Right now, the workflow for debugging and handling `git.get_project` command has been the same. We don't currently have a pattern where complex configuration is overridden by a command parameter so this option is preferred. Option 3 was rejected because it introduces extra steps, more resources, and more complexity for the user and it breaks the customizability of commands.

Although option 1 introduces expansions in to build variant configuration, this is _not_ a precedence for more expanded values. The `modules` field is a special case and is only used by the `git.get_project` command and we are effectively treating it as a field in the `params` for the command.
