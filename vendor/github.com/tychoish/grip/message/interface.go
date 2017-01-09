package message

import "github.com/tychoish/grip/level"

// Composer defines an interface with a "Resolve()" method that
// returns the message in string format. Objects that implement this
// interface, in combination to the Compose[*] operations, the
// Resolve() method is only caled if the priority of the method is
// greater than the threshold priority. This makes it possible to
// defer building log messages (that may be somewhat expensive to
// generate) until it's certain that we're going to be outputting the
// message.
type Composer interface {
	// Returns the content of the message as a string for use in
	// line-printing logging engines.
	Resolve() string

	// A "raw" format of the logging output for use by some Sender
	// implementations that write logged items to interfaces that
	// accept JSON or another structured format.
	Raw() interface{}

	// Returns "true" when the message has content and should be
	// logged, and false otherwise. When false, the sender can
	// (and should!) ignore messages even if they are otherwise
	// above the logging threshold.
	Loggable() bool

	// Priority returns the priority of the message.
	Priority() level.Priority
	SetPriority(level.Priority) error
}

// ConvertToComposer can coerce unknown objects into Composer
// instances, as possible.
func ConvertToComposer(p level.Priority, message interface{}) Composer {
	switch message := message.(type) {
	case Composer:
		_ = message.SetPriority(p)
		return message
	case string:
		return NewDefaultMessage(p, message)
	case []string, []interface{}:
		return NewLineMessage(p, message)
	case error:
		return NewErrorMessage(p, message)
	default:
		return NewFormattedMessage(p, "%+v", message)
	}
}
