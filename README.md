# Mongo Continuous Integration (MCI)

## <a name="requirements"></a>Go Requirements
 * [Install Go from source](http://golang.org/doc/install/source). This is needed to build the binaries using goxc.

## Vendored dependencies
Our dependencies live in the `vendor` subdirectory of the repo. The specifications for what version of each dependency
is used resides in the Godeps file in the root of the repo. If you add a new dependency, or change the version of
an existing one, run the `vendor.sh` script to refresh the downloaded versions in `vendor`.

## Building the binaries
* To build the binaries for the cron tasks and servers, run the `build.sh` script in the root of the repo. This will
install the binaries into the `bin` subdirectory.
* To build the agent binary, run the `build_agent.sh` script in the root of the repo. This will use goxc (located in
vendor/src/github.com/laher/goxc) to cross-compile the agent binary into the `executables` subdirectory. Before doing
this, you have to run `go run vendor/src/github.com/laher/goxc/goxc.go -t` once on the machine where you are building 
the binaries.

## Patches to our vendored libraries
* Patches: See https://wiki.mongodb.com/display/KERNEL/MCI+Internal+Patches

## Terminology
* `distro`: A platform type (e.g. Windows or OSX) plus large-scale architectural details.  One of the targets that we produce executables for.
* `host`: One of the machines used to run tasks on; typically an instance of a distro that does the actual work (see `task`). Equivalently, any machine in our system other than MOTU.
* `MOTU`: “Master of the Universe”; the machine which runs the ruling MCI programs. It also has the main database repository and runs the webapp.
* `revision`: A revision SHA; e.g. ad8a1d4678122bada9d4479b114cf68d39c27724.
* `branch`: A branch in a given repository. e.g. master in the mongodb/mongo repository
* `version`: (`branch` + `revision`).  A version of a `branch`.
* `buildvariant`: `build` specific information.
* `build`: (`version` + `buildvariant`) = (`branch` + `revision` + `buildvariant`).
* `task`: “compile”, “test”, or “push”.  The kinds of things that we want to do on a `host`.

## Running MCI Processes
A single configuration (settings) file drives most of how MCI works. Each MCI process must be supplied this settings file to run properly. For development, tweak the sample configuration file [here](https://github.com/10gen/mci/blob/master/config_test/mci_settings.yml).

## How it Works
 * Both the API and UI server processes are started using the commands above (a mongod must be already running on the port specified in the configuration file).
 * All other processes are run on a cronjob on MOTU. See the [runner.sh](https://github.com/10gen/mci/blob/master/scripts/runner.sh) for more details.
 * For more information, please see the [wiki](https://wiki.mongodb.com/display/10GEN/MCI+Overview) and check out the presentation [slides](https://docs.google.com/a/10gen.com/presentation/d/1qu3Md5iTe-H3kaRCXn_OSRniS70Xqc6X3p9Y5iH8vKU) and [video](https://docs.google.com/a/10gen.com/file/d/0B1ZYxJrE0pbha1VQR3ZrT2lmOTQ/edit).

