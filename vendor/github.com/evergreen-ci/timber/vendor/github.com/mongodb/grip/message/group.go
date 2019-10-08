package message

import (
	"fmt"
	"strings"

	"github.com/mongodb/grip/level"
)

// GroupComposer handles groups of composers as a single message,
// joining messages with a new line for the string format and returning a
// slice of interfaces for the Raw() form.
//
// Unlike most composer types, the GroupComposer is exported, and
// provides the additional Messages() method to access the composer
// objects as a slice.
//
// The GroupComposer type is not safe for concurrent access.
type GroupComposer struct {
	messages []Composer
}

// NewGroupComposer returns a GroupComposer object from a slice of
// Composers.
func NewGroupComposer(msgs []Composer) Composer {
	return &GroupComposer{
		messages: msgs,
	}
}

// MakeGroupComposer provides a variadic interface for creating a
// GroupComposer.
func MakeGroupComposer(msgs ...Composer) Composer {
	return NewGroupComposer(msgs)
}

// String satisfies the fmt.Stringer interface, and returns a string
// of the string form of all constituent composers joined with a newline.
func (g *GroupComposer) String() string {
	if len(g.messages) == 1 && g.messages[0].Loggable() {
		return g.messages[0].String()

	}

	out := []string{}
	for _, m := range g.messages {
		if m == nil {
			continue
		}
		if m.Loggable() {
			out = append(out, m.String())
		}
	}

	return strings.Join(out, "\n")
}

// Raw returns a slice of interfaces containing the raw form of all
// the constituent composers.
func (g *GroupComposer) Raw() interface{} {
	if len(g.messages) == 1 && g.messages[0].Loggable() {
		return g.messages[0].Raw()
	}

	out := []interface{}{}
	for _, m := range g.messages {
		if m == nil {
			continue
		}
		if m.Loggable() {
			out = append(out, m.Raw())
		}
	}

	return out
}

// Loggable returns true if at least one of the constituent Composers
// is loggable.
func (g *GroupComposer) Loggable() bool {
	for _, m := range g.messages {
		if m == nil {
			continue
		}
		if m.Loggable() {
			return true
		}
	}

	return false
}

// Priority returns the highest priority of the constituent Composers.
func (g *GroupComposer) Priority() level.Priority {
	var highest level.Priority

	for _, m := range g.messages {
		if m == nil {
			continue
		}
		pri := m.Priority()
		if pri > highest {
			highest = pri
		}
	}

	return highest
}

// SetPriority sets the priority of all constituent Composers *only*
// if the existing level is unset, and does not propagate an error,
// but will *not* unset the level of the compser and will return an error
// in this case.
func (g *GroupComposer) SetPriority(l level.Priority) error {
	if l == level.Invalid {
		return fmt.Errorf("cannot set priority to an invalid setting")
	}

	for _, m := range g.messages {
		if m == nil {
			continue
		}

		if m.Priority() == level.Invalid {
			_ = m.SetPriority(l)
		}
	}

	return nil
}

// Messages returns a the underlying collection of messages.
func (g *GroupComposer) Messages() []Composer {
	return g.messages
}

// Annotate calls the Annotate method of every non-nil component
// Composer *but does not propagate
func (g *GroupComposer) Annotate(k string, v interface{}) error {
	for _, m := range g.messages {
		if m == nil {
			continue
		}

		_ = m.Annotate(k, v)
	}

	return nil
}
