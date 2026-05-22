# Evergreen Codebase
## Overview

Evergreen is MongoDB's continuous integration. It runs tasks on a mix of static hosts and dynamically provisioned EC2
hosts across multiple platforms. The single binary (`cmd/evergreen/evergreen.go`) serves as the CLI, application server,
and agent for running tasks.

## Building and Testing

Evergreen uses `make` for building/testing/linting. Read the makefile as needed for further available operations.

### Building

```bash
make build          # Compile binary for local system (→ clients/<goos>_<goarch>/evergreen)
make gqlgen         # Regenerate GraphQL code after schema changes
make mod-tidy       # Tidy Go module dependencies
```

### Testing

```bash
# Run all tests for a package (dashes map to path separators: agent-command → ./agent/command)
make test-<package>   # e.g., make test-model, make test-rest-route

# Run all tests in the top-level package (special case).
make test-evergreen

# Run specific tests by regexp pattern.
make test-<package> RUN_TEST=<pattern>

# Provide an explicit admin settings file. Typically needed for integration tests involving third-party services like
# EC2, GitHub, and GraphQL.
SETTINGS_OVERRIDE=<file> make test-thirdparty

# Useful test flags via environment variables
RUN_TEST=<pattern>  # Run specific test names by regexp patterns.
SKIP_LONG=1         # Skip long-running tests.
RACE_DETECTOR=1     # Enable race detector.
TEST_TIMEOUT=5m     # Override default 10m timeout.
RUN_COUNT=5         # Run tests N times.
```

### Linting

```bash
make lint            # Lint all packages.
make lint-<package>  # Lint a specific package.
make lint-evergreen  # Lint the top-level evergreen package (special case).
```

After making changes, run `make lint-<package>` for each affected package and verify there are no new errors beyond any pre-existing golangci-lint toolchain crashes.

Adding a `ServiceFlags` field requires running `make test-service-graphql`.

### CLI Changes

Whenever modifying the `operations/` package (CLI commands), increment `ClientVersion` in `config.go`.
The format is the calendar date (`YYYY-MM-DD`); append a letter suffix (e.g. `2026-05-20a`) if there
are multiple changes on the same day.

### CI Self-Tests

The Evergreen codebase has automated tests defined in `self-tests.yml`, which itself runs in Evergreen. For most tasks in
`self-tests.yml`, there's a roughly 1:1 relationship between Makefile targets and Evergreen self-test tasks.

## Architecture

### Package Organization

| Package        | Description                                                                                               |
|----------------|-----------------------------------------------------------------------------------------------------------|
| `agent/`       | Contains the logic to run the agent. Runs tasks on hosts; receives tasks to run from the app server.      |
| `apimodels/`   | Shared models used across REST API boundaries for the agent.                                              |
| `auth/`        | Authentication backends (naive, GitHub, Okta).                                                            |
| `cloud/`       | Cloud provider integrations (AWS EC2, Docker, etc).                                                       |
| `db/`          | MongoDB interaction layer.                                                                                |
| `graphql/`     | GraphQL backend.                                                                                          |
| `model/`       | Core data models; task, host, build, patch, distro, etc. are in subpackages.                              |
| `operations/`  | CLI command implementations. Also provides an entry point for the app server/agent.                       |
| `repotracker/` | Tracks GitHub repos for new commits and PRs.                                                              |
| `rest/`        | REST API: `route/` (handlers), `data/` (data connectors), `model/` (API models), `client/` (CLI clients). |
| `scheduler/`   | Orders tasks in distro queues.                                                                            |
| `service/`     | HTTP server wiring; UI and legacy REST endpoints.                                                         |
| `thirdparty/`  | Other third-party integrations (GitHub, Jira, etc).                                                       |
| `trigger/`     | Logic to trigger notifications from CI system events.                                                     |
| `units/`       | Background job definitions using the Amboy job framework.                                                 |
| `util/`        | Common utility functions.                                                                                 |
| `validator/`   | Validates project YAML configs and distro settings.                                                       |

## Go Coding Conventions

The conventions below are a distilled, agent-readable summary of the team's full style guide,
[Evergreen Go Coding Conventions](https://docs.google.com/document/d/1-DB1FQ_zb5fX_0pnVcwyU0HmJPrloFBu6qSE8dkk8PU)
(internal). The Google Doc is the canonical source for humans; this file is the canonical source for AI coding agents.
When the doc is updated, please mirror the change here in the same PR so agents stay aligned with the team's
expectations.

### Naming
* Use PascalCase (e.g. `MyExportedFunc`) for exported names (variables, constants, structs, functions, methods). Use
  camelCase (e.g. `myVariable`) for everything else.
* Acronyms and proper names should be cased consistently with its actual name (e.g. GitHub instead of Github) while
  still following PascalCase/camelCase rules.

**Good naming examples:**
`UserID`
`makeGitHubApp`
`githubApp`
`awsVariable`

**Bad:**
`makeAwsConfig` (should be `makeAWSConfig`)
`UserId` (should be `UserID`)

### Formatting
* In Go, when a block of variable declarations or struct fields is vertically aligned (e.g., all the type keywords or `=` signs line up), new entries must maintain that alignment within the same block. A blank line or a comment header (e.g., `// Notification Flags`) starts a new block with its own independent alignment; do not align across block boundaries.

### Code Readability and Comments
* Write self-documenting code through clear naming and structure (such as helper functions for modularity).
* Inline code comments should be full sentences and express complete thoughts. Use proper grammar, punctuation, and
  capitalization.
* Inline code comments should be used intentionally. Do not write a comment if it just explains exactly what the code is
  doing. However, they can be used if they will explain the "why", such as context, intent, or non-obvious behavior.
* Documentation comments should focus on high-level information relevant to usage, such as intent, expected
  inputs/outputs, behavior. It is also valid to highlight hazardous gotchas to be careful about, if any. It should not
  describe implementation details; those should be left to inline comments.

**Good use of comments:**
* Documentation for exported structs, functions, fields, and methods.
* Explaining broader context that can't be understood easily from the immediate implementation.
* Explaining why the code has to do something non-obvious.
* Explaining unusual but necessary implementation decisions.

**Avoid:**
* Comments that simply restate what the code is doing.
* Redundant comments that add clutter or maintenance burden.

### Errors
* When adding context to an error with `errors.Wrap`/`errors.Wrapf`, describe the operation that was being performed.
  Do not prefix the wrapped message with noise phrases like "problem", "error", "unable to", or "failed to" — they add
  no information.
* This applies only to wrapping (`errors.Wrap`/`Wrapf`). It does not apply to base error messages (`errors.New`,
  `errors.Errorf`) or to log/Splunk `message` fields.

**Good:**
```go
return errors.Wrapf(err, "creating temporary file '%s'", name)
```

**Bad:**
```go
return errors.Wrapf(err, "unable to create file '%s'", name)
```

### Testing
* Use `PascalCase` for test names for consistency. Do not use spaces, dashes, underscores, or any other character to
  delimit words in test case names.
* Test names should describe both the inputs and expected outputs or behavior (e.g. `NilInputShouldError`, not
  `NilInput`).
* Use `t.Context` for test contexts.
* Only add assertion comments if they help clarify the assertion beyond what can immediately be seen (e.g. testing a
  non-obvious interaction).
* Use testify for testing libraries. Do not add new GoConvey tests or testify test suites. It's okay to add to existing
  test functions if they're already structured with GoConvey or testify test suites.
* Prefer not to use `assert.New`/`require.New` if possible because they're difficult to use for nested tests or test
  case lists.
* Use `t.Cleanup` to do deferred cleanup at the end of tests.
* Use `require` (not `assert`) before any code that would panic if the assertion failed — for example, before
  dereferencing a pointer, indexing a slice, or accessing a field on a value returned from the call under test. A panic
  aborts the whole test binary, which usually hides the real failure that happened earlier.

### Environment Handling
* Do not call `evergreen.GetEnvironment()` inline at the spot where the environment is needed. Instead, accept the
  environment as a parameter or struct field and propagate it down from the top-level entry point (a REST route handler
  or an Amboy job). Amboy jobs typically store the environment as a job field so it can be mocked in tests.
* Inline calls to `evergreen.GetEnvironment()` make the code dependent on global testing state, which is easy to leave
  in a dirty state between tests and hard to isolate.
* Narrow exceptions where inline `evergreen.GetEnvironment()` is acceptable:
    * The call site is so deeply nested that threading the environment through would require refactoring a large
      number of intermediate functions.
    * The environment is only needed to obtain a DB handle for a query (no admin-settings mutation is involved).

### AI-Generated Code
AI-assisted code is welcome, but the author is responsible for the final result. Treat generated code with the same
scrutiny as code you wrote yourself.
* Read and fully understand any generated code before committing it. You should be able to defend its correctness,
  performance, and security as if you'd written it.
* Be skeptical of new external dependencies the AI introduces — adding a library is often a sign that the AI has
  "lost the plot" on a problem that should be solved with existing code. Read the library's docs before adopting it.
* Remove the AI's explanatory comments unless the comment genuinely captures a non-obvious "why". Generated code tends
  to over-comment what the code is already doing.
* Review generated user-facing strings (error messages, logs) for conciseness and helpfulness.
* Generated tests should be meaningful: prune cases that are impossible, redundant, or that test the mock rather than
  the code under test.
* Check for performance anti-patterns the AI commonly introduces (DB calls inside deep loops, redundant queries).

## Pull Requests

For opening PRs, follow the template in `.github/pull_request_template.md`:

**Title format:**
* Prefix with Jira ticket: `DEVPROD-XXXX: Description of change`
    * Example: `DEVPROD-1234: Add user authentication`
* Include the Jira ticket in the placeholder on the first line of the description.
