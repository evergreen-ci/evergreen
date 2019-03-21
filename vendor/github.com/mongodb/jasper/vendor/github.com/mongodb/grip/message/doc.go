/*
package message defines the Composer interface and a handful of
implementations which represent the structure for messages produced by grip.

Message Composers

The Composer interface provides a common way to define messages, with
two main goals:

1. Provide a common interface for representing structured and
unstructured logging data regardless of logging backend or interface.

2. Provide a method for *lazy* construction of log messages so they're
built *only* if a log message is over threshold.

The message package also contains many implementations of Composer
which should support most logging use cases. However, any package
using grip for logging may need to implement custom composer types.

The Composer implementations in the message package compose the Base
type to provide some common functionality around priority setting and
data collection.

The logging methods in the Journaler interface typically convert all
inputs into a reasonable Composer implementations.
*/
package message

// This file is intentionally documentation only.
