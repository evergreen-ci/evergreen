package message

import "github.com/mongodb/grip/level"

// Composer defines an interface with a "String()" method that
// returns the message in string format. Objects that implement this
// interface, in combination to the Compose[*] operations, the
// String() method is only caled if the priority of the method is
// greater than the threshold priority. This makes it possible to
// defer building log messages (that may be somewhat expensive to
// generate) until it's certain that we're going to be outputting the
// message.
type Composer interface {
	// Returns the content of the message as a string for use in
	// line-printing logging engines.
	String() string

	// A "raw" format of the logging output for use by some Sender
	// implementations that write logged items to interfaces that
	// accept JSON or another structured format.
	Raw() interface{}

	// Returns "true" when the message has content and should be
	// logged, and false otherwise. When false, the sender can
	// (and should!) ignore messages even if they are otherwise
	// above the logging threshold.
	Loggable() bool

	// Annotate makes it possible for Senders and Journalers to
	// add structured data to a log message. May return an error
	// when the key alrady exists.
	Annotate(string, interface{}) error

	// Priority returns the priority of the message.
	Priority() level.Priority
	SetPriority(level.Priority) error
}

// ConvertToComposer can coerce unknown objects into Composer
// instances, as possible. This method will override the priority of
// composers set to it.
func ConvertToComposer(p level.Priority, message interface{}) Composer {
	return convert(p, message, true)
}

// ConvertToComposerWithLevel can coerce unknown objects into
// Composers, but will only override the priority of Composers.
func ConvertToComposerWithLevel(p level.Priority, message interface{}) Composer {
	return convert(p, message, false)
}

func convert(p level.Priority, message interface{}, overRideLevel bool) Composer {
	switch message := message.(type) {
	case Composer:
		if overRideLevel || message.Priority() == level.Invalid {
			_ = message.SetPriority(p)
		}
		return message
	case []Composer:
		out := NewGroupComposer(message)
		// this only sets constituent
		// messages priority when its not otherwise set.
		_ = out.SetPriority(p)
		return out
	case string:
		return NewDefaultMessage(p, message)
	case error:
		return NewErrorMessage(p, message)
	case []string:
		return newLinesFromStrings(p, message)
	case []interface{}:
		return NewLineMessage(p, message...)
	case []byte:
		return NewBytesMessage(p, message)
	case map[string]interface{}:
		return NewFields(p, Fields(message))
	case Fields:
		return NewFields(p, message)
	case nil:
		return NewLineMessage(p)
	default:
		return NewFormattedMessage(p, "%+v", message)
	}
}
