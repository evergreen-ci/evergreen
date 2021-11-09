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
	"os"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
)

const killOverrideVarName = "__GRIP_EXIT_OVERRIDE"

// LogStackTraceAndExit captures a panic, captures and logs a stack
// trace at the Emergency level and then exits.
//
// This operation also attempts to close the underlying log sender.
func LogStackTraceAndExit(opDetails ...string) {
	if p := recover(); p != nil {
		logAndExit(p, logging.MakeGrip(grip.GetSender()), message.MakeFields(getMessage(opDetails)))
	}
}

// LogStackTraceAndContinue recovers from a panic, and then logs the
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
		logAndContinue(p, logging.MakeGrip(grip.GetSender()), message.MakeFields(getMessage(opDetails)))
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
		perr := panicError(p)
		catcher.Add(perr)

		handleWithError(perr, err, logging.MakeGrip(grip.GetSender()), message.MakeFields(getMessage(opDetails)))
	}

	return catcher.Resolve()
}

// AnnotateMessageWithStackTraceAndContinue logs panics and continues
// and is meant to be used in defer statements like
// LogStackTraceAndContinue.
//
// It takes an interface which it converts to a message.Composer using
// the same rules as logging methods, and annotates those messages
// with the stack trace and panic information.
func AnnotateMessageWithStackTraceAndContinue(m interface{}) {
	if p := recover(); p != nil {
		logAndContinue(p, logging.MakeGrip(grip.GetSender()), message.ConvertToComposer(level.Critical, m))
	}
}

// SendStackTraceAndContinue is similar to
// AnnotateMessageWithStackTraceAndContinue, but allows you to inject a
// grip.Journaler interface to receive the log message.
func SendStackTraceAndContinue(logger grip.Journaler, m interface{}) {
	if p := recover(); p != nil {
		logAndContinue(p, logger, message.ConvertToComposer(level.Critical, m))
	}
}

// AnnotateMessageWithStackTraceAndExit logs panics and calls exit
// like LogStackTraceAndExit.
//
// It takes an interface which it converts to a message.Composer using
// the same rules as logging methods, and annotates those messages
// with the stack trace and panic information.
func AnnotateMessageWithStackTraceAndExit(m interface{}) {
	if p := recover(); p != nil {
		logAndExit(p, logging.MakeGrip(grip.GetSender()), message.ConvertToComposer(level.Critical, m))
	}
}

// SendStackTraceMessageAndExit is similar to
// AnnotateMessageWithStackTraceAndExit, but allows you to inject a
// grip.Journaler interface.
func SendStackTraceMessageAndExit(logger grip.Journaler, m interface{}) {
	if p := recover(); p != nil {
		logAndExit(p, logger, message.ConvertToComposer(level.Critical, m))
	}
}

// AnnotateMessageWithPanicError processes a panic and converts it
// into an error, combining it with the value of another error. Like,
// HandlePanicWithError, this method is meant to be used in your own
// defer functions.
//
// It takes an interface which it converts to a message.Composer using
// the same rules as logging methods, and annotates those messages
// with the stack trace and panic information.
func AnnotateMessageWithPanicError(p interface{}, err error, m interface{}) error {
	catcher := grip.NewSimpleCatcher()
	catcher.Add(err)

	if p != nil {
		perr := panicError(p)
		catcher.Add(perr)

		handleWithError(perr, err, logging.MakeGrip(grip.GetSender()), message.ConvertToComposer(level.Critical, m))
	}

	return catcher.Resolve()
}

// SendMessageWithPanicError is similar to
// AnnotateMessageWithPanicError, but allows you to inject a custom
// grip.Jounaler interface to receive the log message.
func SendMessageWithPanicError(p interface{}, err error, logger grip.Journaler, m interface{}) error {
	catcher := grip.NewSimpleCatcher()
	catcher.Add(err)

	if p != nil {
		perr := panicError(p)
		catcher.Add(perr)

		handleWithError(perr, err, logger, message.ConvertToComposer(level.Critical, m))
	}

	return catcher.Resolve()
}

////////////////////////////////////////////////////////////////////////
//
// helpers

func getMessage(details []string) message.Fields {
	m := message.Fields{}

	if len(details) > 0 {
		m["operation"] = strings.Join(details, " ")
	}

	return m
}

func logAndContinue(p interface{}, logger grip.Journaler, msg message.Composer) {
	_ = msg.Annotate("panic", panicString(p))
	_ = msg.Annotate("stack", message.NewStack(3, "").Raw().(message.StackTrace).Frames)
	_ = msg.Annotate(message.FieldsMsgName, "hit panic; recovering")

	logger.Alert(msg)
}

func logAndExit(p interface{}, logger grip.Journaler, msg message.Composer) {
	_ = msg.Annotate("panic", panicString(p))
	_ = msg.Annotate("stack", message.NewStack(3, "").Raw().(message.StackTrace).Frames)
	_ = msg.Annotate(message.FieldsMsgName, "hit panic; exiting")

	// check this env var so that we can avoid exiting in the test.
	if os.Getenv(killOverrideVarName) == "" {
		logger.EmergencyFatal(msg)
	} else {
		logger.Emergency(msg)
	}
}

func handleWithError(p error, err error, logger grip.Journaler, msg message.Composer) {
	_ = msg.Annotate("panic", p.Error())
	_ = msg.Annotate("stack", message.NewStack(3, "").Raw().(message.StackTrace).Frames)
	_ = msg.Annotate(message.FieldsMsgName, "hit panic; adding error")

	if err != nil {
		_ = msg.Annotate("error", err.Error())
	}

	logger.Alert(msg)
}
