package send

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/coreos/go-systemd/journal"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

type systemdJournal struct {
	options map[string]string
	*Base
}

// NewSystemdLogger creates a Sender object that writes log messages
// to the system's systemd journald logging facility. If there's an
// error with the sending to the journald, messages fallback to
// writing to standard output.
func NewSystemdLogger(name string, l LevelInfo) (Sender, error) {
	s, err := MakeSystemdLogger()
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakeSystemdLogger constructs an unconfigured systemd journald
// logger. Pass to Journaler.SetSender or call SetName before using.
func MakeSystemdLogger() (Sender, error) {
	if !journal.Enabled() {
		return nil, errors.New("systemd journal logging is not available on this platform")
	}

	s := &systemdJournal{
		options: make(map[string]string),
		Base:    NewBase(""),
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	_ = s.SetErrorHandler(ErrorHandlerFromLogger(fallback))

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", s.Name()))
	}

	return s, nil
}

func (s *systemdJournal) Send(m message.Composer) {
	defer func() {
		if err := recover(); err != nil {
			s.ErrorHandler()(fmt.Errorf("panic: %v", err), m)
		}
	}()

	if s.Level().ShouldLog(m) {
		err := journal.Send(m.String(), s.level.convertPrioritySystemd(m.Priority()), s.options)
		if err != nil {
			s.ErrorHandler()(err, m)
		}
	}
}

func (s *systemdJournal) Flush(_ context.Context) error { return nil }

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
