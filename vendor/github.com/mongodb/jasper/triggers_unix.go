// +build darwin linux

package jasper

import "syscall"

// makeCleanTerminationSignalTrigger creates a SignalTrigger that is a no-op on
// Unix-based systems.
func makeCleanTerminationSignalTrigger() SignalTrigger {
	return func(_ ProcessInfo, _ syscall.Signal) bool {
		return false
	}
}
