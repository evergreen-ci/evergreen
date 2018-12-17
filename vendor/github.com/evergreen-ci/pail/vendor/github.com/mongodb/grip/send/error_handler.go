package send

import (
	"log"

	"github.com/mongodb/grip/message"
)

// ErrorHandler is a function that you can use define how a sender
// handles errors sending messages. Implementations of this type
// should perform a noop if the err object is nil.
type ErrorHandler func(error, message.Composer)

func ErrorHandlerFromLogger(l *log.Logger) ErrorHandler {
	return func(err error, m message.Composer) {
		if err == nil {
			return
		}

		l.Println("logging error:", err.Error())
		l.Println(m.String())
	}
}

func ErrorHandlerFromSender(s Sender) ErrorHandler {
	return func(err error, m message.Composer) {
		if err == nil {
			return
		}

		s.Send(message.WrapError(err, m))
	}
}
