package send

import (
	"context"
	"log"
	"os"

	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type genericLogger struct {
	*Base
}

// NewGenericLogger constructs a Sender that executes a function
func NewGenericLogger(name string, l LevelInfo) (Sender, error) {
	gl := &genericLogger{
		Base: NewBase(name),
	}

	if err := gl.SetLevel(l); err != nil {
		return nil, err
	}

	gl.SetName(name)

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := gl.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	return gl, nil
}

func (gl *genericLogger) Send(m message.Composer) {
	if !gl.Level().ShouldLog(m) {
		return
	}

	generic := m.Raw().(message.Generic)
	if err := generic.Send(); err != nil {
		gl.ErrorHandler()(errors.Wrap(err, "problem processing generic message"), m)
	}
}

func (gl *genericLogger) Flush(_ context.Context) error { return nil }
