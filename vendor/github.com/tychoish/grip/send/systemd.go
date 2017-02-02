// +build linux

package send

import (
	"fmt"
	"log"
	"os"

	"github.com/coreos/go-systemd/journal"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

type systemdJournal struct {
	options map[string]string
	*base
}

// NewJournaldLogger creates a Sender object that writes log messages
// to the system's systemd journald logging facility. If there's an
// error with the sending to the journald, messages fallback to
// writing to standard output.
func NewSystemdLogger(name string, l LevelInfo) (Sender, error) {
	return setup(MakeSystemdLogger(), name, l)
}

// MakeSystemdLogger constructs an unconfigured systemd journald
// logger. Pass to Journaler.SetSender or call SetName before using.
func MakeSystemdLogger() Sender {
	s := &systemdJournal{
		options: make(map[string]string),
		base:    newBase(""),
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	s.SetErrorHandler(ErrorHandlerFromLogger(fallback))

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s]", s.Name()))
	}

	return s
}

func (s *systemdJournal) Close() error { return nil }

func (s *systemdJournal) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		err := journal.Send(m.String(), s.level.convertPrioritySystemd(m.Priority()), s.options)
		if err != nil {
			s.errHandler(err, m)
		}
	}
}

func (l LevelInfo) convertPrioritySystemd(p level.Priority) journal.Priority {
	switch p {
	case level.Emergency:
		return journal.PriEmerg
	case level.Alert:
		return journal.PriAlert
	case level.Critical:
		return journal.PriCrit
	case level.Error:
		return journal.PriErr
	case level.Warning:
		return journal.PriWarning
	case level.Notice:
		return journal.PriNotice
	case level.Info:
		return journal.PriInfo
	case level.Debug, level.Trace:
		return journal.PriDebug
	default:
		return l.convertPrioritySystemd(l.Default)
	}
}
