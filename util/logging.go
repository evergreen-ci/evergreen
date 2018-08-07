package util

import (
	"fmt"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

func MakeNotificationErrorHandler(name string) send.ErrorHandler {
	return func(err error, m message.Composer) {
		if err != nil {
			return
		}

		grip.Error(message.WrapError(err, message.Fields{
			"notification":        m.String(),
			"message_type":        fmt.Sprintf("%T", m),
			"notification_target": name,
			"event":               m,
		}))
	}
}
