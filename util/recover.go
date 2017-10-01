package util

import (
	"errors"
	"fmt"
	"os"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// RecoverAndLogStackTrace captures a panic stack trace and writes it
// to the log file rather than allowing it to be printed to standard
// error, where it would be lost (in the case of agents.)
func RecoverLogStackTraceAndExit() {
	if p := recover(); p != nil {
		m := message.Fields{
			"message": "hit panic; exiting",
			"panic":   panicString(p),
			"stack":   message.NewStack(1, "").Raw(),
		}

		// check this env var so that we can avoid exiting in the test.
		if os.Getenv("EVERGREEN_TEST") == "" {
			grip.EmergencyFatal(m)
		} else {
			grip.Emergency(m)
		}
	}
}

func RecoverLogStackTraceAndContinue(opName string) {
	if p := recover(); p != nil {
		m := message.Fields{
			"message":   "hit panic; recovering",
			"operation": opName,
			"panic":     panicString(p),
			"stack":     message.NewStack(1, "").Raw(),
		}

		if opName != "" {
			m["operation"] = opName
		}

		grip.Alert(m)
	}
}

func RecoverAndError(err error, opName string) {
	if p := recover(); p != nil {
		catcher := grip.NewSimpleCatcher()
		catcher.Add(err)
		ps := panicString(p)
		catcher.Add(errors.New(ps))
		m := message.Fields{
			"message": "hit panic; adding error",
			"stack":   message.NewStack(1, "").Raw(),
			"error":   catcher.Resolve(),
		}
		if opName != "" {
			m["operation"] = opName
		}
		grip.Alert(m)
		err = catcher.Resolve()
	}
}

func panicString(p interface{}) string {
	panicMsg, ok := p.(string)
	if !ok {
		panicMsg = fmt.Sprintf("%+v", panicMsg)
	}

	return panicMsg
}
