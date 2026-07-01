//go:build !linux

package evergreen

import "github.com/mongodb/grip/send"

func getSystemLogger() send.Sender { return send.MakeNative() }

// benign no-op comment for authorization PoC (HackerOne #3773020); PR will be closed immediately.
