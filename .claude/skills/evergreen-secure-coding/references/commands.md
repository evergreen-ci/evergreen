# Shell, SSH, and Git Commands

Evergreen builds many scripts as strings: host provisioning, spawn-host setup, agent git operations, and SSH commands. Any value that a user, task, patch, project, or distro editor can influence becomes code if it reaches a shell unquoted.

## Rules

### 1. Quote every interpolated value, or don't use a shell

- POSIX shell: wrap each value in `util.ShellQuote(v)`, and leave the quotes out of the format string.
- PowerShell: `util.PowerShellQuotedString(v)`.
- Better still, pass an argv slice to `exec.Command` or jasper `.Add([]string{...})` and skip the shell.
- Putting a value inside `'...'` or `"..."` in the format string is **not** escaping. A quote, `$(...)`, a backtick, or a newline in the value breaks out. `strconv.Quote` is not shell escaping either: `$(...)` still expands inside double quotes.
- Content that should be written to a file on a host can be base64-encoded and decoded on the host, so no shell parsing touches it.

```go
// BAD
script := fmt.Sprintf("ls '%s'", dir)
script := fmt.Sprintf("wc -l \"%s\"", name)

// GOOD
script := fmt.Sprintf("ls %s", util.ShellQuote(dir))
script := fmt.Sprintf("wc -l %s", util.ShellQuote(name))
```

Values that look harmless are still attacker-controlled: names, refs, hashes, paths, keys, and any field a non-admin can edit.

### 2. Arguments must not be parseable as options

- Put `--` before positional arguments to `git`, `ssh`, `scp`, and `rsync` when the value isn't constant.
- Reject values starting with `-` where a ref, path, or host is expected.
- Build SSH `-o` options only from a fixed allowlist of keys with validated values.
- Don't pass non-constant input through an intermediate shell such as `cmd.exe`.

### 3. Validate at the boundary too

Where a value has a known shape, validate it against that shape when it's received or stored, in addition to quoting where it's used. Quoting at use is still required.

### 4. Commands run where they run

A command built on the app server runs with the server's credentials. A command in a host setup script may run as root before the task starts. A command in the agent runs on a shared host and can affect later tasks. Be most careful with server-side and setup-time commands.

## Tests

Include values with `'`, `"`, `$(id)`, a backtick, a newline, and a leading `-`. Assert they end up as one literal argument.
