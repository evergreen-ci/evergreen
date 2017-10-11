// Package recovery provides a number of grip-integrated panic
// handling tools for capturing and responding to panics using grip
// loggers.
//
// These handlers are very useful for capturing panic messages that
// might otherwise be lost, as well as providing implementations for
// several established panic handling practices. Nevertheless, this
// assumes that the panic, or an underlying system issue does not
// affect the logging system or its dependencies. For example, panics
// caused by disk-full or out of memory situations are challenging to
// handle with this approach.
//
// All log message are logged with the default standard logger in the
// grip package.
package recovery

import (
	"errors"
	"os"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const killOverrideVarName = "__GRIP_EXIT_OVERRIDE"

// LogStackTraceAndExit captures a panic, captures and logs a stack
// trace at the Emergency level and then exits.
//
// This operation also attempts to close the underlying log sender.
func LogStackTraceAndExit(opDetails ...string) {
	if p := recover(); p != nil {
		m := message.Fields{
			message.FieldsMsgName: "hit panic; exiting",
			"panic":               panicString(p),
			"stack":               message.NewStack(1, "").Raw(),
		}

		if len(opDetails) > 0 {
			m["operation"] = strings.Join(opDetails, " ")
		}

		// check this env var so that we can avoid exiting in the test.
		if os.Getenv(killOverrideVarName) == "" {
			grip.GetSender().Close()
			grip.EmergencyFatal(m)
		} else {
			grip.Emergency(m)
		}
	}
}

// LogStacktraceAndContinue recovers from a panic, and then logs the
// captures a stack trace and logs a structured message at "Alert"
// level without further action.
//
// The "opDetails" argument is optional, and is joined as an
// "operation" field in the log message for providing additional
// context.
//
// Use in a common defer statement, such as:
//
//    defer recovery.LogStackTraceAndContinue("operation")
//
func LogStackTraceAndContinue(opDetails ...string) {
	if p := recover(); p != nil {
		m := message.Fields{
			message.FieldsMsgName: "hit panic; recovering",
			"panic":               panicString(p),
			"stack":               message.NewStack(1, "").Raw(),
		}

		if len(opDetails) > 0 {
			m["operation"] = strings.Join(opDetails, " ")
		}

		grip.Alert(m)
	}
}

// HandlePanicWithError is used to convert a panic to an error.
//
// The "opDetails" argument is optional, and is joined as an
// "operation" field in the log message for providing additional
// context.
//
// You must construct a recovery function as in the following example:
//
//     defer func() { err = recovery.HandlePanicWithError(recover(),  err, "op") }()
//
// This defer statement must occur in a function that declares a
// default error return value as in:
//
//     func operation() (err error) {}
//
func HandlePanicWithError(p interface{}, err error, opDetails ...string) error {
	catcher := grip.NewSimpleCatcher()
	catcher.Add(err)

	if p != nil {
		ps := panicString(p)
		catcher.Add(errors.New(ps))
		m := message.Fields{
			message.FieldsMsgName: "hit panic; adding error",
			"stack":               message.NewStack(2, "").Raw(),
			"panic":               ps,
		}

		if err != nil {
			m["error"] = err.Error()
		}

		if len(opDetails) > 0 {
			m["operation"] = strings.Join(opDetails, " ")
		}

		grip.Alert(m)
	}

	return catcher.Resolve()
}
