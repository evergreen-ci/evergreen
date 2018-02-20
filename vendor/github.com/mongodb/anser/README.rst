=======================================
``anser`` -- Database Migration Toolkit
=======================================

Summary
-------

Anser is a toolkit for managing evolving data sets for
applications. It focuses on on-line data transformations and providing
higher-level tools to support data modeling and access. 

For the Evergreen project, anser allows us to treat these routine data
migrations, data back fills, and retroactively changing the schema of
legacy data as part of application code rather than one-off shell
scripts.

Overview
--------

In general, anser migrations have a two-phase approach. First a
generator runs with some configuration and an input querey to collect
input documents and creation migration jobs. Then, the output of these
generators, are executed in parallel 

You can define generators either directly in your own code, *or* you
can use the configuration-file based approach for a more flexible
approach.

Concepts
~~~~~~~~

There are three major types of migrations: 

- ``simple``: these migrations perform their transformations using
  MongoDB's update syntax. Use these migrations for very basic
  migrations, particularly when you want to throttle the rate of
  migrations and avoid the use of larger difficult-to-index
  multi-updates.
  
- ``manual``: these migrations call a user-defined function on a
  ``bson.RawDoc`` representation of the document to migrate. Use these
  migrations for more complex transformations or those migrations
  that you want to write in application code. 
  
- ``stream``: these migrations are similar to manual migrations;
  however, they pass a database session *and* an iterator to all
  documents impacted by the migration. These jobs offer ultimate
  flexibility, 
  
Internally these jobs execute using amboy infrastructure and make it
possible to express dependencies between migrations. Additionally the
`MovingAverageRateLimitedWorkers
<https://godoc.org/github.com/mongodb/amboy/pool#NewMovingAverageRateLimitedWorkers>`_
and `SimpleRateLimitingWorkers
<https://godoc.org/github.com/mongodb/amboy/pool#NewSimpleRateLimitedWorkers>`_
were developed to support anser migrations, as well as the `adaptive
ordering local queue
<https://godoc.org/github.com/mongodb/amboy/queue#NewAdaptiveOrderedLocalQueue>`_
which respects dependency-driven ordering.

Considerations
~~~~~~~~~~~~~~

While it's possible to do any kind of migration with anser, we have
found the following properties to be useful to keep in mind when
building migrations: 

- Write your migration implementations so that they are idempotent so
  that it's possible to run them multiple times with the same effect.

- Ensure that generator queries are supported by indexes, otherwise
  the generator processes will force collection scans. 

- Rate-Limiting, provided by configuring the underlying amboy
  infrastructure, focuses on limiting the number of migration (or
  generator) jobs executed, rather than limiting the jobs based on
  their impact. 
  
- Use batch limits. Generators have limits to control the number of
  jobs that they will produce. This is particularly useful for tests,
  but may have adverse effects on job dependency, particularly if
  logical migrations are split across more than one generator
  function.  

Installation
------------

Anser uses `grip <https://github.com/mongodb/grip>`_ for logging and
`amboy <https://github.com/mongodb/amboy>`_ for task
management. Because anser does not vendor these dependencies, you
should also vendor them. 

Resources
---------

Please consult the godoc for most usage. Most of the API is in the `top
level package <https://godoc.org/github.com/mongodb/anser>`_; however,
please do also consider the `model
<https://godoc.org/github.com/mongodb/anser/model>`_ 
and `bsonutil <https://godoc.org/github.com/mongodb/anser/bsonutil>`_ package.

Additionally you can use the interfaces `db
<https://godoc.org/github.com/mongodb/anser/db>`_
package as a wrapper for `mgo <https://godoc.org/github.com/mongodb/anser>`_ to access
MongoDB which allows you to use `mocks
<https://godoc.org/github.com/mongodb/anser/mocks>`_ as needed for
testing without depending on a running database instance.

Project
-------

Please file feature requests and bug reports in the `MAKE project
<https://jira.mongodb.com/browse/MAKE>`_ of the MongoDB Jira
instance. This is also the place to file related amboy and grip
requests.

Future anser development will focus on supporting additional migration
workflows, supporting additional MongoDB and BSON utilites, and
providing tools to support easier data-life-cycle management.
