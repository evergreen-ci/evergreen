package send

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tychoish/grip/message"
)

type fileLogger struct {
	logger *log.Logger
	*base
}

// NewFileLogger creates a Sender implementation that writes log
// output to a file. Returns an error but falls back to a standard
// output logger if there's problems with the file. Internally using
// the go standard library logging system.
func NewFileLogger(name, filePath string, l LevelInfo) (Sender, error) {
	s, err := MakeFileLogger(filePath)
	if err != nil {
		return nil, err
	}

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	s.SetName(name)

	return s, nil
}

// MakeFileLogger creates a file-based logger, writing output to
// the specified file. The Sender instance is not configured: Pass to
// Journaler.SetSender or call SetName before using.
func MakeFileLogger(filePath string) (Sender, error) {
	s := &fileLogger{base: newBase("")}

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("error opening logging file, %s", err.Error())
	}

	s.reset = func() {
		s.logger = log.New(f, strings.Join([]string{"[", s.Name(), "] "}, ""), log.LstdFlags)
	}

	s.closer = func() error {
		return f.Close()
	}

	return s, nil
}

func (f *fileLogger) Type() SenderType { return File }
func (f *fileLogger) Send(m message.Composer) {
	if f.level.ShouldLog(m) {
		f.logger.Printf("[p=%s]: %s", m.Priority(), m.Resolve())
	}
}
