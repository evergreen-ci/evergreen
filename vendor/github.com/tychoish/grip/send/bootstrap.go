package send

import (
	"fmt"

	"github.com/tychoish/grip/message"
)

type bootstrapLogger struct {
	name  string
	level LevelInfo
}

// NewBootstrapLogger returns a minimal, default composer
// implementation, used by the Journaler instances, for storing basic
// threhsold level configuration during journaler creation. Not
// functional as a sender for general use.
func NewBootstrapLogger(name string, l LevelInfo) Sender {
	b := &bootstrapLogger{name, l}

	if err := b.SetLevel(l); err != nil {
		return nil
	}

	return b
}

func (b *bootstrapLogger) Name() string            { return b.name }
func (b *bootstrapLogger) SetName(n string)        { b.name = n }
func (b *bootstrapLogger) Send(_ message.Composer) {}
func (b *bootstrapLogger) Close() error            { return nil }
func (b *bootstrapLogger) Type() SenderType        { return Bootstrap }
func (b *bootstrapLogger) Level() LevelInfo        { return b.level }

func (b *bootstrapLogger) SetLevel(l LevelInfo) error {
	if !l.Valid() {
		return fmt.Errorf("level settings are not valid: %+v", l)
	}

	b.level = l

	return nil
}
