package testutil

import (
	"github.com/tychoish/grip"
	"github.com/tychoish/grip/send"
)

func SetupTestSender(fn string) send.Sender {
	sender, err := send.MakeCallSiteFileLogger(fn, 2)
	if err != nil || fn == "" {
		return send.MakeCallSiteConsoleLogger(2)
	}

	if err := grip.SetSender(sender); err != nil {
		sender.Close()
		grip.CatchEmergencyFatal(err)
	}

	return sender
}
