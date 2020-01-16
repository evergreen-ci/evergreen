# [Spruce](https://evergreen.mongodb.com) &middot; [![GitHub license](https://img.shields.io/badge/license-Apache2.0-blue.svg)](https://github.com/evergreen-ci/evergreen/master/LICENSE)

Evergreen is a distributed continuous integration system built by MongoDB.
It dynamically allocates hosts to run tasks in parallel across many machines.

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

See [the documentation](https://github.com/evergreen-ci/evergreen/wiki) for a full feature list!

## System Requirements

The Evergreen agent, server, and CLI are supported on Linux, macOS, and Windows.

## Go Requirements

- [Install Go 1.9 or later](https://golang.org/dl/).

## Building the Binaries

Setup:

- ensure that your `GOPATH` environment variable is set.
- check out a copy of the repo into your gopath. You can use: `go get github.com/evergreen-ci/evergreen`. If you have an existing checkout
  of the evergreen repository that is not in
  `$GOPATH/src/github.com/evergreen-ci/` move or create a symlink.

Possible Targets:

- run `make build` to compile a binary for your local
  system.
- run `make dist` to compile binaries for all supported systems
  and create a _dist_ tarball with all artifacts.
