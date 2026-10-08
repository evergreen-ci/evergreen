# Agent, Host, and Credential Endpoints

Agent and host routes authenticate a machine, not a user. The middleware validates one task or host (by path variable or header plus secret). Anything the handler does must stay within that identity.

## Rules

### 1. Use the authenticated identity, not a caller-supplied ID

- In handlers behind `requireTask`, get the task with `MustHaveTask(ctx)`. Behind `requireHost`, use `MustHaveHost(ctx)`.
- Ignore task, host, or pod IDs in the JSON body, query string, or headers. If the body must carry one (for compatibility), reject the request when it doesn't match the authenticated identity.
- Take identity only from the values the auth middleware puts in the request context.
- Execution numbers count as identity too: match on ID **and** the authenticated execution so an old execution can't overwrite a newer one.
- Referenced objects need binding too. Any object ID supplied by the agent must belong to the authenticated task's version or project.

```go
// BAD: the agent for task A can write task B's results.
t, err := task.FindOneId(ctx, body.TaskID)

// GOOD
t := MustHaveTask(ctx)
if body.TaskID != "" && body.TaskID != t.Id {
    return gimlet.MakeJSONErrorResponder(errors.New("task ID does not match authenticated task"))
}
```

### 2. Dispatch and assignment must respect project boundaries

Code that hands a task, or a task's data, to a host must confirm that the host, its owner, and its distro are all allowed to access that task's project. A task's secret unlocks that project's variables.

### 3. Credentials are minted for the task's own project, never for a requested target

When minting a GitHub App installation token, AWS STS session, presigned S3 URL, or similar credential:

- Derive the target from the authenticated task's project ref and trusted project config. A request may narrow it, never widen it.
- If the request names a target, check it against the project's allowed set and return 403 on mismatch. There is no fallback to a broader identity.
- Compute every identifier used in a credential decision on the server, from values the caller cannot choose.
- Admin-only credentials (admin-only variables, privileged signing keys) must not be usable by tasks unless they are explicitly authorized for them.

### 4. Secrets are compared in constant time

Compare task secrets, host secrets, setup secrets, and tokens with `subtle.ConstantTimeCompare` (or `hmac.Equal` for MACs), never `==` or `!=`.

## Tests

- A request authenticated as task A that names task B (body, query, header) is rejected or ignored.
- A token request for a repo outside the task's project is refused.
- Dispatch to a host owned by another user or project is refused.
