================================================
``amboy`` -- Task and Worker Pool Infrastructure
================================================

Overview
--------

Amboy is a collection of interfaces and tools for running and managing
asynchronous background work queues in the context of Go programs, and
provides a number of interchangeable and robust methods for running
tasks.

Features
--------

Queues
~~~~~~

Queue implementations impose ordering and dispatching behavior, and
describe the storage of tasks before and after work is
complete. Current queue implementations include:

- an ordered queue that dispatches tasks ordered by dependency
  information to ensure that dependent tasks that are completed before
  the tasks that depend on them.

- an unordered queue that ignores dependency information in tasks. For
  most basic cases these queues are ideal. (`LocalUnordered
  <https://godoc.org/github.com/mongodb/amboy/queue#LocalUnordered>`_
  as implementation detail this queue dispatches tasks in a FIFO order.)

- a limited size queue that keep a fixed number of completed jobs in
  memory, which is ideal for long-running background processes.

- priority queues that dispatch tasks according to priority order.

- remote queues that store all tasks in an external storage system
  (e.g. a database) to support architectures where multiple processes
  can service the same underlying queue.

Queue Groups
~~~~~~~~~~~~

The `QueueGroup <https://godoc.org/github.com/mongodb/amboy#QueueGroup>`_
interface provides a mechanism to manage collections of queues. There are remote
and local versions of the queue group possible, but these groups make it
possible to create new queues at runtime, and improve the isolation of queues
from each other.

Retryable Queues
~~~~~~~~~~~~~~~~

The `RetryableQueue
<https://godoc.org/github.com/mongodb/amboy#RetryableQueue>_` interface provides
a superset of the queue functionality. Along with regular queue operations, it
also supports jobs that can retry. When a job finishes executing and needs to
retry (e.g. due to a transient error), the retryable queue will automatically
re-run the job.

Runners
~~~~~~~

Runners are the execution component of the worker pool, and are
embedded within the queues, and can be injected at run time before
starting the queue pool. The `LocalWorkers
<https://godoc.org/github.com/mongodb/amboy/pool#LocalWorkers>`_
implementation executes tasks in a fixed-size worker pool, which is
the default of most queue implementations.

Additional implementation provide rate limiting, and it would be possible to
implement runners which used the REST interface to distribute workers to a
larger pool of processes, where existing runners simply use go routines.

Dependencies
~~~~~~~~~~~~

The `DependencyManager
<https://godoc.org/github.com/mongodb/amboy/dependency#Manager>`_
interface makes it possible for tasks to express relationships to each
other and to their environment so that Job operations can noop or
block if their requirements are not satisfied. The data about
relationships between jobs can inform task ordering as in the `LocalOrdered
<https://godoc.org/github.com/mongodb/amboy/queue#LocalOrdered>`_
queue.

The handling of dependency information is the responsibility of the
queue implementation.

Management
~~~~~~~~~~

The `management package
<https://godoc.org/github.com/mongodb/amboy/management>`_ centers around a
`management interface
<https://godoc.org/github.com/mongodb/amboy/management#Manager>`_ that provides
methods for reporting and safely interacting with the state of jobs.

REST Interface
~~~~~~~~~~~~~~

The REST interface provides tools to manage jobs in an Amboy queue provided as a
service. The rest package in Amboy provides the tools to build clients and
services, although any client that can construct JSON-formatted Job object can
use the REST API.

Additionally the REST package provides remote implementations of the `management
interface <https://godoc.org/github.com/mongodb/amboy/rest#ManagementService>`_
which makes it possible to manage and report on the jobs in an existing queue,
and the `abortable pool
<https://godoc.org/github.com/mongodb/amboy/rest#AbortablePoolManagementService>`_
interface, that makes it possible to abort running jobs. These management tools
can help administrators of larger amboy systems gain insights into the current
behavior of the system, and promote safe and gentle operational interventions.

See the documentation of the `REST package
<https://godoc.org/github.com/mongodb/amboy/rest>`_

Logger
~~~~~~

The Logger package provides amboy.Queue backed implementation of the grip
logging system's sender interface for asynchronous log message delivery. These
jobs do not support remote-backed queues.

Patterns
--------

The following patterns have emerged during our use of Amboy.

Base Job
~~~~~~~~

Embed the `job.Base <https://godoc.org/github.com/mongodb/amboy/job/#Base>`_
type in your Job implementations. This provides a number of helpers for
basic job definition, in addition to implementations of all general methods in
the interface. With the Base, you only need to implement a ``Run()`` method and
whatever application logic is required for the task.

The only case where embedding the Base type *may* be contraindicated is in
conjunction with the REST interface, as the Base type may require more
complicated initialization processes.

Change Queue Implementations for Different Deployment Architectures
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If your core application operations are implemented in terms of Jobs, then
you can: execute them independently of queues by calling the ``Run()`` method,
use a locally backed queue for synchronous operation for short running queues,
and use a limited size queue or remote-backed queue as part of a long running
service.

Please submit pull requests or `issues <https://github.com/mongodb/amboy>`_ with
additional examples of amboy use.

API and Documentation
---------------------

See the `godoc API documentation <https://godoc.org/github.com/mongodb/amboy>`
for more information about amboy interfaces and internals.

Development
-----------

Getting Started
~~~~~~~~~~~~~~~

Currently amboy vendors all of its dependencies, as a result of an upstream
requirement to build on go1.9; however, eventually the project will move to use
modules. For the time being, have a ``GOPATH`` set, and ensure that you check
out the repository into ``$GOPATH/src/github/mongodb/amboy``.

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

Issues
~~~~~~

Please file all issues in the `EVG project
<https://jira.mongodb.org/browse/EVG>`_ in the `MongoDB Jira
<https://jira.mongodb.org/>`_ instance.
