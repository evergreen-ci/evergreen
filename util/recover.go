package util

import (
	"fmt"
	"os"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// RecoverAndLogStackTrace captures a panic stack trace and writes it
// to the log file rather than allowing it to be printed to standard
// error, where it would be lost (in the case of agents.)
func RecoverAndLogStackTrace() {
	if p := recover(); p != nil {
		panicMsg, ok := p.(string)
		if !ok {
			panicMsg = fmt.Sprintf("%+v", panicMsg)
		}
		m := message.NewStackFormatted(1, "encountered panic '%s' at top level; recovering trace:", panicMsg)
		grip.Alert(m)

		r := m.Raw().(message.StackTrace)
		for idx, f := range r.Frames {
			grip.Criticalf("call #%d\n\t%s\n\t\t%s:%d", idx, f.Function, f.File, f.Line)
		}

		exitMsg := message.NewFormatted("hit panic '%s' at top level; exiting", panicMsg)

		// check this env var so that we can avoid exiting in the test.
		if os.Getenv("EVERGREEN_TEST") == "" {
			grip.EmergencyFatal(exitMsg)
		} else {
			grip.Emergency(exitMsg)
		}
	}
}
