package logging

import (
	"os"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

// Grip provides the core implementation of the Logging interface. The
// interface is mirrored in the "grip" package's public interface, to
// provide a single, global logging interface that requires minimal
// configuration.
type Grip struct{ send.Sender }

// NewGrip takes the name for a logging instance and creates a new
// Grip instance with configured with a local, standard output logging.
// The default level is "Notice" and the threshold level is "info."
func NewGrip(name string) *Grip {
	sender, _ := send.NewNativeLogger(name,
		send.LevelInfo{
			Threshold: level.Info,
			Default:   level.Notice,
		})

	return &Grip{sender}
}

// Internal

// For sending logging messages, in most cases, use the
// Journaler.sender.Send() method, but we have a couple of methods to
// use for the Panic/Fatal helpers.
func (g *Grip) sendPanic(m message.Composer) {
	// the Send method in the Sender interface will perform this
	// check but to add fatal methods we need to do this here.
	if g.Level().ShouldLog(m) {
		g.Send(m)
		panic(m.String())
	}
}

func (g *Grip) sendFatal(m message.Composer) {
	// the Send method in the Sender interface will perform this
	// check but to add fatal methods we need to do this here.
	if g.Level().ShouldLog(m) {
		g.Send(m)
		os.Exit(1)
	}
}
