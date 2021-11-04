package mgo

import (
	"bytes"
)

// Bulk represents an operation that can be prepared with several
// orthogonal changes before being delivered to the server.
//
// MongoDB servers older than version 2.6 do not have proper support for bulk
// operations, so the driver attempts to map its API as much as possible into
// the functionality that works. In particular, in those releases updates and
// removals are sent individually, and inserts are sent in bulk but have
// suboptimal error reporting compared to more recent versions of the server.
// See the documentation of BulkErrorCase for details on that.
//
// Relevant documentation:
//
//   http://blog.mongodb.org/post/84922794768/mongodbs-new-bulk-api
//

// BulkError holds an error returned from running a Bulk operation.
// Individual errors may be obtained and inspected via the Cases method.
type BulkError struct {
	ecases []BulkErrorCase
}

func (e *BulkError) Error() string {
	if len(e.ecases) == 0 {
		return "invalid BulkError instance: no errors"
	}
	if len(e.ecases) == 1 {
		return e.ecases[0].Err.Error()
	}
	msgs := make([]string, 0, len(e.ecases))
	seen := make(map[string]bool)
	for _, ecase := range e.ecases {
		msg := ecase.Err.Error()
		if !seen[msg] {
			seen[msg] = true
			msgs = append(msgs, msg)
		}
	}
	if len(msgs) == 1 {
		return msgs[0]
	}
	var buf bytes.Buffer
	buf.WriteString("multiple errors in bulk operation:\n")
	for _, msg := range msgs {
		buf.WriteString("  - ")
		buf.WriteString(msg)
		buf.WriteByte('\n')
	}
	return buf.String()
}

// BulkErrorCase holds an individual error found while attempting a single change
// within a bulk operation, and the position in which it was enqueued.
//
// MongoDB servers older than version 2.6 do not have proper support for bulk
// operations, so the driver attempts to map its API as much as possible into
// the functionality that works. In particular, only the last error is reported
// for bulk inserts and without any positional information, so the Index
// field is set to -1 in these cases.
type BulkErrorCase struct {
	Index int // Position of operation that failed, or -1 if unknown.
	Err   error
}

// Cases returns all individual errors found while attempting the requested changes.
//
// See the documentation of BulkErrorCase for limitations in older MongoDB releases.
func (e *BulkError) Cases() []BulkErrorCase {
	return e.ecases
}
