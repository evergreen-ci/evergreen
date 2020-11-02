=============================================
``poplar`` -- Golang Performance Test Toolkit
=============================================

Overview
--------

Poplar is a set of tools for running and recording results for
performance tests suites and benchmarks. It provides easy integration
with a number of loosely related tools:

- `ftdc <https://github.com/mongodb/ftdc>`_ is a compression format for
  structured and semi-structured timeseries data. Poplar provides
  service integration for generating these data payloads.

- `cedar <https://github.com/evergreen-ci/cedar>`_ is a service for
  collecting and processing data from builds. Poplar provides a client
  for uploading test results to cedar from static YAML or JSON data.

Additionally, poplar provides a complete benchmark test harness with
integration for collecting ftdc data and sending that data to cedar,
or reporting it externally.

Some popular functionality is included in the `curator
<https://github.com/mongodb/curator>`_ tool, as ``curator poplar``.

Development
-----------

Currently poplar vendors all of its dependencies, as a result of an upstream
requirement to build on go1.9; however, eventually the project will move to use
modules. For the time being, have a ``GOPATH`` set, and ensure that you check
out the repository into ``$GOPATH/src/github/evergreen-ci/poplar``.

All project automation is managed by a makefile, with all output captured in the
`build` directory. Consider the following operations: ::

   make build                   # runs a test compile
   make test                    # tests all packages
   make lint                    # lints all packages
   make test-<package>          # runs the tests only for a specific packages
   make lint-<package>          # lints a specific package
   make html-coverage-<package> # generates the coverage report for a specific package
   make coverage-html           # generates the coverage report for all packages

The buildsystem also has a number of flags, which may be useful for more
iterative development workflows: ::

  RUN_TEST=<TestName>   # specify a test name or regex to run a subset of tests
  RUN_COUNT=<num>       # run a test more than once to isolate an intermittent failure
  RACE_DETECTOR=true    # run specified tests with the race detector enabled. 


Documentation
-------------

See the `godoc <https://godoc.org/github.com/evergreen-ci/poplar/>`_
for complete documentation.
