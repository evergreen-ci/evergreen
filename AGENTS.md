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

### Code Readability and Comments
* Write self-documenting code through clear naming and structure (such as helper functions for modularity).
* Inline code comments should be full sentences and express complete thoughts. Use proper grammar, punctuation, and
  capitalization.
* Inline code comments should be used intentionally. Do not write a comment if it just explains exactly what the code is
  doing. However, they can be used if they will explain the "why", such as context, intent, or non-obvious behavior.

**Good use of comments:**
* Documentation for exported structs, functions, fields, and methods.
* Explaining broader context that can't be understood easily from the immediate implementation.
* Explaining why the code has to do something non-obvious.
* Explaining unusual but necessary implementation decisions.

**Avoid:**
* Comments that simply restate what the code is doing.
* Redundant comments that add clutter or maintenance burden.

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

## Pull Requests

For opening PRs, follow the template in `.github/pull_request_template.md`:

**Title format:**
* Prefix with Jira ticket: `DEVPROD-XXXX: Description of change`
    * Example: `DEVPROD-1234: Add user authentication`
* Include the Jira ticket in the placeholder on the first line of the description.
