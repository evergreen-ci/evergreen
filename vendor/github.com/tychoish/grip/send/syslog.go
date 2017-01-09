// +build linux freebsd solaris darwin

package send

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
	"strings"

	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

type syslogger struct {
	logger   *syslog.Writer
	fallback *log.Logger
	*base
}

// NewSyslogLogger creates a new Sender object that writes all
// loggable messages to a syslog instance on the specified
// network. Uses the Go standard library syslog implementation that is
// only available on Unix systems. Use this constructor to return a
// connection to a remote Syslog interface, but will fall back first
// to the local syslog interface before writing messages to standard
// output.
func NewSyslogLogger(name, network, raddr string, l LevelInfo) (Sender, error) {
	s := MakeSysLogger(network, raddr)

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeSysLogger constructs a minimal and unconfigured logger that
// posts to systemd's journal.
// Pass to Journaler.SetSender or call SetName before using.
func MakeSysLogger(network, raddr string) Sender {
	s := &syslogger{base: newBase("")}

	s.reset = func() {
		s.fallback = log.New(os.Stderr, strings.Join([]string{"[", s.Name(), "] "}, ""), log.LstdFlags)

		if err := s.Close(); err != nil {
			s.fallback.Printf("problem closing existing syslogger: %+v", err)
		}

		w, err := syslog.Dial(network, raddr, syslog.LOG_DEBUG, s.Name())
		if err != nil {
			s.fallback.Printf("error restarting syslog [%s] for logger: %s", err.Error(), s.Name())
			return
		}

		s.closer = func() error {
			return w.Close()
		}

		s.logger = w
	}

	return s
}

// MakeLocalSyslogLogger is a constructor for creating the same kind of
// Sender instance as NewSyslogLogger, except connecting directly to
// the local syslog service. If there is no local syslog service, or
// there are issues connecting to it, writes logging messages to
// standard error. Pass to Journaler.SetSender or call SetName before using.
func MakeLocalSyslogLogger() Sender {
	return MakeSysLogger("", "")
}

func (s *syslogger) Close() error     { return s.logger.Close() }
func (s *syslogger) Type() SenderType { return Syslog }

func (s *syslogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		msg := m.Resolve()

		if err := s.sendToSysLog(m.Priority(), msg); err != nil {
			s.fallback.Println("syslog error:", err.Error())
			s.fallback.Printf("[p=%d]: %s\n", m.Priority(), msg)
		}
	}
}

func (s *syslogger) sendToSysLog(p level.Priority, message string) error {
	switch p {
	case level.Emergency:
		return s.logger.Emerg(message)
	case level.Alert:
		return s.logger.Alert(message)
	case level.Critical:
		return s.logger.Crit(message)
	case level.Error:
		return s.logger.Err(message)
	case level.Warning:
		return s.logger.Warning(message)
	case level.Notice:
		return s.logger.Notice(message)
	case level.Info:
		return s.logger.Info(message)
	case level.Debug, level.Trace:
		return s.logger.Debug(message)
	}

	return fmt.Errorf("encountered error trying to send: {%s}. Possibly, priority related", message)
}
