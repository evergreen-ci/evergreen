# Evergreen

## Build

```bash
make build        # builds the CLI binary
```

Do NOT use `go build ./...` directly. Use the Makefile.

## Test

```bash
make test-<package>              # run tests for a package
make test-validator              # example: validator package
make test-operations             # example: operations package
RUN_TEST=TestFoo make test-bar   # run a specific test
RACE_DETECTOR=1 make test-bar    # run with race detector
```

Package names use hyphens for nested paths (e.g., `rest-route` for `rest/route`, `model-host` for `model/host`). Run `make list-tests` to see all available test targets.

## Lint

```bash
make lint-<package>   # lint a specific package
make lint             # lint all packages
```
