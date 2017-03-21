/*
Package grip provides a flexible logging package for basic Go programs.
Drawing inspiration from Go and Python's standard library
logging, as well as systemd's journal service, and other logging
systems, Grip provides a number of very powerful logging
abstractions in one high-level package.

Logging Instances

The central type of the grip package is the Journaler type,
instances of which provide distinct log capturing system. For ease,
following from the Go standard library, the grip package provides
parallel public methods that use an internal "standard" Jouernaler
instance in the grip package, which has some defaults configured
and may be sufficient for many use cases.

Output

The send.Sender interface provides a way of changing the logging
backend, and the send package provides a number of alternate
implementations of logging systems, including: systemd's journal,
logging to standard output, logging to a file, and generic syslog
support.

Messages

The message.Composer interface is the representation of all
messages. They are implemented to provide a raw structured form as
well as a string representation for more conentional logging
output. Furthermore they are intended to be easy to produce, and defer
more expensive processing until they're being logged, to prevent
expensive operations producing messages that are below threshold.
*/
package grip

// This file is intentionally documentation only.
