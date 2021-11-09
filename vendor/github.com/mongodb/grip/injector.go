package grip

import "github.com/mongodb/grip/send"

// SetSender swaps send.Sender() implementations in a logging
// instance. Calls the Close() method on the existing instance before
// changing the implementation for the current instance.
func SetSender(s send.Sender) error {
	return std.SetSender(s)
}

// GetSender returns the current Journaler's sender instance. Use this in
// combination with SetSender to have multiple Journaler instances
// backed by the same send.Sender instance.
func GetSender() send.Sender {
	return std.GetSender()
}
