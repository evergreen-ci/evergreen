// +build !linux

package send

// MakeDefaultSystem returns a native log sender on all platforms
// other than linux.
func MakeDefaultSystem() (Sender, error) { return MakeNative(), nil }
