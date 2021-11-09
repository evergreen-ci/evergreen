package logging

import (
	"os"
	"sync"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

// Grip provides the core implementation of the Logging interface. The
// interface is mirrored in the "grip" package's public interface, to
// provide a single, global logging interface that requires minimal
// configuration.
type Grip struct {
	impl         send.Sender
	defaultLevel level.Priority
	mu           sync.RWMutex
}

// MakeGrip builds a new logging interface from a sender implmementation
func MakeGrip(s send.Sender) *Grip {
	return &Grip{
		impl:         s,
		defaultLevel: level.Info,
	}
}

// NewGrip takes the name for a logging instance and creates a new
// Grip instance with configured with a local, standard output logging.
// The default level is "Notice" and the threshold level is "info."
func NewGrip(name string) *Grip {
	sender, _ := send.NewNativeLogger(name,
		send.LevelInfo{
			Threshold: level.Trace,
			Default:   level.Trace,
		})

	return &Grip{impl: sender}
}

func (g *Grip) Name() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.impl.Name()
}

func (g *Grip) SetName(n string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	g.impl.SetName(n)
}

func (g *Grip) SetLevel(info send.LevelInfo) error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	sl := g.impl.Level()

	if !info.Default.IsValid() {
		info.Default = sl.Default
	}

	if !info.Threshold.IsValid() {
		info.Threshold = sl.Threshold
	}

	return g.impl.SetLevel(info)
}

func (g *Grip) Send(m interface{}) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	g.impl.Send(message.ConvertToComposer(g.defaultLevel, m))
}

// Internal

// For sending logging messages, in most cases, use the
// Journaler.sender.Send() method, but we have a couple of methods to
// use for the Panic/Fatal helpers.
func (g *Grip) sendPanic(m message.Composer) {
	// the Send method in the Sender interface will perform this
	// check but to add fatal methods we need to do this here.
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.impl.Level().ShouldLog(m) {
		g.impl.Send(m)
		panic(m.String())
	}
}

func (g *Grip) sendFatal(m message.Composer) {
	// the Send method in the Sender interface will perform this
	// check but to add fatal methods we need to do this here.
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.impl.Level().ShouldLog(m) {
		g.impl.Send(m)
		os.Exit(1)
	}
}

func (g *Grip) send(m message.Composer) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	g.impl.Send(m)
}
