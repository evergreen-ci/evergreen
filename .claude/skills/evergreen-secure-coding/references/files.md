# Files and Paths

Untrusted inputs put attacker-chosen names and links on disk. The app server, the agent, and the CLI then read and write those trees.

## Rules

### 1. Contain every path built from untrusted data

Untrusted path segments include any name that came from a user, task, patch, archive, configuration, or API response.

- Reject absolute paths and `..` traversal: `filepath.IsLocal(rel)`.
- After joining, verify the result stays under the base: `rel, err := filepath.Rel(base, joined)`, then reject when `rel` starts with `..`.
- Reuse the package's existing containment helper if there is one. Don't write a new string-prefix check, because `HasPrefix(p, base)` accepts `/base-evil`.
- This applies to client code too: names received from a server are untrusted.

### 2. Don't follow links out of untrusted trees

- Before reading a file from any tree that untrusted code or data could have written, `os.Lstat` it and reject `os.ModeSymlink`. Or resolve it with `filepath.EvalSymlinks` and re-check containment.
- Watch for hard links and special files when copying, archiving, or uploading.
- Extract archives entry by entry, validating each name and link target.

### 3. Servers don't read their own filesystem on behalf of requests

Request-serving code (REST, `service/`, GraphQL) must not read the server's local filesystem on behalf of a caller. Error messages must not echo file contents.

### 4. Shared storage keys need separators

When matching stored objects by an ID prefix, end the prefix with a separator. Scope shared entries by an identifier the caller cannot choose or claim.

## Tests

Use `../x`, `/etc/passwd`, `a/../../x`, a symlink to `/`, and a name with an embedded separator. Assert that nothing is created or read outside the base.
