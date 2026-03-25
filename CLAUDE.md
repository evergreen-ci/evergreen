# Evergreen

## Building

```
make build
```

Compiles all components for the local architecture. Does not cross-compile or compile tests.

## Testing

```
make test-<package>
```

Package names use dashes instead of slashes. Examples:
- `make test-cloud` for `cloud/`
- `make test-model-host` for `model/host/`
- `make test-rest-model` for `rest/model/`
- `make test-rest-data` for `rest/data/`
- `make test-rest-route` for `rest/route/`
- `make test-agent-command` for `agent/command/` (requires compiled Evergreen binary)

## Linting

```
make lint-<package>
```

Same dash-separated naming as test targets.

## Self-tests

Evergreen tests itself via `self-tests.yml`. There is roughly a 1:1 relationship between Makefile targets and Evergreen self-test tasks.

Tests require a local MongoDB replica set. See the dev guide (go/evergreen-dev-guide) for setup instructions.
