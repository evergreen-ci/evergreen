=======================================================
``grip`` -- A Go Library for Logging and Error Handling
=======================================================

*Under Construction*

``grip`` isn't any thing special, but it does a few pretty great
things:

#. Provide a common logging interface with support for multiple
   logging backends including syslog, systemd's journal, slack, xmpp,
   a JSON logging system, and others.

#. Provides some simple methods for handling errors, particularly when
   you want to accumulate and then return errors.

#. Provides tools for collecting structured logging information.

*You just get a grip, folks.*

Use
---

Download:

::

   go get -u github.com/tychoish/grip

Import:

::

   import "github.com/tychoish/grip"

Components
----------

Output Formats
~~~~~~~~~~~~~~

Grip supports a number of different logging output backends:

- systemd's journal (linux-only)
- syslog (unix-only)
- writing messages to standard output. (default)
- writing messages to a file.
- sending messages to a slack's channel
- sending messages to a user via XMPP (jabber.)

The default logger interface has methods to switch the backend to
the standard output (native; default), and file-based loggers. The
SetSender() and CloneSender() methods allow to replace the sender
implementation in your logger.

See the documentation of the `Sender interface
<https://godoc.org/github.com/tychoish/grip/send#Sender>`_ for more
information on building new senders.

Logging
~~~~~~~

Provides a fully featured level-based logging system with multiple
backends (e.g. send.Sender). By default logging messages are printed
to standard output, but backends exists for many possible targets. The
interface for logging is provided by the Journaler interface.

By default ``grip.std`` defines a standard global  instances
that you can use with a set of ``grip.<Level>`` functions, or you can
create your own ``Journaler`` instance and embed it in your own
structures and packages.

Defined helpers exist for the following levels/actions:

- ``Debug``
- ``Info``
- ``Notice``
- ``Warning``
- ``Error``
- ``Critical``
- ``Alert``
- ``Emergency``
- ``EmergencyPanic``
- ``EmergencyFatal``

Helpers ending with ``Panic`` call ``panic()`` after logging the message
message, and helpers ending with ``Fatal`` call ``os.Exit(1)`` after
logging the message. These are primarily for handling errors in your
main() function and should be used sparingly, if at all, elsewhere.

``Journaler`` instances have a notion of "default" log levels and
thresholds, which provide the basis for verbosity control and sane
default behavior. The default level defines the priority/level of any
message with an invalid priority specified. The threshold level,
defines the minimum priority or level that ``grip`` sends to the
logging system. It's not possible to suppress the highest log level,
``Emergency`` messages will always log.

``Journaler`` objects have the following, additional methods (also
available as functions in the ``grip`` package to manage the global
standard logger instance.):

- ``SetName(<string>)`` to reset the name of the logger. ``grip``
  attempts to set this to the name of your program for the standard
  logger.

- ``SetDefault(<level int>)`` change the default log level. Levels are
  values between ``0`` and ``7``, where lower numbers are *more*
  severe. ``grip`` does *not* forbid configurations where default
  levels are *below* the configured threshold.

- ``SetThreshold(<level int>)`` Change the lowest log level that the
  ``grip`` will transmit to the logging mechanism (either ``systemd``
  ``journald`` or Go's standard logging.) Log messages with lower
  levels are not captured and ignored.

The ``Journaler.InvertFallback`` flag (bool) switches a ``Journaler``
instance to prefer the standard logging mechanism rather than
``systemd``.

By default:

- the log level uses the "Notice" level (``5``)

- the minimum threshold for logging is the "Info" level (``6``)
  (suppressing only debug.)

- fallback logging writes to standard output.

Collector for "Continue on Error" Semantics
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you want to do something other than just swallow errors, but don't
need to hard abort, the ``MultiCatcher`` object makes this pattern
swell, a la:

::

   func doStuff(dirname string) (error) {
           files, err := ioutil.ReadDir(dirname)
           if err != nil {
                   // should abort here because we shouldn't continue.
                   return err
           }

           catcher := grip.NewCatcher()
           for _, f := range files {
               err = doStuffToFile(f.Name())
               catcher.Add(err)
           }

           return catcher.Resolve()
   }


Simple Error Catching
~~~~~~~~~~~~~~~~~~~~~

Use ``grip.Catch(<err>)`` to check and print error messages.

There are also helper functions on ``Journaler`` objects that check
and log error messages using either the default (global) ``Journaler``
instance, or as a method on specific ``Journaler`` instances, at all
levels:

- ``CatchDebug``
- ``CatchInfo``
- ``CatchNotice``
- ``CatchWarning``
- ``CatchError``
- ``CatchCritical``
- ``CatchAlert``
- ``CatchEmergency``
- ``CatchEmergencyPanic``
- ``CatchEmergencyFatal``

Conditional Logging
~~~~~~~~~~~~~~~~~~~

``grip`` incldues support for conditional logging, so that you can
only log a message in certain situations, by adding a Boolean argument
to the logging call. Use this to implement "log sometimes" messages to
minimize verbosity without complicating the calling code around the
logging.

These methods have a ``<Level>When<>`` format. For
example: ``AlertWhen``, ``AlertWhenln``, ``AlertWhenf``.

Composed Logging
~~~~~~~~~~~~~~~~

If the production of the log message is resource intensive or
complicated, you may wish to use a "composed logging," which delays
the generation of the log message from the logging call site to the
message propagation, to avoid generating the log message unless
neccessary. Rather than passing the log message as a string, pass the
logging function an instance of a type that implements the
``MessageComposer`` interface: ::

   type MessageComposer interface {
        String() string
        Raw() interface{}
        Loggable() bool
        Priority() level.Priority
        SetPriority(level.Priority) error
   }

Composed logging may be useful for some debugging logging that depends
on additional database, API queries, or data serialization. Composers
are also the mechanism through which the ``Catch<>`` methods are
implemented,

Grip uses composers internally, but you can pass composers directly to
any of the basic logging method (e.g. ``Info()``, ``Debug()``) for
composed logging.

Grip includes a number of message types, including those that collect
system information, process information, stacktraces, or simple
user-specified structured information.
