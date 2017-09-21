/*
Package registry contains infrastructure to support the persistence of
Job definitions.

Job and Dependency Registries

Systems need to be able to create and access Jobs and Dependency
instances potentially from other implementations, and the registry
provides a way to register new Job types both internal and external to
the amboy package, and ensures that Jobs can be persiststed and
handled generically as needed.

When you implement a new amboy/dependency.Manager or amboy.Job type,
be sure to write a simple factory function for the type and register
the factory in an init() function. Consider the following example:

   func init() {
      RegisterJobType("noop", noopJobFactory)
   }

   func noopJobFactory() amboy.Job {
      return &NoopJob{}  /
   }

The dependency and job registers have similar interfaces.

The registry package also provides functions for converting between an
"interchange" format for persisting Job objects of mixed types
consistently. Typically only authors of Queue implementations will
need to use these operations.
*/
package registry

// This file is intentionally documentation only.
