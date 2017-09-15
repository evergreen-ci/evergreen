package logging

import (
	"errors"

	"github.com/mongodb/grip/send"
)

// SetSender swaps send.Sender() implementations in a logging
// instance. Calls the Close() method on the existing instance before
// changing the implementation for the current instance. SetSender
// will configure the incoming sender to have the same name as well as
// default and threshold level as the outgoing sender.
func (g *Grip) SetSender(s send.Sender) error {
	if s == nil {
		return errors.New("cannot set the sender to nil")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if err := s.SetLevel(g.impl.Level()); err != nil {
		return err
	}

	if err := g.impl.Close(); err != nil {
		return err
	}

	s.SetName(g.impl.Name())
	g.impl = s

	return nil
}

// GetSender returns the current Journaler's sender instance. Use this in
// combination with SetSender() to have multiple Journaler instances
// backed by the same send.Sender instance.
func (g *Grip) GetSender() send.Sender {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.impl
}
