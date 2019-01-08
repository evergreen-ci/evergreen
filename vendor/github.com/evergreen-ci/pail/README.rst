===========================================
``pail`` -- Blob Storage System Abstraction
===========================================

Overview
--------

Pail is a high-level Go interface to blob storage containers like AWS's
S3 and similar services. Pail also provides implementation backed by
local file systems or MongoDB's GridFS for testing and different kinds
of applications.

Documentation
-------------

The core API documentation is in the `godoc
<https://godoc.org/github.com/evergreen-ci/pail/>`_.

Contribute
----------

Open tickets in the `MAKE project <http://jira.mongodb.org/browse/MAKE>`_, and
feel free to open pull requests here.

Development
-----------

The pail project uses a ``makefile`` to coordinate testing. Use the following
command to build the cedar binary: ::

  make build

The artifact is at ``build/pail``. The makefile provides the following
targets:

``test``
   Runs all tests, sequentially, for all packages.

``test-<package>``
   Runs all tests for a specific package

``race``, ``race-<package>``
   As with their ``test`` counterpart, these targets run tests with
   the race detector enabled.

``lint``, ``lint-<package>``
   Installs and runs the ``gometaliter`` with appropriate settings to
   lint the project.
