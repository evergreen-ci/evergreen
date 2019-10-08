=======================================
``aviation`` -- gRPC Middleware Helpers
=======================================

Overview
--------

Aviation is a set of tools for building services, as a kind of
analogue for what `gimlet <https://github.com/evergreen-ci/gimlet>`_
does for REST services.

Goals
-----

- provide middleware to support common logging, recovery, middleware.

- support basic authentication mechanism for interoperability with
  gimlet services.

Caveats
-------

- aviation does not (and should not!) attempt to support building
  client interceptors.

- aviation should not attempt to wrap gRPC in the way that gimlet
  wraps negroni and gorilla mux.
