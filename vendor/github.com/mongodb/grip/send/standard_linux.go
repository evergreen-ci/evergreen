// +build linux

package send

import "github.com/coreos/go-systemd/journal"

// MakeDefaultSystem constructs a default logger that pushes to
// systemd on platforms where that's available and standard output
// otherwise.
func MakeDefaultSystem() (Sender, error) {
	if journal.Enabled() {
		return MakeSystemdLogger()
	}

	return MakeNative(), nil
}
