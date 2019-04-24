/*
Package amboy provides basic infrastructure for running and describing
tasks and task workflows with, potentially, minimal overhead and
additional complexity.

Overview and Motivation

Amboy works with 4 basic logical objects: jobs, or descriptions of
tasks; runnners, which are responsible for executing tasks; queues,
that represent pipelines and offline workflows of tasks (e.g. not real
time, processes that run outside of the primary execution path of a
program); and dependencies that represent relationships between jobs.

The inspiration for amboy was to be able to provide a unified way to
define and run jobs, that would feel equally "native" for distributed
applications and distributed web application, and move easily between
different architectures.

While amboy users will generally implement their own Job and
dependency implementations, Amboy itself provides several example
Queue implementations, as well as several generic examples and
prototypes of Job and dependency.Manager objects.

Generally speaking you should be able to use included amboy components
to provide the queue and runner components, in conjunction with custom
and generic job and dependency variations.

Consider the following example:

   queue := queue.SimpleQueue(12) // pass the number of worker threads
   job := job.NewShellJob("make compile")

   err := queue.Put(job)
   if err != nil {
      // handle error case
   }

   err = queue.Start(ctx) // the queue starts a SimpleRunner object and
		       // creates required channels.
   if err != nil {
      // handle error case
   }

   Wait(queue) // waits for all tasks to finish.
   queue.Close() // waits for all tasks to finish and releases
		 // all resources.
*/
package amboy

// This file is intentionally documentation only.

// The following content is intentionally excluded from godoc, but is
// a reference for maintainers.

/*
Code Organization

For the most part, the amboy package itself contains a few basic types
and interfaces, and then several sub-packages are responsible for
providing implementations and infrastructure to support these systems
and interactions. The sub-package are quite small and intentionally
isolated to make it easier to test and also avoid unintentional
dependencies between the implementations of various components.

Consider the following component packages:

Registry

The registry provides a way to declare job and dependency types so
that Queue implementations, as well as the job.Group implementation,
can persist job object generically.

Pool

Contains implementations of a Queue-compatible worker pool
(i.e. Runners). Intentionally, runner implementations are naive and
simple so there's less useful variation.

Job

Provides several generically useful Job implementations, for executing
groups of sub-jobs or running shell commands in job
tasks. Additionally the package also contains tools used in writing
specific job implementations, including a type used to interchange
jobs, and a a monotonically increasing JobId generator.

Queue

Queue provides implementations of the Queue implementation, which
provide different task dispatching and distribution strategies.

Dependency

The Dependency package contains the interface that describes how jobs
and queues communicate about the dependency between jobs
(dependecy.Manager), as well as several generic dependency
implementations.
*/
