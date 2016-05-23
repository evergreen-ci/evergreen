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
 * [Install Go 1.6 or later](https://golang.org/dl/).

## Vendoring Dependencies
Our dependencies live in the `vendor` subdirectory of the repo.
The specifications for what version of each dependency is used resides in the Godeps file in the root of the repo.
If you add a new dependency, or change the version of an existing one, run the `vendor.sh` script to refresh the downloaded versions in `vendor`.

## Building the Binaries
* Run the `build.sh` script in the root of the repo to generate binaries (in the `bin/` subdirectory) for the UI server, API server, and runner daemon.
* To build the agent binaries for all platforms, run the `build_agent_gc.sh` script in the root of the repo. 
* To run tests or build manually, you must update the GOPATH variable by running `. ./setgopath.sh` in your shell.

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
