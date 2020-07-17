// +build linux

package evergreen

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

func getSystemLogger() send.Sender {
	grip.Critical(message.Fields{"test": "ignore pls"})
	sender, err := send.MakeSystemdLogger()
	if err != nil {
		return send.MakeNative()
	}

	return sender
}
