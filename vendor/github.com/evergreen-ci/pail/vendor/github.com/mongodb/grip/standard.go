package grip

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip/send"
)

var std = NewJournaler("grip")

func init() {
	if !strings.Contains(os.Args[0], "go-build") {
		std.SetName(filepath.Base(os.Args[0]))
	}

	sender, err := send.NewNativeLogger(std.Name(), std.GetSender().Level())
	std.CatchAlert(std.SetSender(sender))
	std.CatchAlert(err)
}
