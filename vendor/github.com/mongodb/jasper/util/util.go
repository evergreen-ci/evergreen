package util

import (
	"io"

	"github.com/mongodb/grip/send"
)

// CloseFunc is a function used to close a service or close the client
// connection to a service.
type CloseFunc func() error

// ConvertWriter wraps the given written in a send.WriterSender for
// compatibility with Grip.
func ConvertWriter(w io.Writer, err error) send.Sender {
	if err != nil {
		return nil
	}

	if w == nil {
		return nil
	}

	return send.WrapWriter(w)
}
