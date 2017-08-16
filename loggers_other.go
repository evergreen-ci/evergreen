// +build !linux

package evergreen

import "github.com/mongodb/grip/send"

func getSystemLogger() send.Sender { return send.MakeNative() }
