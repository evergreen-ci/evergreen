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

## Go Requirements

- [Install Go 1.16 or later](https://golang.org/dl/).
- This project uses Go modules.

## Building the Binaries

Setup:

- If you're going to use the makefile, set `GOMODCACHE` (you can just set it to
  the output of `go env GOMODCACHE`, unless you want it somewhere else).
- check out a copy of the repo into your gopath. You can use:
  `go get github.com/evergreen-ci/evergreen` or just
  `git clone https://github.com/evergreen-ci/evergreen`.

Possible Targets:

- run `make build` to compile a binary for your local system.
- run `make dist` to compile binaries for all supported systems and create a
  _dist_ tarball with all artifacts.
- run `make local-evergreen` to start a local Evergreen. You will need a mongod
  running, listening on 27017. Log in at http://localhost:9090/login with user
  `admin` and password `password`. Visiting http://localhost:9090/ should show
  redirect you the waterfall on the new UI. The new UI is available at
  https://github.com/evergreen-ci/ui. You need to run the UI server separately
  in order to view the new UI.
