package grip

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
)

var std = NewJournaler("grip")

func init() {
	if !strings.Contains(os.Args[0], "go-build") {
		std.SetName(filepath.Base(os.Args[0]))
	}

	sender, err := send.NewNativeLogger(std.Name(), std.GetSender().Level())
	std.Alert(std.SetSender(sender))
	std.Alert(err)
}

// MakeStandardLogger constructs a standard library logging instance
// that logs all messages to the global grip logging instance.
func MakeStandardLogger(p level.Priority) *log.Logger {
	return send.MakeStandardLogger(std.GetSender(), p)
}

// GetDefaultJournaler returns the default journal instance used by
// this library.
func GetDefaultJournaler() Journaler {
	return std
}

// SetDefaultStandardLogger set's the standard library's global
// logging instance to use grip's global logger at the specified
// level.
func SetDefaultStandardLogger(p level.Priority) {
	log.SetFlags(0)
	log.SetOutput(send.MakeWriterSender(std.GetSender(), p))
}
