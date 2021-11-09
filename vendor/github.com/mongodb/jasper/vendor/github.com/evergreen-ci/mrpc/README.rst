=========================================================
``mprc`` -- Arbitrary MongoDB Wire Protocol RPC Framework
=========================================================

Overview
--------

MRPC is a set of tools to support writing RPC-like services using
the MongoDB wire protocol as a transport layer. The idea is, if you
have a MongoDB driver (or *are* a MongoDB driver,) you should be able
to communicate with other services without needing a second client
driver, or to fall back to shell scripting for interacting with tools
and utility.

Furthermore, it should be trivially easy to use this kind of interface
to wrap up functionality for arbitrary tools.

History
-------

The "mongowire" and "bson" packages are heavily adapted from
`github.com/erh/mongonet <https://github.com/erh/mongonet>`_. Indeed
this repository retains that history.

Use
---

Create a new service instance with the ``NewService`` function: ::

   service := NewService("127.0.0.1", 3000)

For each operation, you must define an ``mongowire.OpScope`` and a
handler function, as in: ::

   op := &mogowire.OpScope{
        Type:      mongowire.OP_COMMAND,
        Context:   "admin",
        Command:   "listCommand",
   }

   handler := func(ctx context.Context, w io.Writer, m mongowire.Message) {
        // operation implementation
   }

Then register the operation: ::

   err := service.RegisterOperation(op, handler)

The ``RegisterOperation`` method returns an error if the operation
scope is not unique, or the handler is nil. You can validate an
operation scope using its ``Validate`` method.

When you have defined all methods, you can start the service using the
``Run`` method: ::

   err := service.Run(ctx)

The ``Run`` method returns an error if there are any issues starting
the service or if the context is canceled.

Quirks
------

- "Legacy" commands, which are issued as queries against a special
  collection are "up" converted to OP_COMMAND messages internally (though,
  mrpc does track that it has performed the conversion so you can
  see that.) Thus, you must register an OP_COMMAND, even if your
  clients are sending OP_QUERY messages.

- Handler functions are responsible for determining the "appropriate"
  response, and the framework cannot ensure that OP_COMMAND requests
  get OP_COMMAND_REPLY messages.


Dependencies
------------

Currently, ``mrpc`` does not vendor its dependencies, and uses:

- `github.com/mongodb/grip <https://github.com/mongodb/grip>`_ (for logging)
- `github.com/pkg/errors <https;//github.com/pkg/errors>`_ (for error annotation.)
- `github.com/evergreen-ci/birch <https://github.com/evergreen-ci/birch>`_ (for bson parsing)
