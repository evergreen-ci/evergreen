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

Remote Queues
~~~~~~~~~~~~~

Currently amboy has a single remote-backed queue implementation. This
implementation implements an unordered queue, backed by a pluggable `Driver
<https://godoc.org/github.com/mongodb/amboy/queue/driver#Driver>`_
implementation.

Amboy currently provides several different driver implementations,
with different semantics. Some driver implementations do use local
storage.

- MongoDB storage (tasks are dispatched in either a non-specified
  order *or* in priority order, depending on configuration.)

- capped results storage (tasks are dispatched in insertion order, and
  a specified number of results are retained in completion order.)

- priority queue (tasks are dispatched in priority ordering of tasks.)

- internal (tasks are dispatched in a randomized order.)

Users can inject any Driver interface, or implement their own. While
the Driver is quite straightforward, all Drivers require a
compatibile `Lock
<https://godoc.org/github.com/mongodb/amboy/queue/driver#JobLock>`_
implementation for synchronizing queue operations between multiple
processes backed by the same queue.

In general, for "remote" queues backed by drivers that use local
storage, the direct-local queue implementations are more efficient
because they require less aggressive locking.

The Amboy queue system should be able to support additional remote
storage systems and additional support for stronger ordering
constraints.

Runners
~~~~~~~

Runners are the execution component of the worker pool, and are
embedded within the queues, and can be injected at run time before
starting the queue pool. The `LocalWorkers
<https://godoc.org/github.com/mongodb/amboy/pool#LocalWorkers>`_
implementation executes tasks in a fixed-size worker pool, which is
the default of most queue implementations.

The runner interface can be used to manage execution of tasks on
remote machines or dispatch tasks to alternate queuing systems.

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

REST Interface
~~~~~~~~~~~~~~

The REST interface provides tools to submit jobs to an Amboy queue
provided as a service. The rest package in Amboy provides the tools to
build clients and services, although any client that can construct
JSON formated Job object can use the REST API.

See the documentation of the `REST package
<https://godoc.org/github.com/mongodb/amboy/rest>`_

Logger
~~~~~~

The Logger package provides amboy.Queue backed implementation of the
grip logging system's sender interface for asynchronous log message
delivery. These jobs do not support remote-backed queues.

Patterns
--------

The following patterns have emerged during our use of Amboy.

Base Job
~~~~~~~~

Embed the `job.Base
<https://godoc.org/github.com/mongodb/amboy/job/#Base>`_
type in your amboy.Job implementations. This provides a number of
helpers for basic job defintion in addition to implementations of all
general methods in the interface. With the Base, you only need to
implement a ``Run()`` method and whatever application logic is required
for the task.

The only case where embedding the Base type *may* be contraindicated is
in conjunction with the REST interface, as the Base type may require
more complicated initialization processes.

Change Queue Implementations for Different Deployment Architectures
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If your core application operations are implemented in terms of
amboy.Jobs, then you can: execute them independently of queues by
calling the ``Run()`` method, use a locally backed queue for
synchronous operation for short running queues, and use a limited size
queue or remote-backed queue as part of a long running service.

Examples
--------

- `curator <https://github.com/mongodb/curator>`_ uses amboy to
  support the file sync operation as part of the `sthree (s3)
  <http://godoc.org/github.com/mongodb/curator/sthree>`_
  package. Additionally, the main `repobuilder operation (Job)
  <http://godoc.org/github.com/mongodb/curator/repobuilder>`_
  operation is implemented in terms of an amboy.Job instance but
  executed directly to support alternate deployments as needs change.

- All checks in the `greenbay <https://github.com/mongodb/greenbay>`_
  tool implement an interface that is a super-set of the Job
  interface and executed in a local queue.

Please submit pull requests or `issues
<https://github.com/mongodb/amboy>`_ with additional examples of amboy
use.

API and Documentation
---------------------

See the `godoc API documentation
<http://godoc.org/github.com/mongodb/amboy>` for more information
about amboy interfaces and internals.

Development
-----------

Please file all issues in the `MAKE project
<https://jira.mongodb.org/browse/MAKE>`_ in the `MongoDB Jira
<https://jira.mongodb.org/>`_ instance.
