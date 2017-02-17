package testutil

import "github.com/tychoish/grip/send"

func SetupTestSender(fn string) send.Sender {
	console := send.MakeCallSiteConsoleLogger(2)
	if fn == "" {
		return console
	}

	sender, err := send.MakeCallSiteFileLogger(fn, 2)
	if err != nil {
		return console
	}

	return sender
}
