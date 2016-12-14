# Evergreen
Version 0.9.0 Alpha

Evergreen is a distributed continuous integration system built by MongoDB.
It dynamically allocates hosts (via AWS, digitalocean, etc) to run tasks in parallel across many machines at once to decrease the total amount of time needed to complete a test workload.

Using Evergreen, we've significantly enhanced the productivity of our engineers.

# Features

#### Elastic Host Allocation
Use only the computing resources you need.

#### Clean UI
Easily navigate the state of your tests, logs, and commit history.

#### Multiplatform Support
Run jobs on Linux (including PowerPC and ZSeries), Windows, OSX, Solaris, and BSD.

#### Spawn Hosts
Spin up a copy of any machine in your test infrastructure for debugging.

#### Patch Builds
See test results for your code changes before committing.

#### Stepback on Failure
Automatically run past commits to pinpoint the origin of a test failure

See [the documentation](https://github.com/evergreen-ci/evergreen/wiki) for a full feature list!

# Usage
Evergreen requires the configuration of a main server along with a cloud provider or static servers.
Please refer to [our tutorial](https://github.com/evergreen-ci/evergreen/wiki/Getting-Started) for full installation instructions.

## System Requirements
The Evergreen Agent and Command Line Tool are supported on Linux, OSX, Windows, and Solaris operating systems.
However, the Evergreen API Server, UI Server, and Runner program are currently only supported and tested on Linux and OSX.

## Go Requirements
* [Install Go 1.7 or later](https://golang.org/dl/).

## Vendoring Dependencies
Our dependencies live in the `vendor` subdirectory of the repo, and
are managed using [glide](https://github.com/Masterminds/glide). To
add a new dependency, add the package to `glide.yaml` and the package
name and revision to `glide.lock`, and run `glide install -s`. To
refresh the entire `vendor` tree, run `make vendor-sync`

## Building the Binaries

Setup:

* ensure that your `GOPATH` environment variable is set.
* check out a copy of the repo into your gopath. You can use: `go get
  github.com/evergreen-ci/evergreen`. If you have an existing checkout
  of the evergreen repository that is not in
  `$GOPATH/src/github.com/evergreen-ci/` move or create a symlink.
* run `make vendor` to set up the vendoring environment.

Possible Targets:

* run `make build` to compile all server binaries for your local
  system.
* run `make agent cli` to compile the agent and cli for your local
  platform.
* run `make agents clis` to compile and cross compile all agent and
  command line interface binaries.
* run `make dist` to compile all server, commandline, and agent
  binaries and create a *dist* tarball with all artifacts.

## Terminology
* `distro`: A platform type (e.g. Windows or OSX) plus large-scale architectural details.  One of the targets that we produce executables for.
* `host`: One of the machines used to run tasks on; typically an instance of a distro that does the actual work (see `task`). Equivalently, any machine in our system other than MOTU.
* `revision`: A revision SHA; e.g. ad8a1d4678122bada9d4479b114cf68d39c27724.
* `project`: A branch in a given repository. e.g. master in the mongodb/mongo repository
* `version`: (`project` + `revision`).  A version of a `project`.
* `buildvariant`: `build` specific information.
* `build`: (`version` + `buildvariant`) = (`project` + `revision` + `buildvariant`).
* `task`: “compile”, “test”, or “push”.  The kinds of things that we want to do on a `host`.

## Running Evergreen Processes
A single configuration file drives most of how Evergreen works. Each Evergreen process must be supplied this settings file to run properly.
For development, tweak the sample configuration file [here](https://github.com/evergreen-ci/evergreen/blob/master/docs/evg_example_config.yml).

All Evergreen programs accept a configuration file with the `-conf` flag.

## How It Works
 * A MongoDB server must be already running on the port specified in the configuration file.
 * Both the API and UI server processes are started manually and listen for connections.
 * All other processes are run by the runner process.
 * For detailed instructions, please see the [wiki](https://github.com/evergreen-ci/evergreen/wiki).
