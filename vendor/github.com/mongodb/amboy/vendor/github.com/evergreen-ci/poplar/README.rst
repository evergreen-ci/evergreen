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

Documentation
-------------

See the `godoc <https://godoc.org/github.com/evergreen-ci/poplar/>`_
for complete documentation.
