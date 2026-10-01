See [the docs](https://docs.devprod.prod.corp.mongodb.com/evergreen/Home/) for
user-facing documentation, or
[in the repo](https://github.com/evergreen-ci/evergreen/tree/main/docs/) if you
don't have access to internal MongoDB sites.

See [the API docs](https://pkg.go.dev/github.com/evergreen-ci/evergreen) for
developer documentation. For an overview of the architecture, see the list of
directories and their descriptions at the bottom of that page.

# Features

#### Elastic Host Allocation

Use only the computing resources you need.

#### Clean UI

Easily navigate the state of your tests, logs, and commit history.

#### Multiplatform Support

Run jobs on any platform Go can cross-compile to.

#### Spawn Hosts

Spin up a copy of any machine in your test infrastructure for debugging.

#### Patch Builds

See test results for your code changes before committing.

#### Stepback on Failure

Automatically run past commits to pinpoint the origin of a test failure.

## Build Requirements

- Install Git, Make, Bash, curl, tar, and either `sha256sum` or `shasum`.
  Windows builds also require a Unix shell environment with `cygpath` and `unzip`.
- A system Go installation is not required. The Makefile downloads the exact Go
  version from `go.mod`, verifies its SHA-256 checksum, and caches it under
  `bin/go-sdk/`. Downloads support Linux, macOS, and Windows hosts.
- Builds ignore the system Go installation, inherited `GOROOT` and
  `GOTOOLCHAIN`, and persistent `go env -w` settings. Automatic toolchain
  switching is disabled. Cross-compilation still accepts `GOOS` and `GOARCH`.
- The first build requires network access for the SDK and Go modules. Later
  builds reuse their caches. C/C++ compilers are still required for cgo and
  race-detector builds.

## Building the Binaries

Setup:

- Clone the repository with `git clone https://github.com/evergreen-ci/evergreen`
  and change into its directory.
- Optionally set `GOMODCACHE` and `GOCACHE` to share existing caches. The defaults
  are `bin/.mod-cache` and `bin/.cache`.

Possible Targets:

- run `make build` to compile a binary for your local system.
- run `make go-sdk` to download the SDK without building, or
  `bash scripts/go-sdk.sh go version` to run Go directly with the pinned SDK.
- run `make local-evergreen` to start a local Evergreen. You will need a mongod
  running, listening on 27017. To run the UI locally, see [Spruce's README](https://github.com/evergreen-ci/ui/tree/main/apps/spruce#running-locally).

To upgrade the build SDK, update the `go` directive in `go.mod` and the archive
checksums in `scripts/go-sdk-checksums.txt` using the official
[Go release metadata](https://go.dev/dl/?mode=json&include=all). Each supported
host needs a checksum; an unpinned archive is rejected. `make modernize` uses a
separate Go 1.26.0 SDK pinned in `scripts/go-sdk.sh` because its analyzer requires
a newer Go release.
